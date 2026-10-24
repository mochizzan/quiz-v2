package identity

// Integration tests for the identity domain. They run only when TEST_DB_DSN is
// set (storetest.DB skips otherwise), because they need the real MariaDB schema,
// unique keys, CHECK constraints, and transaction semantics:
//
//	TEST_DB_DSN='quiz:quiz@tcp(127.0.0.1:3306)/quiz?parseTime=true&loc=UTC' go test ./internal/identity/...
//
// Each test builds one fixture (a fresh pool, the application signer, and the
// session store) and starts from an empty database. The bcrypt-backed commands
// are the only slow paths: every seeded password shares one hash.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/authn"
	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/internal/store/storetest"
	"quiz-v2/internal/viewmodel"
)

const (
	seedPassword  = "SeedPass1"
	seedUA        = "integration-agent/1.0"
	testTeacherIP = "192.168.1.10"
)

// fixture is one test's database-backed identity surface.
type fixture struct {
	t      *testing.T
	db     *store.DB
	signer *secure.Signer
	auth   *authn.Store
	svc    *Service
	ctx    context.Context
}

// accountReferenceHeals clears the two self-referential foreign keys on
// accounts (fk_accounts_deleter, fk_accounts_approver). DELETE FROM accounts
// alone cannot drain them, which is how a stale aborted run makes
// storetest.Reset unreachable for any suite that does not heal first.
var accountReferenceHeals = []string{
	`UPDATE accounts SET deleted_by_account_id = NULL WHERE deleted_by_account_id IS NOT NULL`,
	`UPDATE accounts SET approved_by_account_id = NULL WHERE approved_by_account_id IS NOT NULL`,
}

// healAndDrain heals the self-references and then hands the drained tables to
// storetest.Reset. It runs on fixture entry (recovering from any stale state,
// including aborted runs) and once more in the test's cleanup, so the suite
// does not leave deleter/approver rows behind for the next package's Reset.
func healAndDrain(t *testing.T, db *store.DB) {
	t.Helper()
	for _, heal := range accountReferenceHeals {
		if _, err := db.ExecContext(context.Background(), heal); err != nil {
			t.Fatalf("heal %q: %v", heal, err)
		}
	}
	storetest.Reset(t, db)
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := storetest.DB(t)
	healAndDrain(t, db)
	// The exit heal mirrors the entry heal: without it, the rows this suite
	// leaves behind brick storetest.Reset for every later package (it runs
	// before identity alphabetically and cannot heal on its own).
	t.Cleanup(func() { healAndDrain(t, db) })
	signer, err := secure.NewSigner([]byte("identity-integration-signing-key-32b!!"))
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	f := &fixture{
		t:      t,
		db:     db,
		signer: signer,
		auth:   authn.NewStore(db),
		ctx:    context.Background(),
	}
	f.svc = New(db, signer, f.auth)
	return f
}

func (f *fixture) exec(query string, args ...any) sql.Result {
	f.t.Helper()
	res, err := f.db.ExecContext(f.ctx, query, args...)
	if err != nil {
		f.t.Fatalf("seed exec %q: %v", query, err)
	}
	return res
}

func (f *fixture) id(res sql.Result, what string) int64 {
	f.t.Helper()
	id, err := res.LastInsertId()
	if err != nil {
		f.t.Fatalf("seed %s id: %v", what, err)
	}
	return id
}

// seedHash shares one bcrypt digest across every seeded account so the suite
// pays the cost-12 price once.
var (
	seedHashOnce sync.Once
	seedHash     string
)

func (f *fixture) hash() string {
	f.t.Helper()
	seedHashOnce.Do(func() {
		sum, err := secure.HashPassword(seedPassword)
		if err != nil {
			f.t.Fatalf("hash seed password: %v", err)
		}
		seedHash = sum
	})
	return seedHash
}

// seedTeacher inserts one teacher account. A pending teacher must have no
// approval stamp (chk_accounts_teacher_approval), an active one must.
func (f *fixture) seedTeacher(email, name string, status viewmodel.AccountStatus) int64 {
	f.t.Helper()
	approved := "NULL"
	if status == viewmodel.AccountActive {
		approved = "UTC_TIMESTAMP()"
	}
	res := f.exec(fmt.Sprintf(`INSERT INTO accounts
		(full_name, email, password_hash, role, status, approved_at, must_change_password)
		VALUES (?, ?, ?, 'teacher', ?, %s, 0)`, approved),
		name, email, f.hash(), string(status))
	return f.id(res, "teacher "+email)
}

// seedStudent inserts one active student account and its live profile; both
// ids are returned.
func (f *fixture) seedStudent(email, name string, grade int, majorID int64, mustChange int) (accountID, studentID int64) {
	f.t.Helper()
	res := f.exec(`INSERT INTO accounts
		(full_name, email, password_hash, role, status, approved_at, must_change_password)
		VALUES (?, ?, ?, 'student', 'active', NULL, ?)`, name, email, f.hash(), mustChange)
	accountID = f.id(res, "student account "+email)
	studentID = f.id(f.exec(`
		INSERT INTO students (account_id, grade_level, major_id) VALUES (?, ?, ?)`,
		accountID, grade, majorID), "student profile "+email)
	return accountID, studentID
}

func (f *fixture) seedMajor(name, code string) int64 {
	f.t.Helper()
	return f.id(f.exec("INSERT INTO majors (name, code) VALUES (?, ?)", name, code), "major "+name)
}

// softDeleteAccount stamps the canonical soft-delete on one account; the
// deleter must itself be live (fk_accounts_deleter). The cleanup drops the
// cross-row deleter link afterwards so the self-referential foreign key never
// blocks the next test's storetest.Reset.
func (f *fixture) softDeleteAccount(actorID, accountID int64) {
	f.t.Helper()
	f.exec(`UPDATE accounts SET deleted_at = UTC_TIMESTAMP(), deleted_by_account_id = ?
		WHERE id = ? AND deleted_at = ?`, actorID, accountID, store.LiveAt)
	f.t.Cleanup(func() {
		if _, err := f.db.ExecContext(f.ctx, `UPDATE accounts
			SET deleted_by_account_id = NULL WHERE deleted_by_account_id IS NOT NULL`); err != nil {
			f.t.Errorf("heal fk_accounts_deleter: %v", err)
		}
	})
}

func (f *fixture) count(table, where string, args ...any) int64 {
	f.t.Helper()
	var n int64
	if err := f.db.QueryRowContext(f.ctx,
		"SELECT COUNT(*) FROM "+table+" "+where, args...).Scan(&n); err != nil {
		f.t.Fatalf("count %s %s: %v", table, where, err)
	}
	return n
}

// countLive counts the still-live rows of one table.
func (f *fixture) countLive(table string) int64 {
	return f.count(table, "WHERE deleted_at = ?", store.LiveAt)
}

// qustring returns the first column of a one-row query as a NullString.
func (f *fixture) qustring(source string, args ...any) sql.NullString {
	f.t.Helper()
	var v sql.NullString
	if err := f.db.QueryRowContext(f.ctx, source, args...).Scan(&v); err != nil {
		f.t.Fatalf("query %s: %v", source, err)
	}
	return v
}

// quBool scans a boolean expression that MariaDB returns as 0/1.
func (f *fixture) quBool(source string, args ...any) bool {
	f.t.Helper()
	var n int
	if err := f.db.QueryRowContext(f.ctx, source, args...).Scan(&n); err != nil {
		f.t.Fatalf("query %s: %v", source, err)
	}
	return n == 1
}

// actorIP is the fixed peer address the credential commands run under.
func actorIP() net.IP { return net.IPv4(192, 168, 1, 10) }

// countingDummyCompare swaps the unknown-email bcrypt seam for a counter and
// restores it when the test ends (IDN-060).
func countingDummyCompare(t *testing.T) *int {
	t.Helper()
	count := new(int)
	original := spendDummyCompare
	spendDummyCompare = func(string) { *count++ }
	t.Cleanup(func() { spendDummyCompare = original })
	return count
}

