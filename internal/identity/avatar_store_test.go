package identity

// Tests for the avatar store (design §4.2, §11 "Identity store"). The
// commit/remove tests need the real schema, the unique key, and real
// transaction semantics, so they build on the same fixture as
// integration_test.go and skip without TEST_DB_DSN. The grammar and
// derivation tests need no database at all.

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// testAvatarName builds a valid generated name (AVT-010) from one repeated
// hex digit, so each call site's names are distinct by construction.
func testAvatarName(tag string) string {
	return strings.Repeat(tag, 32) + ".webp"
}

// writeAvatarSrc stages the bytes a conversion worker would have produced
// and returns that temp file's path — the srcPath Commit moves.
func writeAvatarSrc(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "produced.webp")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return path
}

// requireAvatarFile asserts the stored photo's bytes and leaves the check of
// the mode to the caller (Windows exposes no POSIX modes).
func requireAvatarFile(t *testing.T, dir, name, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read stored %s: %v", name, err)
	}
	if string(got) != want {
		t.Fatalf("stored %s = %q, want %q", name, got, want)
	}
}

// requireNoFile asserts one path does not exist.
func requireNoFile(t *testing.T, path, what string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s still exists (stat err %v)", what, err)
	}
}

// requireNoTemps asserts no dot-prefixed temp file survives in the avatars
// directory (§11: "No dot-prefixed temp file survives any failure path").
func requireNoTemps(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read avatars dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Errorf("leftover temp file %q", e.Name())
		}
	}
}

// ---- Grammar and construction (no database) ---------------------------------

func TestAvatarNameGrammar(t *testing.T) {
	uploads := t.TempDir()
	avatars := NewAvatarStore(nil, uploads)
	src := writeAvatarSrc(t, "webp-bytes")

	bad := []string{
		"",
		"avatar",
		"photo.webp",
		"g" + strings.Repeat("a", 31) + ".webp",   // not hex
		strings.Repeat("A", 32) + ".webp",         // uppercase hex
		strings.Repeat("a", 31) + ".webp",         // one short
		strings.Repeat("a", 33) + ".webp",         // one long
		strings.Repeat("a", 32) + ".png",          // wrong extension
		strings.Repeat("a", 32) + ".WEBP",         // wrong case
		strings.Repeat("a", 32) + ".webp ",        // trailing space
		"../" + strings.Repeat("a", 32) + ".webp", // traversal
		strings.Repeat("a", 32) + "/x.webp",       // separator
	}
	for _, name := range bad {
		if err := avatars.Commit(context.Background(), 1, src, name); !errors.Is(err, ErrAvatarName) {
			t.Errorf("Commit(%q) err = %v, want ErrAvatarName", name, err)
		}
	}

	// Validation runs before any I/O: no directory, no temp, source intact.
	if _, err := os.Stat(filepath.Join(uploads, avatarDirName)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("avatars dir exists after rejected names: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source consumed by a rejected name: %v", err)
	}

	// A valid name on a store with no pool answers 500, never panics — the
	// nil-pool tolerance every service in this package has.
	err := avatars.Commit(context.Background(), 1, src, testAvatarName("1"))
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Kind() != apperr.Internal {
		t.Errorf("nil-pool commit err = %v, want internal", err)
	}
	if err := avatars.Remove(context.Background(), 1); err == nil {
		t.Error("nil-pool remove succeeded, want internal fault")
	}
	// A malformed serving lookup answers before the pool is touched.
	if ok, err := avatars.HasLiveOwner(context.Background(), "not-a-name"); err != nil || ok {
		t.Errorf("HasLiveOwner(malformed) = %v, %v, want false, nil", ok, err)
	}
}

func TestAvatarDirDerivation(t *testing.T) {
	avatars := NewAvatarStore(nil, filepath.Join("srv", "uploads"))
	want := filepath.Join("srv", "uploads", avatarDirName)
	if got := avatars.Dir(); got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}
}

// ---- Commit: replace order (§4.2) -------------------------------------------

