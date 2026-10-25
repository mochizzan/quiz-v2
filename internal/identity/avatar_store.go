package identity

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"

	"quiz-v2/internal/store"
)

// avatarNameRe is the generated-name grammar of AVT-010: 16 random bytes as
// lowercase hex plus the .webp the server always re-encodes to. The regexp is
// local to this package on purpose — identity is the only writer of
// accounts.avatar_name and of the avatars directory (design §13), so the
// grammar lives beside the writer, never beside a caller.
var avatarNameRe = regexp.MustCompile(`^[a-f0-9]{32}\.webp$`)

// ErrAvatarName is a newName outside that grammar. It is a programming fault,
// never a user-input failure: names are minted by the caller and validated
// here before any I/O happens.
var ErrAvatarName = errors.New("identity: invalid avatar name")

const (
	// avatarDirName is the subdirectory of UPLOADS_DIR the photos live in
	// (AVT-103: no new environment variable; the path is derived).
	avatarDirName = "avatars"
	// avatarDirMode is the avatars directory mode (AVT-100, OPS-245).
	avatarDirMode = 0o750
	// avatarFileMode is the stored file mode (AVT-100, OPS-245).
	avatarFileMode = 0o640
)

// AvatarStore is the only writer of accounts.avatar_name and of
// UPLOADS_DIR/avatars (design §4.2). It binds one pool to the configured
// upload root exactly like the authoring upload service, is built per request
// by the HTTP layer, and holds no request state. Every file it writes is
// complete and fsynced before any row references it, and no row stops
// referencing a file before that file is unlinked.
type AvatarStore struct {
	db      *store.DB
	uploads string
	logger  *slog.Logger
}

// NewAvatarStore returns the avatar store over the pool and the configured
// upload root (UPLOADS_DIR). A nil pool is tolerated at construction for the
// handler tests; every method that needs the database answers 500 instead of
// panicking.
func NewAvatarStore(db *store.DB, uploadsDir string) *AvatarStore {
	return &AvatarStore{db: db, uploads: uploadsDir, logger: slog.Default()}
}

// WithLogger injects the process logger so orphan-file events land in the
// same structured stream as the HTTP layer. A nil logger keeps the default.
func (s *AvatarStore) WithLogger(l *slog.Logger) *AvatarStore {
	if l != nil {
		s.logger = l
	}
	return s
}

// Dir returns the avatar root: UPLOADS_DIR/avatars, created lazily by the
// first write (AVT-103). The serving route reads only this directory
// (AVT-074), so it uses Dir as its file root.
func (s *AvatarStore) Dir() string { return filepath.Join(s.uploads, avatarDirName) }

// Commit places the produced WebP at srcPath under newName in the avatars
// directory and points accounts.avatar_name at it, in the mandatory storage
// order of design §4.2:
//
//	write and fsync the new file → rename it to newName → commit the column
//	update → unlink the old file
//
// newName must match the generated grammar or the call fails with
// ErrAvatarName before anything is created. Any failure before the commit
// removes the new file and leaves the column — and therefore the old photo —
// untouched. A failed unlink of the old file is logged and tolerated: that
// name is already unreferenced, so its URL 404s and the bytes are an orphan
// for AVT-106 to collect. A missing or soft-deleted account is ErrSessionGone,
// exactly like the other account-row writes.
func (s *AvatarStore) Commit(ctx context.Context, accountID int64, srcPath, newName string) error {
	if !avatarNameRe.MatchString(newName) {
		return ErrAvatarName
	}
	if s.db == nil {
		return internalErr(errors.New("identity: no database"))
	}
	dir := s.Dir()
	tmpPath, err := stageAvatar(dir, srcPath)
	if err != nil {
		return internalErr(err)
	}
	// newPath tracks the new bytes wherever they live right now — the temp
	// file until the rename, the final name after it — so every failure path
	// below removes exactly the file that exists, and only a committed
	// column keeps it.
	newPath := tmpPath
	committed := false
	defer func() {
		if committed {
			return
		}
		if err := os.Remove(newPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			s.logger.Warn("avatar_orphan", "rule", "AVT-106", "name", filepath.Base(newPath), "error", err)
		}
	}()
	if err := os.Rename(tmpPath, filepath.Join(dir, newName)); err != nil {
		_ = os.Remove(tmpPath)
		return internalErr(err)
	}
	newPath = filepath.Join(dir, newName)
	if err := os.Chmod(newPath, avatarFileMode); err != nil {
		return internalErr(err)
	}

	var replaced sql.NullString
	err = s.db.TxRetry(ctx, func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, `
			SELECT id, avatar_name FROM accounts
			WHERE id = ? AND deleted_at = ?
			FOR UPDATE`, accountID, store.LiveAt).Scan(&id, &replaced)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSessionGone
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE accounts
			SET avatar_name = ?
			WHERE id = ? AND deleted_at = ?`,
			newName, accountID, store.LiveAt)
		return err
	})
	if err != nil {
		// The deferred cleanup removes the new file; the rolled-back column
		// still references the old one, which is untouched on disk.
		return classified(err)
	}
	committed = true
	// A repeated name would unlink the file just committed; with 128 random
	// bits that cannot happen, but the guard costs nothing.
	if replaced.Valid && replaced.String != newName {
		s.discardAvatar(replaced.String)
	}
	return nil
}

// Remove clears accounts.avatar_name and unlinks the file (AVT-061).
// Removing a photo that does not exist succeeds: the command is idempotent.
// Like every account-row write it locks the row first, so a concurrently
// deleted account is ErrSessionGone rather than a silent success. A failed
// unlink is logged and tolerated (the column is already clear, so the bytes
// are an orphan AVT-106 collects later).
func (s *AvatarStore) Remove(ctx context.Context, accountID int64) error {
	if s.db == nil {
		return internalErr(errors.New("identity: no database"))
	}
	var removed sql.NullString
	err := s.db.TxRetry(ctx, func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, `
			SELECT id, avatar_name FROM accounts
			WHERE id = ? AND deleted_at = ?
			FOR UPDATE`, accountID, store.LiveAt).Scan(&id, &removed)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSessionGone
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE accounts
			SET avatar_name = NULL
			WHERE id = ? AND deleted_at = ?`,
			accountID, store.LiveAt)
		return err
	})
	if err != nil {
		return classified(err)
	}
	s.discardAvatar(removed.String)
	return nil
}