func requireIdentical(t *testing.T, err error, want error, what string) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s: err = %v, want %v", what, err, want)
	}
}

func requireForm(t *testing.T, err error, field, message string) {
	t.Helper()
	var fe *FormError
	if !errors.As(err, &fe) {
		t.Fatalf("err = %v (%T), want *FormError", err, err)
	}
	if got := fe.Fields[field]; got != message {
		t.Fatalf("form field %q = %q (%v), want %q", field, got, fe.Fields, message)
	}
}

// ---- Login (IDN-C03, IDN-060, IDN-073) --------------------------------------

func TestLoginCredentialBranches(t *testing.T) {
	f := newFixture(t)
	_ = f.seedTeacher("guru@x.test", "Guru Aktif", viewmodel.AccountActive)
	_ = f.seedTeacher("baru@x.test", "Guru Baru", viewmodel.AccountPending)
	_, _ = f.seedStudent("murid@x.test", "Murid Satu", 10, f.seedMajor("Logistik", "LOG"), 0)

	t.Run("unknown email spends exactly one dummy compare", func(t *testing.T) {
		n := countingDummyCompare(t)
		_, err := f.svc.Login(f.ctx, "nosuch@x.test", seedPassword, actorIP(), seedUA)
		requireIdentical(t, err, ErrInvalidCredentials, "unknown email")
		if *n != 1 {
			t.Fatalf("unknown email dummy compares = %d, want 1 (IDN-060)", *n)
		}
	})

	t.Run("malformed email costs like unknown", func(t *testing.T) {
		n := countingDummyCompare(t)
		_, err := f.svc.Login(f.ctx, "not-an-email", seedPassword, actorIP(), seedUA)
		requireIdentical(t, err, ErrInvalidCredentials, "malformed email")
		if *n != 1 {
			t.Fatalf("malformed email dummy compares = %d, want 1", *n)
		}
	})

	t.Run("wrong password spends no dummy compare", func(t *testing.T) {
		n := countingDummyCompare(t)
		_, err := f.svc.Login(f.ctx, "guru@x.test", "WrongPass9", actorIP(), seedUA)
		requireIdentical(t, err, ErrInvalidCredentials, "wrong password")
		if *n != 0 {
			t.Fatalf("wrong password dummy compares = %d, want 0", *n)
		}
	})

	t.Run("pending teacher is its own branch", func(t *testing.T) {
		n := countingDummyCompare(t)
		_, err := f.svc.Login(f.ctx, "baru@x.test", seedPassword, actorIP(), seedUA)
		requireIdentical(t, err, ErrPendingTeacher, "pending teacher")
		if *n != 0 {
			t.Fatalf("pending teacher dummy compares = %d, want 0", *n)
		}
	})

	t.Run("soft-deleted account is invisible", func(t *testing.T) {
		support := f.seedTeacher("support@x.test", "Guru Penghapus", viewmodel.AccountActive)
		victim := f.seedTeacher("gone@x.test", "Guru Terhapus", viewmodel.AccountActive)
		f.softDeleteAccount(support, victim)
		if _, err := f.svc.Login(f.ctx, "gone@x.test", seedPassword, actorIP(), seedUA); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("deleted account login err = %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("email normalisation finds the cased variant", func(t *testing.T) {
		res, err := f.svc.Login(f.ctx, "  Guru@X.TEST ", seedPassword, actorIP(), seedUA)
		if err != nil {
			t.Fatalf("cased login: %v", err)
		}
		if res.Role != viewmodel.RoleTeacher {
			t.Fatalf("role = %q", res.Role)
		}
	})

	t.Run("nil session store is an internal fault", func(t *testing.T) {
		plain := New(f.db, f.signer, nil)
		_, err := plain.Login(f.ctx, "guru@x.test", seedPassword, actorIP(), seedUA)
		var appErr *apperr.Error
		if !errors.As(err, &appErr) || appErr.Kind() != apperr.Internal {
			t.Fatalf("nil-store login err = %v, want internal", err)
		}
	})
}

func TestLoginSuccessSessionAndStamp(t *testing.T) {
	f := newFixture(t)
	major := f.seedMajor("Logistik", "LOG")
	teacherID := f.seedTeacher("guru@x.test", "Guru Aktif", viewmodel.AccountActive)
	studentID, _ := f.seedStudent("murid@x.test", "Murid Satu", 10, major, 1)

	res, err := f.svc.Login(f.ctx, "guru@x.test", seedPassword, actorIP(), seedUA)
	if err != nil {
		t.Fatalf("teacher login: %v", err)
	}
	if res.Token == "" || res.SessionID == 0 {
		t.Fatalf("login result incomplete: %+v", res)
	}
	if res.Role != viewmodel.RoleTeacher || res.MustChangePassword {
		t.Fatalf("teacher login flags: %+v", res)
	}

	sess, err := f.auth.Resolve(f.ctx, res.Token)
	if err != nil {
		t.Fatalf("resolve issued token: %v", err)
	}
	if sess == nil || sess.AccountID != teacherID {
		t.Fatalf("resolved session = %+v, want account %d", sess, teacherID)
	}

	if ua := f.qustring("SELECT user_agent FROM sessions WHERE id = ?", res.SessionID); ua.String != seedUA {
		t.Fatalf("session user_agent = %q", ua.String)
	}
	if !f.quBool("SELECT last_login_at IS NOT NULL FROM accounts WHERE id = ?", teacherID) {
		t.Fatal("last_login_at missing after successful login (IDN-073)")
	}

	// must_change_password travels on the credential projection (IDN-021).
	sres, err := f.svc.Login(f.ctx, "murid@x.test", seedPassword, actorIP(), seedUA)
	if err != nil {
		t.Fatalf("student login: %v", err)
	}
	if !sres.MustChangePassword || sres.Role != viewmodel.RoleStudent {
		t.Fatalf("student login flags: %+v", sres)
	}

	if got := f.count("sessions", "WHERE account_id IN (?, ?)", teacherID, studentID); got != 2 {
		t.Fatalf("sessions rows = %d, want 2", got)
	}
}

// ---- Register teacher (IDN-C05) --------------------------------------------

func TestRegisterTeacherCommand(t *testing.T) {
	f := newFixture(t)

	token, err := f.svc.RegisterTeacher(f.ctx, "Guru Baru", "Baru@X.test", seedPassword, seedPassword)
	if err != nil {
		t.Fatalf("register teacher: %v", err)
	}

	// The token pins exactly the new account id (R-23, IDN-088).
	accountID, err := ParsePendingToken(f.signer, token, time.Now().UTC())
	if err != nil || accountID <= 0 {
		t.Fatalf("pending token parse = %d, %v", accountID, err)
	}
	var role, status string
	var approved sql.NullTime
	if err := f.db.QueryRowContext(f.ctx, `SELECT role, status, approved_at
		FROM accounts WHERE id = ?`, accountID).Scan(&role, &status, &approved); err != nil {
		t.Fatalf("registered teacher row: %v", err)
	}
	if role != string(viewmodel.RoleTeacher) || status != string(viewmodel.AccountPending) || approved.Valid {
		t.Fatalf("teacher row = (%q, %q, %v)", role, status, approved)
	}
	if got := f.count("sessions", "WHERE account_id = ?", accountID); got != 0 {
		t.Fatalf("registration minted %d sessions; want none (IDN-073)", got)
	}
	if got := f.count("students", "WHERE account_id = ?", accountID); got != 0 {
		t.Fatalf("teacher profile exists (INV-06)")
	}

	t.Run("duplicate live email is the email field", func(t *testing.T) {
		_, err := f.svc.RegisterTeacher(f.ctx, "Guru Dua", "baru@x.test", seedPassword, seedPassword)
		requireForm(t, err, "email", apperr.MsgEmailTaken)
	})

	t.Run("invalid form carries every missing field", func(t *testing.T) {
		_, err := f.svc.RegisterTeacher(f.ctx, "", "", "", "")
		var fe *FormError
		if !errors.As(err, &fe) {
			t.Fatalf("blank register err = %v", err)
		}
		for _, field := range []string{"full_name", "email", "password", "password_confirm"} {
			if fe.Fields[field] == "" {
				t.Fatalf("blank form missing %q: %v", field, fe.Fields)
			}
		}
		if got := f.countLive("accounts"); got != 1 {
			t.Fatalf("invalid submission inserted rows: %d live accounts", got)
		}
	})
}

// ---- Register student (IDN-C07, INV-02) -------------------------------------

func TestRegisterStudentCommand(t *testing.T) {
	f := newFixture(t)
	majorID := f.seedMajor("Rekayasa Perangkat Lunak", "RPL")

	res, err := f.svc.RegisterStudent(f.ctx, "Murid Satu", "Murid@X.test",
		seedPassword, seedPassword, "10", fmt.Sprintf("%d", majorID), actorIP(), seedUA)
	if err != nil {
		t.Fatalf("register student: %v", err)
	}
	if res.Token == "" || res.SessionID == 0 {
		t.Fatalf("student registration incomplete: %+v", res)
	}
	accID, stID := f.studentBy("murid@x.test")
	if accID == 0 || stID == 0 {
		t.Fatalf("student rows missing")
	}
	var grade int
	var major, status, role string
	if err := f.db.QueryRowContext(f.ctx, `
		SELECT s.major_id, a.status, a.role, s.grade_level
		FROM students s JOIN accounts a ON a.id = s.account_id WHERE s.id = ?`, stID).
		Scan(&major, &status, &role, &grade); err != nil {
		t.Fatalf("student profile row: %v", err)
	}
	if grade != 10 || fmt.Sprintf("%s", major) != fmt.Sprintf("%d", majorID) ||
		status != string(viewmodel.AccountActive) || role != string(viewmodel.RoleStudent) {
		t.Fatalf("student profile = (major %q, %q, %q, grade %d)", major, status, role, grade)
	}
	if got := f.count("sessions", "WHERE account_id = ?", accID); got != 1 {
		t.Fatalf("student session rows = %d, want 1", got)
	}

	sess, err := f.auth.Resolve(f.ctx, res.Token)
	if err != nil {
		t.Fatalf("resolve registration token: %v", err)
	}
	if sess == nil || sess.GradeLevel != 10 {
		t.Fatalf("registration session = %+v", sess)
	}

	t.Run("dead major fails atomically under major_id", func(t *testing.T) {
		before := f.countLive("accounts")
		_, err := f.svc.RegisterStudent(f.ctx, "Murid Dua", "dua@x.test",
			seedPassword, seedPassword, "11", "999999", actorIP(), seedUA)
		requireForm(t, err, "major_id", MsgMajorRequired)
		if got := f.countLive("accounts"); got != before {
			t.Fatalf("dead-major registration left %d new accounts (INV-02)", got-before)
		}
	})

	t.Run("duplicate email is email_taken", func(t *testing.T) {
		_, err := f.svc.RegisterStudent(f.ctx, "Murid Tiga", "murid@x.test",
			seedPassword, seedPassword, "12", fmt.Sprintf("%d", majorID), actorIP(), seedUA)
		requireForm(t, err, "email", apperr.MsgEmailTaken)
	})
}

// studentBy looks up one seed by its normalised email.
func (f *fixture) studentBy(email string) (accountID, studentID int64) {
	f.t.Helper()
	err := f.db.QueryRowContext(f.ctx, `
		SELECT a.id, s.id
		FROM accounts a JOIN students s ON s.account_id = a.id
		WHERE a.email = ? AND a.deleted_at = ? AND s.deleted_at = ?`,
		email, store.LiveAt, store.LiveAt).Scan(&accountID, &studentID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0
	}
	if err != nil {
		f.t.Fatalf("studentBy(%s): %v", email, err)
	}
	return accountID, studentID
}

func (f *fixture) studentIDOf(email string) int64 {
	f.t.Helper()
	_, stID := f.studentBy(email)
	return stID
}

func (f *fixture) studentAccountOf(studentID int64) int64 {
	f.t.Helper()
	var accID int64
	if err := f.db.QueryRowContext(f.ctx,
		"SELECT account_id FROM students WHERE id = ?", studentID).Scan(&accID); err != nil {
		f.t.Fatalf("studentAccountOf(%d): %v", studentID, err)
	}
	return accID
}

// ---- Approve teacher (IDN-C22) ---------------------------------------------

func TestApproveTeacherMonotonic(t *testing.T) {
	f := newFixture(t)
	actor := f.seedTeacher("ketua@x.test", "Ketua Guru", viewmodel.AccountActive)
	other := f.seedTeacher("kedua@x.test", "Guru Kedua", viewmodel.AccountActive)
	target := f.seedTeacher("baru@x.test", "Guru Baru", viewmodel.AccountPending)
	studentTarget, _ := f.seedStudent("murid@x.test", "Murid", 10, f.seedMajor("Logistik", "LOG"), 0)

	flash, err := f.svc.ApproveTeacher(f.ctx, actor, target)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if flash != FlashApproved {
		t.Fatalf("first approval flash = %q, want %q", flash, FlashApproved)
	}
	var status, approver string
	var approvedAt time.Time
	if err := f.db.QueryRowContext(f.ctx, `SELECT a.status,
			COALESCE(x.full_name, ''), a.approved_at
		FROM accounts a
		LEFT JOIN accounts x ON x.id = a.approved_by_account_id
		WHERE a.id = ?`, target).Scan(&status, &approver, &approvedAt); err != nil {
		t.Fatalf("approved row: %v", err)
	}
	if status != string(viewmodel.AccountActive) || approver != "Ketua Guru" || approvedAt.IsZero() {
		t.Fatalf("approved row = (%q, %q, %v)", status, approver, approvedAt)
	}

	// The second approval is a no-op carrying the idempotent key (IDN-C22).
	flash, err = f.svc.ApproveTeacher(f.ctx, other, target)
	if err != nil {
		t.Fatalf("second approve: %v", err)
	}
	if flash != FlashAlreadyApproved {
		t.Fatalf("second approval flash = %q, want %q", flash, FlashAlreadyApproved)
	}
	if again := f.qustring(`SELECT COALESCE(x.full_name, '') FROM accounts a
		LEFT JOIN accounts x ON x.id = a.approved_by_account_id WHERE a.id = ?`, target); again.String != "Ketua Guru" {
		t.Fatalf("second approval rewrote the approver: %q", again.String)
	}

	for name, target := range map[string]int64{
		"student target": studentTarget, "missing target": 999999,
		"zero target": 0, "negative target": -3,
	} {
		if _, err := f.svc.ApproveTeacher(f.ctx, actor, target); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s: err = %v, want ErrNotFound", name, err)
		}
	}
}

