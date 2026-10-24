package authoring

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
)

// ImageRecord is the serving metadata of one live question image (AUT-054).
// The bytes stay on disk; this row is what the route is allowed to believe.
type ImageRecord struct {
	Name       string
	QuestionID int64
	QuizID     int64
	MIME       string
	Bytes      int64
	SHA256     []byte
}

// ValidImageName reports whether name is exactly the generated 128-bit
// capability name pinned by AUT-015/AUT-028: 32 lowercase hex characters and
// one of the three stored extensions. Uppercase hex, extra dots, `.jpeg`,
// directories and traversal patterns are all false.
func ValidImageName(name string) bool {
	return safeImageName.MatchString(name)
}

// LookupImage resolves one image name to its live question and live quiz
// (AUT-054). A missing name, a deleted question, a deleted quiz and a
// non-matching path are the same 404. A duplicated path or a partial
// path/MIME/bytes/SHA row is an integrity fault, never a served image
// (AUT-029, INV-24).
func (s *Service) LookupImage(ctx context.Context, name string) (*ImageRecord, error) {
	if !ValidImageName(name) {
		return nil, apperr.NotFoundf().WithRule("AUT-015")
	}
	if s.db == nil {
		return nil, internal(errors.New("authoring: no database"))
	}

	rows, err := s.db.QueryContext(ctx, `SELECT z.id, z.quiz_id, z.image_mime, z.image_bytes, z.image_sha256
		FROM questions z JOIN quizzes q ON q.id = z.quiz_id
		WHERE z.image_path = ? AND z.deleted_at = ? AND q.deleted_at = ?
		LIMIT 2`, name, store.LiveAt, store.LiveAt)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()

	var found []ImageRecord
	for rows.Next() {
		var (
			rec   ImageRecord
			mime  sql.NullString
			bytes sql.NullInt64
		)
		if err := rows.Scan(&rec.QuestionID, &rec.QuizID, &mime, &bytes, &rec.SHA256); err != nil {
			return nil, internal(err)
		}
		if !mime.Valid || !bytes.Valid || len(rec.SHA256) != 32 {
			return nil, internal(fmt.Errorf("authoring: question %d has a partial image row", rec.QuestionID)).WithRule("AUT-029")
		}
		if _, ok := extensionForMIME(mime.String); !ok {
			return nil, internal(fmt.Errorf("authoring: question %d has an unsupported image MIME", rec.QuestionID)).WithRule("AUT-029")
		}
		if want, ok := mimeForExtension(filepath.Ext(name)); !ok || want != mime.String {
			return nil, internal(fmt.Errorf("authoring: question %d image extension does not match its MIME", rec.QuestionID)).WithRule("AUT-029")
		}
		if bytes.Int64 < 0 {
			return nil, internal(fmt.Errorf("authoring: question %d has a negative image size", rec.QuestionID)).WithRule("AUT-029")
		}
		rec.Name = name
		rec.MIME = mime.String
		rec.Bytes = bytes.Int64
		found = append(found, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}

	switch len(found) {
	case 0:
		return nil, apperr.NotFoundf().WithRule("AUT-054")
	case 1:
		return &found[0], nil
	default:
		// One file may never be shared between questions (AUT-INV-09).
		return nil, internal(fmt.Errorf("authoring: image %q is referenced by more than one live question", name)).WithRule("AUT-INV-09")
	}
}
