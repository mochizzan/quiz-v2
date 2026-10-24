// Package migrate applies migrations/*.up.sql to the database and seeds the
// first teacher account.
//
// The runner contract is schema spec §9.3: it is a one-shot command (never a
// boot hook), it takes GET_LOCK on a pinned connection so two racers cannot
// interleave, it applies files sorted by name, it writes the ledger row only
// after every statement of a file succeeded, and it stops at the first
// failure. Recovery is fix-forward, because every statement is idempotent
// (§9.1); there is no down-migration (§9.6), so nothing here ever deletes.
//
// DDL lives only here — application code never alters schema (OPS-256).
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/migrations"

	"golang.org/x/crypto/bcrypt"
)

// lockName is the advisory lock that makes concurrent appliers serial. It is
// connection-scoped, so every statement runs on the pinned connection below.
const lockName = "quiz.migrations"

// lockWaitSeconds is the GET_LOCK timeout from §9.3 step 3: one applier at a
// time, waiting up to a half minute before giving up.
const lockWaitSeconds = 30

// liveClause is the soft-delete predicate shared with every domain read. The
// sentinel is a constant in the schema, so inlining it into SQL is exact, not
// injection.
const liveClause = "'" + store.LiveAt + "'"

// ensureLedger is §9.3 step 2.0001 also creates this table; seeding it here
// keeps the contract verbatim even if the first file is ever renamed.
const ensureLedger = `CREATE TABLE IF NOT EXISTS schema_migrations (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  name VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
  PRIMARY KEY (id),
  UNIQUE KEY uq_migrations_name (name, deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`

// Apply executes every migration not yet in the ledger, oldest name first,
// and returns the names it applied. It is safe to run any number of times and
// from any number of processes.
func Apply(ctx context.Context, db *store.DB) (applied []string, err error) {
	return apply(ctx, db, lockWaitSeconds)
}

// apply is Apply with an injectable lock wait, so tests can prove the runner
// refuses to proceed when another process already holds the advisory lock.
func apply(ctx context.Context, db *store.DB, waitSeconds int) (applied []string, err error) {
	// GET_LOCK is scoped to a connection, so the pool is not enough:
	// database/sql may hand the next statement a different connection while
	// the lock quietly evaporates (§9.3 step 3). Everything below runs on
	// conn itself.
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: pin connection: %w", err)
	}
	defer func() {
		if cerr := conn.Close(); err == nil && cerr != nil {
			err = fmt.Errorf("migrate: close connection: %w", cerr)
		}
	}()

	// GET_LOCK reports its verdict as a value, not an error: 1 acquired, 0
	// timed out, NULL a bad call. Checking only ExecContext's error would let
	// two runners past the lock and interleave DDL and ledger writes.
	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", lockName, waitSeconds).Scan(&acquired); err != nil {
		return nil, fmt.Errorf("migrate: acquire %s: %w", lockName, err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return nil, fmt.Errorf("migrate: acquire %s: not granted after %ds, another runner holds it",
			lockName, waitSeconds)
	}

	// Release on the same connection; after ExecContext the result is not
	// checked because every later failure path is more interesting than a
	// missed advisory lock that dies with the connection anyway.
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", lockName)
	}()

	if _, err := conn.ExecContext(ctx, ensureLedger); err != nil {
		return nil, fmt.Errorf("migrate: ledger table: %w", err)
	}

	names, err := embeddedMigrations()
	if err != nil {
		return nil, err
	}

	return applyAll(ctx, conn, names)
}

// migration is one embedded .up.sql file: name without the extension, plus
// its source. Tests construct additional entries directly.
type migration struct {
	name   string
	source string
}

// applyAll runs the migration list, oldest first, skipping names already in
// the ledger. It is Apply's core with the file list injected, so the
// fail-and-stop rule can be tested with a deliberately broken file.
func applyAll(ctx context.Context, conn *sql.Conn, files []migration) (applied []string, err error) {
	done, err := appliedNames(ctx, conn)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if done[file.name] {
			continue
		}
		if err := applyFile(ctx, conn, file); err != nil {
			// Stop at the first failure (§9.3 step 6): the skipped remainder
			// is retried on the next run, and the ledger lists only files
			// whose statements all succeeded (§9.1 ledger last).
			return applied, err
		}
		applied = append(applied, file.name)
	}
	return applied, nil
}

// embeddedMigrations reads every embedded .up.sql file, oldest name first.
// A file that exists but cannot be read is a build packaging error and fails
// the run.
func embeddedMigrations() ([]migration, error) {
	names, err := Files()
	if err != nil {
		return nil, err
	}
	files := make([]migration, 0, len(names))
	for _, name := range names {
		source, err := migrations.SQL.ReadFile(name + ".up.sql")
		if err != nil {
			return nil, fmt.Errorf("migrate: read %s: %w", name, err)
		}
		files = append(files, migration{name: name, source: string(source)})
	}
	return files, nil
}

// applyFile executes one migration's statements in order, then records it. The
// statements run unwrapped because DDL implicitly commits anyway (§9.1); the
// ledger row is the only transaction-like boundary.
func applyFile(ctx context.Context, tx *sql.Conn, file migration) error {
	stmts, err := Statements(file.source)
	if err != nil {
		return fmt.Errorf("migrate: %s: %w", file.name, err)
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: %s: %w", file.name, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (name) VALUES (?)", file.name); err != nil {
		return fmt.Errorf("migrate: ledger %s: %w", file.name, err)
	}
	return nil
}

