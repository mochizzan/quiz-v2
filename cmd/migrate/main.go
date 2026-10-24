// Command migrate applies the schema migrations and seeds the first teacher.
//
// It is a one-shot command, never an application boot hook (schema spec
// §9.3): the app may start before the database is reachable, and an advisory
// ping must not become a hard dependency. Run it after a deploy that ships
// new migrations, or before the first start against an empty database.
//
// Actions:
//
//	migrate                     apply every unapplied migration, oldest first
//	migrate bootstrap-teacher   apply migrations, then create the first
//	                            teacher account, idempotent on email
//	migrate hash                read a password from stdin and print the
//	                            bcrypt hash for BOOTSTRAP_TEACHER_PASSWORD_HASH
//
// Environment:
//
//	DB_DSN                          the application DSN; default config.DefaultDSN,
//	                                checked with store.CheckDSN
//	BOOTSTRAP_TEACHER_NAME          bootstrap-teacher: full name
//	BOOTSTRAP_TEACHER_EMAIL         bootstrap-teacher: email
//	BOOTSTRAP_TEACHER_PASSWORD      bootstrap-teacher: the handoff password; the
//	                                runner hashes it, so no hash has to be
//	                                produced and pasted by hand
//	BOOTSTRAP_TEACHER_PASSWORD_HASH bootstrap-teacher: alternative to the above,
//	                                a bcrypt hash (produce with `migrate hash`)
//
// Seeding is mandatory: a bootstrap that cannot produce a live teacher account
// exits non-zero, and compose stops the deployment on that exit. A second run
// with the same email is a success, because the account is already there.
//
// The runner is the only place DDL may appear (OPS-256); console output is a
// short operator report, and exit 0 means the database is current.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"quiz-v2/internal/config"
	"quiz-v2/internal/migrate"
	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = config.DefaultDSN
	}
	if err := store.CheckDSN(dsn); err != nil {
		return fmt.Errorf("migrate: %w (set DB_DSN)", err)
	}

	action, err := actionName()
	if err != nil {
		return err
	}
	if action == "hash" {
		// Produces the value for BOOTSTRAP_TEACHER_PASSWORD_HASH without a
		// password ever touching argv (visible in shell history) or git.
		hash, err := hashPassword(os.Stdin)
		if err != nil {
			return fmt.Errorf("migrate: hash: %w", err)
		}
		fmt.Println(hash)
		return nil
	}

	db, err := store.Open(dsn)
	if err != nil {
		return fmt.Errorf("migrate: open pool: %w", err)
	}
	defer db.Close()

	switch action {
	case "":
		applied, ledger, err := applyAll(ctx, db)
		if err != nil {
			return err
		}
		fmt.Printf("applied %d, ledger holds %d, database is current\n", applied, ledger)
		return nil

	case "bootstrap-teacher": // §9.5: idempotent on email, approved at creation
		applied, ledger, err := applyAll(ctx, db)
		if err != nil {
			return err
		}
		fmt.Printf("applied %d, ledger holds %d, schema is current\n", applied, ledger)
		fullName := os.Getenv("BOOTSTRAP_TEACHER_NAME")
		email := os.Getenv("BOOTSTRAP_TEACHER_EMAIL")
		if fullName == "" || email == "" {
			return fmt.Errorf("migrate: bootstrap-teacher needs BOOTSTRAP_TEACHER_NAME and BOOTSTRAP_TEACHER_EMAIL")
		}
		hash, err := bootstrapHash(os.Getenv("BOOTSTRAP_TEACHER_PASSWORD"), os.Getenv("BOOTSTRAP_TEACHER_PASSWORD_HASH"))
		if err != nil {
			return err
		}
		created, err := migrate.BootstrapTeacher(ctx, db, fullName, email, hash)
		if err != nil {
			return err
		}
		if !created {
			fmt.Println("teacher account for that email already exists, nothing created")
		} else {
			// The account is created with must_change_password=1 (§9.5); the
			// invited password is a handoff, not a credential (§9.5).
			fmt.Println("teacher account created, first login forces a password change")
		}
		return nil

	default:
		return fmt.Errorf("migrate: unknown action %q (use one of: apply, bootstrap-teacher, hash)", action)
	}
}

func actionName() (string, error) {
	switch len(os.Args) {
	case 1:
		return "", nil
	case 2:
		return os.Args[1], nil
	default:
		return "", fmt.Errorf("migrate: one action at a time, got %v", os.Args[1:])
	}
}

func applyAll(ctx context.Context, db *store.DB) (applied, ledger int, err error) {
	names, err := migrate.Apply(ctx, db)
	if err != nil {
		return 0, 0, err
	}
	total, err := migrate.LedgerCount(ctx, db)
	if err != nil {
		return 0, 0, err
	}
	return len(names), total, nil
}

// bootstrapHash resolves the invited password for a bootstrap run. Plaintext is
// the documented path: one value in .env, hashed here, so no hash has to be
// produced by hand and the secret never reaches argv. The pre-hashed form stays
// for operators who do not want a cleartext credential on disk.
func bootstrapHash(password, hash string) (string, error) {
	switch {
	case password != "" && hash != "":
		return "", errors.New("migrate: set BOOTSTRAP_TEACHER_PASSWORD or BOOTSTRAP_TEACHER_PASSWORD_HASH, not both")
	case password != "":
		if err := secure.ValidatePassword(password); err != nil {
			return "", fmt.Errorf("migrate: BOOTSTRAP_TEACHER_PASSWORD: %w", err)
		}
		return secure.HashPassword(password)
	case hash != "":
		return hash, nil
	default:
		return "", errors.New("migrate: bootstrap-teacher needs BOOTSTRAP_TEACHER_PASSWORD (or BOOTSTRAP_TEACHER_PASSWORD_HASH)")
	}
}

// hashPassword reads one line from bin and prints its bcrypt hash. The
// password policy applies: the operator's handoff must also be a valid
// password for a first-login student.
func hashPassword(bin io.Reader) (string, error) {
	line, err := bufio.NewReader(bin).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	password := strings.Trim(line, " \t\r\n")
	if err := secure.ValidatePassword(password); err != nil {
		return "", err
	}
	return secure.HashPassword(password)
}