// ---- Pending page read model (IDN-C12) --------------------------------------

// mustTokenOtherKey signs one pending payload with a foreign key.
func mustTokenOtherKey(t *testing.T) string {
	t.Helper()
	other, err := secure.NewSigner([]byte("pending-token-signed-by-foreign-key!!!"))
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	raw, err := NewPendingToken(other, 5, time.Now().UTC())
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return raw
}

func TestPendingPageStates(t *testing.T) {
	f := newFixture(t)
	approver := f.seedTeacher("ketua@x.test", "Ketua Guru", viewmodel.AccountActive)
	_ = approver

	token, err := f.svc.RegisterTeacher(f.ctx, "Guru Menunggu", "menunggu@x.test", seedPassword, seedPassword)
	if err != nil {
		t.Fatalf("register teacher: %v", err)
	}
	accountID, err := ParsePendingToken(f.signer, token, time.Now().UTC())
	if err != nil {
		t.Fatalf("parse pending token: %v", err)
	}

	if info, err := f.svc.Pending(f.ctx, token); err != nil ||
		info.State != viewmodel.PendingWaiting || info.FullName != "Guru Menunggu" {
		t.Fatalf("waiting state = %+v, %v", info, err)
	}

	if _, err := f.svc.ApproveTeacher(f.ctx, f.seedTeacher("ketua2@x.test", "Ketua Dua", viewmodel.AccountActive), accountID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if info, err := f.svc.Pending(f.ctx, token); err != nil ||
		info.State != viewmodel.PendingApproved || info.FullName != "Guru Menunggu" {
		t.Fatalf("approved state = %+v, %v", info, err)
	}

	// The 24-hour window is enforced by the token, not the database.
	later := New(f.db, f.signer, f.auth)
	later.now = func() time.Time { return time.Now().UTC().Add(PendingTokenTTL + time.Minute) }
	if info, err := later.Pending(f.ctx, token); err != nil ||
		info.State != viewmodel.PendingExpired {
		t.Fatalf("shifted-clock state = %+v, %v", info, err)
	}

	states := []struct {
		name  string
		token string
	}{
		{name: "empty token"},
		{name: "garbage token", token: "not-a-token"},
		{name: "other-key token", token: mustTokenOtherKey(t)},
		{name: "truncated token", token: token[:len(token)-2]},
	}
	for _, tc := range states {
		info, err := f.svc.Pending(f.ctx, tc.token)
		if err != nil {
			t.Fatalf("%s: unexpected error %v", tc.name, err)
		}
		if info.State != viewmodel.PendingExpired {
			t.Fatalf("%s: state = %v, want expired", tc.name, info.State)
		}
	}

	// An id whose account is not a teacher resolves to expired, never 500.
	studentID, _ := f.seedStudent("murid@x.test", "Murid", 10, f.seedMajor("Logistik", "LOG"), 0)
	studentToken, err := NewPendingToken(f.signer, studentID, time.Now().UTC())
	if err != nil {
		t.Fatalf("student token: %v", err)
	}
	if info, err := f.svc.Pending(f.ctx, studentToken); err != nil || info.State != viewmodel.PendingExpired {
		t.Fatalf("student-role token = %+v, %v", info, err)
	}

	// An id whose account no longer exists is expired too.
	missingToken, err := NewPendingToken(f.signer, 987654, time.Now().UTC())
	if err != nil {
		t.Fatalf("missing token: %v", err)
	}
	if info, err := f.svc.Pending(f.ctx, missingToken); err != nil || info.State != viewmodel.PendingExpired {
		t.Fatalf("missing-account token = %+v, %v", info, err)
	}
}

// ---- Change password (IDN-C11, IDN-044, IDN-045) ----------------------------

func TestChangePasswordLifecycle(t *testing.T) {
	f := newFixture(t)
	major := f.seedMajor("Logistik", "LOG")
	accountID, _ := f.seedStudent("murid@x.test", "Murid Tukar", 10, major, 1)

	// Two live sessions of this account; the current one survives (IDN-045).
	_, idA := f.seedSession(accountID)
	_, idB := f.seedSession(accountID)
	stranger := f.seedTeacher("status@x.test", "Guru Status", viewmodel.AccountActive)
	_, idS := f.seedSession(stranger)

	newPassword := "SandiBaru99"
	err := f.svc.ChangePassword(f.ctx, accountID, idA, ChangePasswordInput{
		Current: seedPassword, New: newPassword, Confirm: newPassword,
	})
	if err != nil {
		t.Fatalf("change password: %v", err)
	}

	// The new secret works, the old one is dead, must-change cleared (IDN-044).
	if _, err := f.svc.Login(f.ctx, "murid@x.test", newPassword, actorIP(), seedUA); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
	if _, err := f.svc.Login(f.ctx, "murid@x.test", seedPassword, actorIP(), seedUA); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password err = %v, want ErrInvalidCredentials", err)
	}
	if got := f.quBool("SELECT must_change_password = 0 FROM accounts WHERE id = ?", accountID); !got {
		t.Fatal("must_change_password survived the change")
	}

	// Other sessions of this account are revoked; other accounts are not.
	live := func(id int64) bool {
		return f.count("sessions", "WHERE id = ? AND deleted_at = ?", id, store.LiveAt) == 1
	}
	if live(idB) {
		t.Fatal("the other session survived the password change (IDN-045)")
	}
	if !live(idA) || !live(idS) {
		t.Fatalf("current/foreign sessions revoked: %d %d", idA, idS)
	}

	t.Run("wrong current password", func(t *testing.T) {
		err := f.svc.ChangePassword(f.ctx, accountID, idA, ChangePasswordInput{
			Current: "BadCurrent1", New: "LainLagi22", Confirm: "LainLagi22",
		})
		requireIdentical(t, err, ErrWrongPassword, "wrong current")
	})

	t.Run("same-as-current password", func(t *testing.T) {
		err := f.svc.ChangePassword(f.ctx, accountID, idA, ChangePasswordInput{
			Current: newPassword, New: newPassword, Confirm: newPassword,
		})
		requireIdentical(t, err, ErrSamePassword, "same password")
	})

	t.Run("revoked current session", func(t *testing.T) {
		err := f.svc.ChangePassword(f.ctx, accountID, idB, ChangePasswordInput{
			Current: newPassword, New: "SandiKetiga22", Confirm: "SandiKetiga22",
		})
		requireIdentical(t, err, ErrSessionGone, "revoked session")
	})

	t.Run("form errors never touch the database", func(t *testing.T) {
		before := f.stringOf("SELECT password_hash FROM accounts WHERE id = ?", accountID)
		err := f.svc.ChangePassword(f.ctx, accountID, idA, ChangePasswordInput{
			Current: newPassword, New: "weak", Confirm: "weak",
		})
		requireForm(t, err, "new_password", apperr.MsgPasswordPolicy)
		if after := f.stringOf("SELECT password_hash FROM accounts WHERE id = ?", accountID); after != before {
			t.Fatal("weak submission rewrote the hash")
		}
	})

	t.Run("mismatched account/session pair", func(t *testing.T) {
		// A session row of another account paired with this account is the
		// mixed-key fault; the locked session check answers it (IDN-112).
		err := f.svc.ChangePassword(f.ctx, stranger, idA, ChangePasswordInput{
			Current: seedPassword, New: "SandiGanti22", Confirm: "SandiGanti22",
		})
		requireIdentical(t, err, ErrSessionGone, "mismatched pair")
	})
}