func TestAvatarCommitReplacesPhoto(t *testing.T) {
	f := newFixture(t)
	major := f.seedMajor("Logistik", "LOG")
	accountID, _ := f.seedStudent("murid@x.test", "Murid Foto", 10, major, 0)
	avatars := NewAvatarStore(f.db, t.TempDir())
	dir := avatars.Dir()

	first, second := testAvatarName("1"), testAvatarName("2")

	// First commit: file, column, and mode agree.
	if err := avatars.Commit(f.ctx, accountID, writeAvatarSrc(t, "first-bytes"), first); err != nil {
		t.Fatalf("first commit: %v", err)
	}
	if got := f.stringOf("SELECT COALESCE(avatar_name, '') FROM accounts WHERE id = ?", accountID); got != first {
		t.Fatalf("avatar_name = %q, want %q", got, first)
	}
	requireAvatarFile(t, dir, first, "first-bytes")
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, first))
		if err != nil {
			t.Fatalf("stat stored file: %v", err)
		}
		if perm := info.Mode().Perm(); perm != avatarFileMode {
			t.Fatalf("stored mode = %o, want %o", perm, avatarFileMode)
		}
	}
	requireNoTemps(t, dir)

	// Replacement: the column moves to the new name and the old file is
	// unlinked only after that commit.
	if err := avatars.Commit(f.ctx, accountID, writeAvatarSrc(t, "second-bytes"), second); err != nil {
		t.Fatalf("second commit: %v", err)
	}
	if got := f.stringOf("SELECT COALESCE(avatar_name, '') FROM accounts WHERE id = ?", accountID); got != second {
		t.Fatalf("avatar_name after replace = %q, want %q", got, second)
	}
	requireAvatarFile(t, dir, second, "second-bytes")
	requireNoFile(t, filepath.Join(dir, first), "replaced file "+first)
	requireNoTemps(t, dir)
}

// ---- Commit: a failed transaction removes the new file ----------------------

func TestAvatarCommitFailureKeepsOldPhoto(t *testing.T) {
	f := newFixture(t)
	major := f.seedMajor("Logistik", "LOG")
	accountID, _ := f.seedStudent("murid@x.test", "Murid Gagal", 10, major, 0)
	other := f.seedTeacher("kedua@x.test", "Guru Kedua", viewmodel.AccountActive)
	avatars := NewAvatarStore(f.db, t.TempDir())
	dir := avatars.Dir()

	first, claimed := testAvatarName("1"), testAvatarName("2")
	if err := avatars.Commit(f.ctx, accountID, writeAvatarSrc(t, "old-bytes"), first); err != nil {
		t.Fatalf("seed commit: %v", err)
	}

	// Another live account claims the next name, so the update trips
	// uq_accounts_avatar from migration 0010 and the transaction rolls back.
	f.exec("UPDATE accounts SET avatar_name = ? WHERE id = ?", claimed, other)

	err := avatars.Commit(f.ctx, accountID, writeAvatarSrc(t, "new-bytes"), claimed)
	if err == nil {
		t.Fatal("commit against a taken name succeeded, want a failure")
	}
	if !store.IsDuplicate(err) {
		t.Fatalf("err = %v, want the unique-key duplicate", err)
	}
	if got := f.stringOf("SELECT COALESCE(avatar_name, '') FROM accounts WHERE id = ?", accountID); got != first {
		t.Fatalf("avatar_name after failed commit = %q, want the old %q", got, first)
	}
	requireNoFile(t, filepath.Join(dir, claimed), "uncommitted file "+claimed)
	requireAvatarFile(t, dir, first, "old-bytes")
	requireNoTemps(t, dir)

	// A missing account fails after the file was staged: the new file is
	// removed and the column it would have written never existed.
	orphan := testAvatarName("3")
	err = avatars.Commit(f.ctx, 999999, writeAvatarSrc(t, "doomed-bytes"), orphan)
	if !errors.Is(err, ErrSessionGone) {
		t.Fatalf("missing-account commit err = %v, want ErrSessionGone", err)
	}
	requireNoFile(t, filepath.Join(dir, orphan), "file of the failed commit "+orphan)
	requireAvatarFile(t, dir, first, "old-bytes")
	requireNoTemps(t, dir)
}