// HasLiveOwner reports whether a live, active account still references name —
// the serving route's only database check (AVT-070). A name outside the
// generated grammar can never be live and answers false before any I/O; a
// soft-deleted, pending, or replaced owner answers false as soon as the row
// changes. A database failure is an error, never a fabricated miss.
func (s *AvatarStore) HasLiveOwner(ctx context.Context, name string) (bool, error) {
	if !avatarNameRe.MatchString(name) {
		return false, nil
	}
	if s.db == nil {
		return false, internalErr(errors.New("identity: no database"))
	}
	var one int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1 FROM accounts
		WHERE avatar_name = ? AND deleted_at = ? AND status = 'active'
		LIMIT 1`, name, store.LiveAt).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, internalErr(err)
	}
	return true, nil
}

// discardAvatar unlinks a no-longer-referenced avatar file. A name outside
// the grammar is refused before it can become a path (the same guard the
// authoring delete has), a missing file is success, and any other failure is
// logged and tolerated: the column no longer references the name, so its URL
// already 404s.
func (s *AvatarStore) discardAvatar(name string) {
	if name == "" || !avatarNameRe.MatchString(name) {
		return
	}
	if err := os.Remove(filepath.Join(s.Dir(), name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		s.logger.Warn("avatar_orphan", "rule", "AVT-106", "name", name, "error", err)
	}
}

// stageAvatar places srcPath's bytes at a fresh dot-prefixed `.<random>.tmp`
// inside dir and leaves that file complete, fsynced, and closed, so a crash
// can only ever strand a temp file — never a half-written avatar under a live
// name (§4.2). The staging file is moved when it shares the uploads
// filesystem; a staging file on another filesystem falls back to a streamed
// copy. The leading dot keeps a crash's leftovers out of the avatar grammar
// and the .tmp suffix matches the startup cleanup (OPS-237).
func stageAvatar(dir, srcPath string) (tmpPath string, err error) {
	if err := os.MkdirAll(dir, avatarDirMode); err != nil {
		return "", err
	}
	suffix, err := randomHex(8)
	if err != nil {
		return "", err
	}
	tmpPath = filepath.Join(dir, "."+suffix+".tmp")
	if renameErr := os.Rename(srcPath, tmpPath); renameErr != nil {
		// Same move, other filesystem: copy through the exclusive temp name.
		// copyInto syncs, closes, and removes the temp on its own failures.
		if err := copyInto(tmpPath, srcPath); err != nil {
			return "", err
		}
		return tmpPath, nil
	}
	// The move kept the writer's bytes but not its descriptor: reopen to
	// fsync, so the data is on disk before it can gain a live name.
	f, err := os.OpenFile(tmpPath, os.O_WRONLY, 0)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

// copyInto streams src into a freshly created dst (mode 0600, exclusive) and
// leaves it synced and closed. Any failure removes dst, so the caller never
// has to distinguish how far the copy got.
func copyInto(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	cleanup := func(err error) error {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		return cleanup(err)
	}
	if err := out.Sync(); err != nil {
		return cleanup(err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}

// randomHex mints n random bytes as lowercase hex.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