// seedSession issues one session row through its own transaction and returns
// the raw token and row id.
func (f *fixture) seedSession(accountID int64) (string, int64) {
	f.t.Helper()
	tx, err := f.db.BeginTx(f.ctx, nil)
	if err != nil {
		f.t.Fatalf("begin seed tx: %v", err)
	}
	token, id, err := f.auth.Issue(f.ctx, tx, accountID, actorIP(), seedUA)
	if err != nil {
		tx.Rollback()
		f.t.Fatalf("issue session for %d: %v", accountID, err)
	}
	tx.Commit()
	return token, id
}

// stringOf is the plain value of a one-row one-column string query.
func (f *fixture) stringOf(source string, args ...any) string {
	f.t.Helper()
	var v string
	if err := f.db.QueryRowContext(f.ctx, source, args...).Scan(&v); err != nil {
		f.t.Fatalf("query %s: %v", source, err)
	}
	return v
}

// ---- Reset password issue + consume (IDN-C20, IDN-C09) ----------------------

func TestResetPasswordLifecycle(t *testing.T) {
	f := newFixture(t)
	teacherID := f.seedTeacher("ketua@x.test", "Ketua Guru", viewmodel.AccountActive)
	secondary := f.seedTeacher("kedua@x.test", "Guru Kedua", viewmodel.AccountActive)
	accountID, studentID := f.seedStudent("murid@x.test", "Murid Reset", 11, f.seedMajor("Logistik", "LOG"), 1)
	_ = accountID

	// A live session the consumption must revoke (IDN-074).
	_, sessID := f.seedSession(accountID)

	first, err := f.svc.IssueReset(f.ctx, teacherID, studentID)
	if err != nil {
		t.Fatalf("issue reset: %v", err)
	}
	if first.StudentID != studentID || first.Token == "" || first.ExpiresAt.IsZero() {
		t.Fatalf("issued reset = %+v", first)
	}

	t.Run("the token is attributed to its issuer", func(t *testing.T) {
		other, err := f.svc.IssueReset(f.ctx, secondary, studentID)
		if err != nil {
			t.Fatalf("secondary issue: %v", err)
		}
		issuer := f.qustring(`
			SELECT issued_by_account_id FROM password_reset_tokens
			WHERE token_hash = ? AND deleted_at = ?`,
			secure.HashToken(other.Token), store.LiveAt)
		if issuer.String != fmt.Sprintf("%d", secondary) {
			t.Fatalf("issuer = %q, want %d", issuer.String, secondary)
		}
	})

	// Issuing again supersedes the earlier token (IDN-C20).
	second, err := f.svc.IssueReset(f.ctx, teacherID, studentID)
	if err != nil {
		t.Fatalf("reissue: %v", err)
	}
	if err := f.svc.ResetPassword(f.ctx, first.Token, "SandiBaru88", "SandiBaru88"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("superseded token err = %v, want ErrTokenInvalid", err)
	}

	// An expired-but-never-used token row is invalid, not used (IDN-C09).
	expiredRaw := "expired-token-raw-value"
	f.exec(`INSERT INTO password_reset_tokens
		(student_id, issued_by_account_id, token_hash, expires_at, created_at)
		VALUES (?, ?, ?, UTC_TIMESTAMP() - INTERVAL 1 HOUR, UTC_TIMESTAMP() - INTERVAL 25 HOUR)`,
		studentID, teacherID, secure.HashToken(expiredRaw))
	if err := f.svc.ResetPassword(f.ctx, expiredRaw, "SandiBaru88", "SandiBaru88"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expired token err = %v, want ErrTokenInvalid", err)
	}

	if err := f.svc.ResetPassword(f.ctx, second.Token, "SandiBaru88", "SandiBaru88"); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if got := f.quBool(`SELECT a.must_change_password = 0
		FROM password_reset_tokens t
		JOIN students s ON s.id = t.student_id
		JOIN accounts a ON a.id = s.account_id
		WHERE t.token_hash = ?`, secure.HashToken(second.Token)); !got {
		t.Fatal("must_change_password survived the reset (IDN-044)")
	}
	if used := f.qustring("SELECT IFNULL(used_at, '') FROM password_reset_tokens WHERE token_hash = ?",
		secure.HashToken(second.Token)); used.String == "" {
		t.Fatal("token not marked used after consumption")
	}
	if gone := f.count("sessions", "WHERE id = ? AND deleted_at <> ?", sessID, store.LiveAt); gone != 1 {
		t.Fatal("the student's session survived consumption (IDN-074)")
	}

	t.Run("replay is token_used", func(t *testing.T) {
		err := f.svc.ResetPassword(f.ctx, second.Token, "SandiKedua88", "SandiKedua88")
		requireIdentical(t, err, ErrTokenUsed, "replay")
	})

	t.Run("unknown token is invalid", func(t *testing.T) {
		err := f.svc.ResetPassword(f.ctx, "no-such-token-at-all", "SandiKedua88", "SandiKedua88")
		requireIdentical(t, err, ErrTokenInvalid, "unknown token")
	})

	t.Run("form errors are field-scoped", func(t *testing.T) {
		requireForm(t, f.svc.ResetPassword(f.ctx, "  ", "SandiKedua88", "SandiKedua88"),
			"token", MsgTokenRequired)
		requireForm(t, f.svc.ResetPassword(f.ctx, strings.Repeat("z", MaxTokenLength+1), "SandiKedua88", "SandiKedua88"),
			"token", apperr.MsgInvalidToken)
		requireForm(t, f.svc.ResetPassword(f.ctx, "tok", "weak", "weak"),
			"password", apperr.MsgPasswordPolicy)
	})

	// The password works and the temporary flag is gone (IDN-044).
	if _, err := f.svc.Login(f.ctx, "murid@x.test", "SandiBaru88", actorIP(), seedUA); err != nil {
		t.Fatalf("post-reset login: %v", err)
	}
}