// appliedNames reads the live ledger into a set. A soft-deleted ledger row is
// deliberately absent: it re-enqueues the file, which every statement's
// idempotency makes safe (§6.15) — but never something done on purpose.
func appliedNames(ctx context.Context, conn *sql.Conn) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx,
		"SELECT name FROM schema_migrations WHERE deleted_at = "+liveClause)
	if err != nil {
		return nil, fmt.Errorf("migrate: read ledger: %w", err)
	}
	defer rows.Close()

	done := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("migrate: scan ledger: %w", err)
		}
		done[name] = true
	}
	return done, rows.Err()
}

// Files lists the embedded migration names (without ".up.sql") sorted
// lexicographically, which for zero-padded numbers is oldest first (§9.3
// step 4).
func Files() ([]string, error) {
	entries, err := fs.ReadDir(migrations.SQL, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: list: %w", err)
	}
	names := []string{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".up.sql"))
	}
	sort.Strings(names)
	return names, nil
}

// LedgerCount reports how many migrations the ledger currently records. The
// apply action prints it so an operator can compare it against the count of
// files in migrations/ with one glance.
func LedgerCount(ctx context.Context, db *store.DB) (int, error) {
	var n int
	err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM schema_migrations").Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("migrate: count ledger: %w", err)
	}
	return n, nil
}

// Statements splits an .up.sql file into executable statements. The files
// belonging to this schema (§9.1) use no DELIMITER blocks and never put a
// semicolon inside a string literal, so a line-oriented split is exact: script
// headers start with "--" and are dropped, and a statement is everything read
// until a line ends with ";".
func Statements(source string) (stmts []string, err error) {
	var buf strings.Builder
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		if strings.HasSuffix(trimmed, ";") {
			buf.WriteString(strings.TrimSuffix(trimmed, ";"))
			stmt := strings.TrimSpace(buf.String())
			if stmt != "" {
				stmts = append(stmts, stmt)
			}
			buf.Reset()
			continue
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	rest := strings.TrimSpace(buf.String())
	if rest != "" && !strings.HasPrefix(rest, "--") {
		return nil, errors.New("statements: unterminated statement (missing ';')")
	}
	return stmts, nil
}

// BootstrapTeacher creates the one account that cannot be created by
// invitation: the first teacher nobody has approved yet (schema §9.5). It is
// idempotent on email — a second run with the same address creates nothing —
// and it never verifies the caller's password: the hash arrives pre-computed
// so a credential never needs to be written to disk or git.
func BootstrapTeacher(ctx context.Context, db *store.DB, fullName, email, passwordHash string) (created bool, err error) {
	normEmail, err := secure.NormalizeEmail(email)
	if err != nil {
		return false, fmt.Errorf("migrate: bootstrap teacher: %w", err)
	}
	normName := secure.NormalizeName(fullName)
	if normName == "" {
		return false, errors.New("migrate: bootstrap teacher: BOOTSTRAP_TEACHER_NAME is empty")
	}
	if err := checkBcryptHash(passwordHash); err != nil {
		return false, fmt.Errorf("migrate: bootstrap teacher: %w", err)
	}

	var existing int64
	err = db.QueryRowContext(ctx,
		"SELECT id FROM accounts WHERE email = ? AND deleted_at = "+liveClause,
		normEmail).Scan(&existing)
	switch {
	case err == nil:
		return false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return false, fmt.Errorf("migrate: bootstrap teacher: %w", err)
	}

	// must_change_password=1 is the rule that makes the invited password
	// disposable: the teacher replaces it on first login (§9.5).
	result, err := db.ExecContext(ctx,
		"INSERT INTO accounts (full_name, email, password_hash, role, status, approved_at, must_change_password) "+
			"VALUES (?, ?, ?, 'teacher', 'active', CURRENT_TIMESTAMP, 1)",
		normName, normEmail, passwordHash)
	if store.IsDuplicate(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("migrate: bootstrap teacher: %w", err)
	}
	created, err = resultRowsAffected(result)
	if err != nil {
		return false, fmt.Errorf("migrate: bootstrap teacher: %w", err)
	}
	return created, nil
}

// checkBcryptHash refuses anything the login path could not verify. A
// committed or half-typed hash fails here instead of producing an account
// nobody can log into. Cost comes from the same library as VerifyPassword, so
// "the runner accepts it" and "bcrypt compares it" cannot disagree.
func checkBcryptHash(hash string) error {
	if len(hash) != 60 {
		return errors.New("BOOTSTRAP_TEACHER_PASSWORD_HASH is not a bcrypt hash (60 chars, starts $2)")
	}
	if _, err := bcrypt.Cost([]byte(hash)); err != nil {
		return errors.New("BOOTSTRAP_TEACHER_PASSWORD_HASH is not a bcrypt hash (60 chars, starts $2)")
	}
	return nil
}

// resultRowsAffected reports whether the statement wrote exactly one row.
func resultRowsAffected(result sql.Result) (bool, error) {
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}