// ---- Remove: clear, unlink, idempotence -------------------------------------

func TestAvatarRemoveClearsAndUnlinks(t *testing.T) {
	f := newFixture(t)
	major := f.seedMajor("Logistik", "LOG")
	accountID, _ := f.seedStudent("murid@x.test", "Murid Hapus", 10, major, 0)
	avatars := NewAvatarStore(f.db, t.TempDir())
	dir := avatars.Dir()

	// Removing nothing succeeds from the start (AVT-061).
	if err := avatars.Remove(f.ctx, accountID); err != nil {
		t.Fatalf("remove without a photo: %v", err)
	}

	name := testAvatarName("a")
	if err := avatars.Commit(f.ctx, accountID, writeAvatarSrc(t, "photo-bytes"), name); err != nil {
		t.Fatalf("seed commit: %v", err)
	}
	if err := avatars.Remove(f.ctx, accountID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := f.qustring("SELECT avatar_name FROM accounts WHERE id = ?", accountID); got.Valid {
		t.Fatalf("avatar_name = %v, want NULL", got.String)
	}
	requireNoFile(t, filepath.Join(dir, name), "removed file "+name)
	requireNoTemps(t, dir)

	// Idempotent: a second remove is still success and writes nothing.
	if err := avatars.Remove(f.ctx, accountID); err != nil {
		t.Fatalf("second remove: %v", err)
	}
	if got := f.qustring("SELECT avatar_name FROM accounts WHERE id = ?", accountID); got.Valid {
		t.Fatalf("avatar_name after second remove = %v, want NULL", got.String)
	}

	// A concurrently deleted account is session_expired, never a silent win.
	other := f.seedTeacher("hapus@x.test", "Guru Penghapus", viewmodel.AccountActive)
	f.softDeleteAccount(other, accountID)
	if err := avatars.Remove(f.ctx, accountID); !errors.Is(err, ErrSessionGone) {
		t.Fatalf("remove of deleted account err = %v, want ErrSessionGone", err)
	}
}

// ---- Serving lookup (AVT-070) -----------------------------------------------

func TestAvatarHasLiveOwner(t *testing.T) {
	f := newFixture(t)
	major := f.seedMajor("Logistik", "LOG")
	accountID, _ := f.seedStudent("murid@x.test", "Murid Foto", 10, major, 0)
	pending := f.seedTeacher("baru@x.test", "Guru Baru", viewmodel.AccountPending)
	avatars := NewAvatarStore(f.db, t.TempDir())

	mine, theirs := testAvatarName("1"), testAvatarName("2")
	f.exec("UPDATE accounts SET avatar_name = ? WHERE id = ?", mine, accountID)

	if ok, err := avatars.HasLiveOwner(f.ctx, mine); err != nil || !ok {
		t.Fatalf("live owner = %v, %v, want true, nil", ok, err)
	}
	if ok, err := avatars.HasLiveOwner(f.ctx, theirs); err != nil || ok {
		t.Fatalf("unreferenced name = %v, %v, want false, nil", ok, err)
	}
	if ok, err := avatars.HasLiveOwner(f.ctx, "../"+mine); err != nil || ok {
		t.Fatalf("malformed name = %v, %v, want false, nil", ok, err)
	}

	// A pending owner is not live, so its photo stops resolving.
	f.exec("UPDATE accounts SET avatar_name = ? WHERE id = ?", theirs, pending)
	if ok, err := avatars.HasLiveOwner(f.ctx, theirs); err != nil || ok {
		t.Fatalf("pending owner = %v, %v, want false, nil", ok, err)
	}

	// A soft-deleted owner stops resolving the moment the row changes.
	other := f.seedTeacher("ketua@x.test", "Ketua Guru", viewmodel.AccountActive)
	f.softDeleteAccount(other, accountID)
	if ok, err := avatars.HasLiveOwner(f.ctx, mine); err != nil || ok {
		t.Fatalf("soft-deleted owner = %v, %v, want false, nil", ok, err)
	}
}