// ---- Roster (IDN-091 … 094, IDN-105, IDN-121) --------------------------------

func TestRosterLoadingAndFilters(t *testing.T) {
	f := newFixture(t)
	teacher := f.seedTeacher("ketua@x.test", "Ketua Guru", viewmodel.AccountActive)
	other := f.seedTeacher("roster-other@x.test", "Guru Lain", viewmodel.AccountActive)

	majorRPL := f.seedMajor("Rekayasa Perangkat Lunak", "RPL")
	majorIPA := f.seedMajor("Ilmu Pengetahuan Alam", "IPA")
	majorDead := f.seedMajor("Jurusan Tua", "TU")

	type seed struct {
		email, name string
		grade       int
		major       int64
	}
	students := []seed{
		{"s01@x.test", "andi satu", 10, majorRPL},
		{"s02@x.test", "budi dua", 11, majorRPL},
		{"s03@x.test", "citra tiga", 12, majorIPA},
		{"s04@x.test", "dodi empat", 10, majorIPA},
		{"s05@x.test", "eka lima", 11, majorRPL},
		{"s06@x.test", "fajar enam", 12, majorIPA},
		{"s07@x.test", "gita tujuh", 12, majorIPA},
		{"s08@x.test", "hadi delapan", 12, majorIPA},
		{"s09@x.test", "murid layar", 12, majorIPA},
		{"s10@x.test", "murid bercahaya", 12, majorIPA},
		{"s11@x.test", "nia sembilan", 12, majorIPA},
		{"pct@x.test", "siswa 100% energi", 10, majorRPL},
		{"udr@x.test", "a__b khusus", 11, majorRPL},
		{"drift@x.test", "0 pengapung", 12, majorDead},
		{"zzz@x.test", "zzz selesai", 11, majorIPA},
	}
	_, driftSt := f.seedStudent(students[13].email, students[13].name, students[13].grade, students[13].major, 0)
	for _, s := range students {
		if s.email == "drift@x.test" {
			continue
		}
		f.seedStudent(s.email, s.name, s.grade, s.major, 0)
	}

	// Distinct created_at stamps make the pinned orders observable; the
	// sequence matches the seed list, so created order == id order.
	for i, s := range students {
		day := i + 1
		_ = day
		f.exec("UPDATE accounts SET created_at = DATE_ADD('2026-01-01', INTERVAL ? DAY) WHERE email = ?",
			i, s.email)
	}
	// The drift student's major dies after the roster exists (IDN-121/IDN-090).
	f.exec("UPDATE majors SET deleted_at = UTC_TIMESTAMP() WHERE id = ?", majorDead)

	t.Run("unfiltered first page", func(t *testing.T) {
		list, err := f.svc.Students(f.ctx, RosterInput{ActorID: teacher})
		if err != nil {
			t.Fatalf("roster: %v", err)
		}
		if list.Students.Total != 15 || list.Students.Page != 1 || list.Pagination.Pages != 2 {
			t.Fatalf("unfiltered = total %d page %d pages %d, want 15/1/2",
				list.Students.Total, list.Students.Page, list.Pagination.Pages)
		}
		if len(list.Students.Items) != 10 || len(list.Majors) != 2 {
			t.Fatalf("first page rows = %d majors = %d, want 10/2",
				len(list.Students.Items), len(list.Majors))
		}
		for _, row := range list.Students.Items {
			want := fmt.Sprintf("/teacher/students/%d", row.ID)
			if row.EditHref != want || row.ResetHref != want+"/reset-token" || row.DeleteHref != want+"/delete" {
				t.Fatalf("hrefs for %d: %q %q %q", row.ID, row.EditHref, row.ResetHref, row.DeleteHref)
			}
		}
	})

	t.Run("drifted major renders the dash", func(t *testing.T) {
		list, err := f.svc.Students(f.ctx, RosterInput{Query: "pengapung", ActorID: teacher})
		if err != nil || len(list.Students.Items) != 1 {
			t.Fatalf("drift search = %d rows, %v", len(list.Students.Items), err)
		}
		if got := list.Students.Items[0].ClassLabel; got != viewmodel.ClassLabel(0, "") {
			t.Fatalf("drifted class label = %q, want %q", got, viewmodel.ClassLabel(0, ""))
		}
	})

	t.Run("grade filter is exact", func(t *testing.T) {
		list, err := f.svc.Students(f.ctx, RosterInput{Grade: 11, ActorID: teacher})
		if err != nil {
			t.Fatalf("grade filter: %v", err)
		}
		for _, row := range list.Students.Items {
			if row.GradeLevel != 11 {
				t.Fatalf("grade filter leaked grade %d", row.GradeLevel)
			}
		}
		if list.Students.Total != 4 {
			t.Fatalf("grade 11 total = %d, want 4", list.Students.Total)
		}
	})

	t.Run("major filter only live majors", func(t *testing.T) {
		list, err := f.svc.Students(f.ctx, RosterInput{MajorID: majorRPL, ActorID: teacher})
		if err != nil {
			t.Fatalf("major filter: %v", err)
		}
		if list.Students.Total != 5 {
			t.Fatalf("RPL total = %d, want 5", list.Students.Total)
		}
		// A dead major id silently degrades to no filter (IDN-091).
		all, err := f.svc.Students(f.ctx, RosterInput{MajorID: majorDead, ActorID: teacher})
		if err != nil || all.Students.Total != 15 {
			t.Fatalf("dead major filter total = %d, %v", all.Students.Total, err)
		}
	})

	t.Run("search escapes LIKE wildcards", func(t *testing.T) {
		tests := map[string]int{
			"100%":  1,
			"_":     1,
			"__":    1,
			"murid": 2,
			"zzz":   1,
			"0%":    1,
		}
		for q, want := range tests {
			list, err := f.svc.Students(f.ctx, RosterInput{Query: q, ActorID: teacher})
			if err != nil {
				t.Fatalf("search %q: %v", q, err)
			}
			if list.Students.Total != want {
				t.Fatalf("search %q total = %d, want %d", q, list.Students.Total, want)
			}
		}
	})

	t.Run("pinned orders", func(t *testing.T) {
		newest, err := f.svc.Students(f.ctx, RosterInput{Sort: viewmodel.SortNewest, ActorID: teacher})
		if err != nil {
			t.Fatalf("sort newest: %v", err)
		}
		if got := newest.Students.Items[0].Email; got != "zzz@x.test" {
			t.Fatalf("newest first row = %q, want zzz (latest created_at)", got)
		}
		oldest, err := f.svc.Students(f.ctx, RosterInput{Sort: viewmodel.SortOldest, ActorID: teacher})
		if err != nil {
			t.Fatalf("sort oldest: %v", err)
		}
		if got := oldest.Students.Items[0].Email; got != "s01@x.test" {
			t.Fatalf("oldest first row = %q, want s01", got)
		}
		// Digit-leading names sort before letters in both ASCII and the
		// case-insensitive database collation.
		ascending, err := f.svc.Students(f.ctx, RosterInput{Sort: viewmodel.SortNameAsc, ActorID: teacher})
		if err != nil {
			t.Fatalf("sort name_asc: %v", err)
		}
		if got := ascending.Students.Items[0].FullName; got != "0 pengapung" {
			t.Fatalf("name_asc first = %q, want the digit-leading row", got)
		}
		descending, err := f.svc.Students(f.ctx, RosterInput{Sort: viewmodel.SortNameDesc, ActorID: teacher})
		if err != nil {
			t.Fatalf("sort name_desc: %v", err)
		}
		if got := descending.Students.Items[0].FullName; got != "zzz selesai" {
			t.Fatalf("name_desc first = %q, want zzz", got)
		}
	})

	t.Run("clamped pagination and empty states", func(t *testing.T) {
		list, err := f.svc.Students(f.ctx, RosterInput{Page: 99, ActorID: teacher})
		if err != nil {
			t.Fatalf("page 99: %v", err)
		}
		if list.Students.Page != 2 || len(list.Students.Items) != 5 {
			t.Fatalf("clamped page = %d rows %d, want page 2 with 5 rows", list.Students.Page, len(list.Students.Items))
		}
		if !list.Pagination.Show() || list.Pagination.NextHref != "" {
			t.Fatalf("last-page pagination = %+v", list.Pagination)
		}
		empty, err := f.svc.Students(f.ctx, RosterInput{Query: "nothingmatches", ActorID: teacher})
		if err != nil || empty.Students.Total != 0 || len(empty.Students.Items) != 0 {
			t.Fatalf("empty search = %+v %v", empty, err)
		}
		if empty.Empty.Action == nil {
			t.Fatal("filtered empty state lost its clear action (IDN-092)")
		}
		unfiltered, _ := f.svc.Students(f.ctx, RosterInput{ActorID: teacher})
		if unfiltered.Empty.Action != nil {
			t.Fatal("unfiltered empty state must not offer clear")
		}
	})

	t.Run("filter values echo into links", func(t *testing.T) {
		list, err := f.svc.Students(f.ctx, RosterInput{
			Query: "murid", Grade: 11, Sort: viewmodel.SortNameAsc, ActorID: teacher,
		})
		if err != nil {
			t.Fatalf("filtered roster: %v", err)
		}
		for _, tab := range list.Tabs {
			if !strings.Contains(tab.Href, "q=murid") {
				t.Fatalf("tab lost the query: %q", tab.Href)
			}
		}
		if list.Query.Q != "murid" || list.Query.Sort != viewmodel.SortNameAsc {
			t.Fatalf("query state = %+v", list.Query)
		}
		// Bare links only carry the page key (IDN-108).
		bare, _ := f.svc.Students(f.ctx, RosterInput{ActorID: teacher})
		if got := bare.Pagination.Numbers[1].Href; got != "/teacher/students?page=2" {
			t.Fatalf("bare page-2 href = %q", got)
		}
		if got := bare.Pagination.Numbers[0].Href; got != "/teacher/students" {
			t.Fatalf("bare page-1 href = %q", got)
		}
	})

	t.Run("reveal panel revalidates ownership and use", func(t *testing.T) {
		issued, err := f.svc.IssueReset(f.ctx, teacher, driftSt)
		if err != nil {
			t.Fatalf("issue for reveal: %v", err)
		}
		reveal := &Reveal{Teacher: teacher, Student: driftSt, Token: issued.Token, ExpiresAt: issued.ExpiresAt.Unix()}

		// Another teacher's cookie is not this list's panel.
		visible, err := f.svc.Students(f.ctx, RosterInput{ActorID: other, Reveal: reveal})
		if err != nil {
			t.Fatalf("foreign reveal: %v", err)
		}
		if visible.ResetToken != nil {
			t.Fatalf("foreign reveal surfaced %+v", visible.ResetToken)
		}

		mine, err := f.svc.Students(f.ctx, RosterInput{ActorID: teacher, Reveal: reveal})
		if err != nil {
			t.Fatalf("own reveal: %v", err)
		}
		if mine.ResetToken == nil || mine.ResetToken.Token != issued.Token {
			t.Fatalf("own reveal = %+v, want token %q", mine.ResetToken, issued.Token)
		}

		// After consumption the same cookie is no panel (IDN-105 one-shot).
		if err := f.svc.ResetPassword(f.ctx, issued.Token, "SandiPanel22", "SandiPanel22"); err != nil {
			t.Fatalf("consume revealed token: %v", err)
		}
		again, err := f.svc.Students(f.ctx, RosterInput{ActorID: teacher, Reveal: reveal})
		if err != nil {
			t.Fatalf("used reveal: %v", err)
		}
		if again.ResetToken != nil {
			t.Fatalf("used token surfaced %+v", again.ResetToken)
		}
	})
}

// ---- Student edit (IDN-C17/C18) ---------------------------------------------

func TestStudentFormAndUpdate(t *testing.T) {
	f := newFixture(t)
	majorRPL := f.seedMajor("Rekayasa Perangkat Lunak", "RPL")
	majorTKJ := f.seedMajor("Teknik Komputer Jaringan", "TKJ")
	f.seedStudent("kawan@x.test", "Kawan Edit", 10, majorRPL, 0)
	_, stID := f.seedStudent("murid@x.test", "Murid Edit", 10, majorRPL, 0)

	form, err := f.svc.StudentForm(f.ctx, stID)
	if err != nil {
		t.Fatalf("student form: %v", err)
	}
	if form.FullName != "Murid Edit" || form.Email != "murid@x.test" ||
		form.GradeLevel != 10 || form.MajorID != majorRPL || form.ID != stID {
		t.Fatalf("form = %+v", form)
	}
	if len(form.Majors) != 2 || form.Action != fmt.Sprintf("/teacher/students/%d", stID) ||
		form.CancelHref != "/teacher/students" || form.IsNew {
		t.Fatalf("form chrome = %+v", form)
	}

	t.Run("missing and dead ids are NotFound", func(t *testing.T) {
		for _, bad := range []int64{0, -1, 999999} {
			if _, err := f.svc.StudentForm(f.ctx, bad); !errors.Is(err, ErrNotFound) {
				t.Fatalf("StudentForm(%d) err = %v, want ErrNotFound", bad, err)
			}
		}
	})

	t.Run("a dead major is not offered", func(t *testing.T) {
		f.exec("UPDATE majors SET deleted_at = UTC_TIMESTAMP() WHERE id = ?", majorTKJ)
		form, err := f.svc.StudentForm(f.ctx, stID)
		if err != nil {
			t.Fatalf("form after major death: %v", err)
		}
		if len(form.Majors) != 1 || form.Majors[0].ID != majorRPL {
			t.Fatalf("dead major still offered: %+v", form.Majors)
		}
	})

	t.Run("happy edit", func(t *testing.T) {
		err := f.svc.UpdateStudent(f.ctx, stID, StudentInput{
			FullName: "Murid Baru", Email: "baru@x.test",
			Grade: "12", MajorID: fmt.Sprintf("%d", majorRPL),
		})
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		form, err := f.svc.StudentForm(f.ctx, stID)
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		if form.FullName != "Murid Baru" || form.Email != "baru@x.test" ||
			form.GradeLevel != 12 || form.MajorID != majorRPL {
			t.Fatalf("edit did not land: %+v", form)
		}
	})

	t.Run("conflicts", func(t *testing.T) {
		otherSt := f.studentIDOf("kawan@x.test")
		if err := f.svc.UpdateStudent(f.ctx, otherSt, StudentInput{
			FullName: "Kawan Edit", Email: "baru@x.test",
			Grade: "10", MajorID: fmt.Sprintf("%d", majorRPL),
		}); !errors.Is(err, ErrEmailTaken) {
			t.Fatalf("email clash err = %v, want ErrEmailTaken", err)
		}
		if err := f.svc.UpdateStudent(f.ctx, stID, StudentInput{
			FullName: "Murid Baru", Email: "baru2@x.test",
			Grade: "12", MajorID: "888888",
		}); !errors.Is(err, ErrMajorRequired) {
			t.Fatalf("dead major err = %v, want ErrMajorRequired", err)
		}
		if err := f.svc.UpdateStudent(f.ctx, 999999, StudentInput{
			FullName: "Siapa", Email: "siapa@x.test", Grade: "10", MajorID: "1",
		}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("missing target err = %v, want ErrNotFound", err)
		}
		if err := f.svc.UpdateStudent(f.ctx, stID, StudentInput{
			FullName: "", Email: "bad email", Grade: "13", MajorID: "",
		}); err == nil {
			t.Fatal("invalid inputs accepted")
		}
	})
}

// ---- Profile (IDN-C14/C15) ---------------------------------------------------

func TestProfileLoadAndSave(t *testing.T) {
	f := newFixture(t)
	major := f.seedMajor("Logistik", "LOG")
	accountID, _ := f.seedStudent("murid@x.test", "Murid Profil", 11, major, 0)

	page, err := f.svc.Profile(f.ctx, accountID)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	if page.FullName != "Murid Profil" || page.Email != "murid@x.test" ||
		page.GradeLevel != 11 || page.MajorID != major || len(page.Majors) != 1 {
		t.Fatalf("profile page = %+v", page)
	}

	t.Run("save name and email", func(t *testing.T) {
		if err := f.svc.SaveProfile(f.ctx, accountID, ProfileInput{
			FullName: "Murid Diubah", Email: "Baru@X.test",
		}); err != nil {
			t.Fatalf("save: %v", err)
		}
		if name := f.stringOf("SELECT full_name FROM accounts WHERE id = ?", accountID); name != "Murid Diubah" {
			t.Fatalf("saved name = %q", name)
		}
		if email := f.stringOf("SELECT email FROM accounts WHERE id = ?", accountID); email != "baru@x.test" {
			t.Fatalf("saved email = %q", email)
		}
		if grade := f.stringOf("SELECT grade_level FROM students WHERE account_id = ?", accountID); grade != "11" ||
			f.stringOf("SELECT major_id FROM students WHERE account_id = ?", accountID) != fmt.Sprintf("%d", major) {
			t.Fatal("profile save touched grade/major")
		}
	})

	t.Run("email conflict", func(t *testing.T) {
		f.seedStudent("lain@x.test", "Murid Lain", 10, major, 0)
		err := f.svc.SaveProfile(f.ctx, accountID, ProfileInput{
			FullName: "Murid Diubah", Email: "lain@x.test",
		})
		requireForm(t, err, "email", apperr.MsgEmailTaken)
	})

	t.Run("deleted account is session_expired", func(t *testing.T) {
		other := f.seedTeacher("hapus@x.test", "Guru Penghapus", viewmodel.AccountActive)
		f.softDeleteAccount(other, accountID)
		err := f.svc.SaveProfile(f.ctx, accountID, ProfileInput{FullName: "X", Email: "x@x.test"})
		requireIdentical(t, err, ErrSessionGone, "deleted account")
	})

	t.Run("nil pool is an internal fault", func(t *testing.T) {
		demo := New(nil, f.signer, f.auth)
		if _, err := demo.Profile(f.ctx, accountID); err == nil {
			t.Fatal("nil pool profile must fail, not panic")
		}
		if _, err := demo.Students(f.ctx, RosterInput{}); err == nil {
			t.Fatal("nil pool roster must fail, not panic")
		}
		if err := demo.SaveProfile(f.ctx, accountID, ProfileInput{FullName: "A", Email: "a@x.test"}); err == nil {
			t.Fatal("nil pool save must fail, not panic")
		}
	})
}

// ---- Delete student (IDN-C19, IDN-140, IDN-147) -----------------------------

func TestDeleteStudentCascade(t *testing.T) {
	f := newFixture(t)
	teacher := f.seedTeacher("ketua@x.test", "Ketua Guru", viewmodel.AccountActive)
	major := f.seedMajor("Logistik", "LOG")
	_, victim := f.seedStudent("korban@x.test", "Murid Korban", 11, major, 0)
	_, neighbour := f.seedStudent("tetangga@x.test", "Murid Tetangga", 11, major, 0)

	// Full attempt trees for both students on a shared live quiz.
	quiz := f.seedQuiz(teacher, teacher, "Kuis Korban", "123456", "invite-victim")
	question := f.seedQuestion(quiz, 1, "Soal korban", "essay")
	option := f.seedOption(question, 1, "Pilihan A")
	regV := f.seedRegistration(quiz, victim)
	regN := f.seedRegistration(quiz, neighbour)
	attV := f.seedAttempt(regV, 1)
	attN := f.seedAttempt(regN, 1)
	ansV := f.seedAnswer(attV, question, "Jawaban korban")
	ansN := f.seedAnswer(attN, question, "Jawaban tetangga")
	f.seedSelection(ansV, option)
	f.seedSelection(ansN, option)
	f.seedFocus(attV, "blur")

	// The victim's extra surfaces: a live session and an outstanding token.
	victimAcc := f.studentAccountOf(victim)
	_, victimSession := f.seedSession(victimAcc)
	if _, err := f.svc.IssueReset(f.ctx, teacher, victim); err != nil {
		t.Fatalf("issue victim token: %v", err)
	}

	t.Run("missing ids are NotFound", func(t *testing.T) {
		for _, bad := range []int64{0, -2, 987654} {
			if err := f.svc.DeleteStudent(f.ctx, teacher, bad); !errors.Is(err, ErrNotFound) {
				t.Fatalf("DeleteStudent(%d) = %v, want ErrNotFound", bad, err)
			}
		}
	})

	if err := f.svc.DeleteStudent(f.ctx, teacher, victim); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// One database timestamp stamps the whole cascade (IDN-140).
	var ts time.Time
	if err := f.db.QueryRowContext(f.ctx, `
		SELECT deleted_at FROM students WHERE id = ?`, victim).Scan(&ts); err != nil {
		t.Fatalf("victim students stamp: %v", err)
	}
	if ts.Equal(store.LiveAtTime) || ts.IsZero() {
		t.Fatalf("students stamp = %v", ts)
	}
	victimRows := []string{
		fmt.Sprintf("SELECT deleted_at FROM accounts WHERE id = %d", victimAcc),
		fmt.Sprintf("SELECT deleted_at FROM sessions WHERE id = %d", victimSession),
		fmt.Sprintf("SELECT deleted_at FROM password_reset_tokens WHERE student_id = %d AND deleted_at <> '%s' LIMIT 1", victim, store.LiveAt),
		fmt.Sprintf("SELECT deleted_at FROM registrations WHERE id = %d", regV),
		fmt.Sprintf("SELECT deleted_at FROM quiz_attempts WHERE id = %d", attV),
		fmt.Sprintf("SELECT deleted_at FROM attempt_answers WHERE id = %d", ansV),
		fmt.Sprintf("SELECT deleted_at FROM attempt_answer_selections WHERE attempt_answer_id = %d", ansV),
		fmt.Sprintf("SELECT deleted_at FROM focus_loss_events WHERE attempt_id = %d", attV),
	}
	for i, source := range victimRows {
		var at time.Time
		if err := f.db.QueryRowContext(f.ctx, source).Scan(&at); err != nil {
			t.Fatalf("cascade stamp %d (%s): %v", i, source, err)
		}
		if !at.Equal(ts) {
			t.Fatalf("cascade stamp %d = %v, want the shared %v (IDN-140)", i, at, ts)
		}
	}
	if !f.quBool(`SELECT deleted_by_account_id IS NOT NULL AND deleted_by_account_id = ?
		FROM accounts WHERE id = ? AND DELETED_AT <> ?`, teacher, victimAcc, store.LiveAt) {
		t.Fatal("acting teacher must be recorded (IDN-139)")
	}

	// The neighbour's entire tree is untouched (IDN-147).
	neighbourRows := []string{
		fmt.Sprintf("SELECT deleted_at FROM students WHERE id = %d", neighbour),
		fmt.Sprintf("SELECT deleted_at FROM registrations WHERE id = %d", regN),
		fmt.Sprintf("SELECT deleted_at FROM quiz_attempts WHERE id = %d", attN),
		fmt.Sprintf("SELECT deleted_at FROM attempt_answers WHERE id = %d", ansN),
		fmt.Sprintf("SELECT deleted_at FROM attempt_answer_selections WHERE attempt_answer_id = %d", ansN),
	}
	for i, source := range neighbourRows {
		var at time.Time
		if err := f.db.QueryRowContext(f.ctx, source).Scan(&at); err != nil {
			t.Fatalf("neighbour stamp %d: %v", i, err)
		}
		if !at.Equal(store.LiveAtTime) {
			t.Fatalf("neighbour row carried %v (IDN-147 violation)", at)
		}
	}

	// Idempotence: a second delete is NotFound.
	if err := f.svc.DeleteStudent(f.ctx, teacher, victim); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete err = %v, want ErrNotFound", err)
	}

	// The roster no longer lists the victim (IDN-097), and the teacher edit
	// form answers 404 for the deleted students.id.
	list, err := f.svc.Students(f.ctx, RosterInput{ActorID: teacher})
	if err != nil {
		t.Fatalf("roster after delete: %v", err)
	}
	for _, row := range list.Students.Items {
		if row.ID == victim {
			t.Fatal("deleted student still on the roster")
		}
	}
	if _, err := f.svc.StudentForm(f.ctx, victim); !errors.Is(err, ErrNotFound) {
		t.Fatalf("edit form for deleted student = %v, want ErrNotFound", err)
	}
}

// The seed helpers below build the minimum authoring/participation rows the
// cascade needs; they live here because the cascade, not authoring, is what
// this test certifies.

func (f *fixture) seedQuiz(owner, editor int64, title, code, invite string) int64 {
	f.t.Helper()
	return f.id(f.exec(`INSERT INTO quizzes
		(owner_account_id, updated_by_account_id, title, visibility, state, code, invite_token)
		VALUES (?, ?, ?, 'public', 'draft', ?, ?)`, owner, editor, title, code, invite),
		"quiz "+title)
}

func (f *fixture) seedQuestion(quizID int64, position int, prompt, kind string) int64 {
	f.t.Helper()
	return f.id(f.exec(`INSERT INTO questions (quiz_id, position, type, prompt)
		VALUES (?, ?, ?, ?)`, quizID, position, kind, prompt), "question")
}

func (f *fixture) seedOption(questionID int64, position int, body string) int64 {
	f.t.Helper()
	return f.id(f.exec(`INSERT INTO answer_options (question_id, position, body, is_correct)
		VALUES (?, ?, ?, 1)`, questionID, position, body), "option")
}

func (f *fixture) seedRegistration(quizID, studentID int64) int64 {
	f.t.Helper()
	return f.id(f.exec(`INSERT INTO registrations (quiz_id, student_id) VALUES (?, ?)`,
		quizID, studentID), "registration")
}

func (f *fixture) seedAttempt(registrationID int64, n int) int64 {
	f.t.Helper()
	return f.id(f.exec(`INSERT INTO quiz_attempts (registration_id, attempt_no, shuffle_seed)
		VALUES (?, ?, ?)`, registrationID, n, 1), "attempt")
}

func (f *fixture) seedAnswer(attemptID, questionID int64, text string) int64 {
	f.t.Helper()
	return f.id(f.exec(`INSERT INTO attempt_answers (attempt_id, question_id, answer_text)
		VALUES (?, ?, ?)`, attemptID, questionID, text), "answer")
}

func (f *fixture) seedSelection(answerID, optionID int64) int64 {
	f.t.Helper()
	return f.id(f.exec(`INSERT INTO attempt_answer_selections (attempt_answer_id, option_id)
		VALUES (?, ?)`, answerID, optionID), "selection")
}

func (f *fixture) seedFocus(attemptID int64, kind string) int64 {
	f.t.Helper()
	return f.id(f.exec(`INSERT INTO focus_loss_events (attempt_id, kind) VALUES (?, ?)`,
		attemptID, kind), "focus event")
}
