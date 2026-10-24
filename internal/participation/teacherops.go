package participation

// teacherops.go implements the PAR teacher write surfaces: editing and deleting
// an anonymous identity (PAR-096…PAR-108) and creating, renaming and deleting a
// major (PAR-109…PAR-125). Every command runs in one transaction, resolves the
// teacher inside it, and takes its locks in the OPS order identity →
// registration → attempt → answer → selection (§9.1).
//
// Validation failures are returned as field errors after the page has been
// rebuilt inside the same transaction; the transaction then rolls back, so a
// rejected form never writes.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// AnonymousEditInput is the POST /teacher/anonymous/:id body. Values stay raw
// strings so the same identify rules can produce field errors (PAR-098).
type AnonymousEditInput struct {
	Actor      ActorInput
	ID         int64
	FullName   string
	GradeLevel string
	MajorID    string
}

// AnonymousEditResult carries either the rebuilt 422 form or the 303 target.
type AnonymousEditResult struct {
	Form   viewmodel.AnonymousForm
	Result Result
}

// AnonymousDeleteInput is the POST /teacher/anonymous/:id/delete material.
type AnonymousDeleteInput struct {
	Actor ActorInput
	ID    int64
}

// MajorCreateInput is the POST /teacher/majors body: exactly `name`
// (PAR-109). The code is generated, never submitted.
type MajorCreateInput struct {
	Actor ActorInput
	Name  string
}

// MajorRenameInput is the POST /teacher/majors/:id body.
type MajorRenameInput struct {
	Actor ActorInput
	ID    int64
	Name  string
}

// MajorDeleteInput is the POST /teacher/majors/:id/delete material.
type MajorDeleteInput struct {
	Actor ActorInput
	ID    int64
}

// MajorWriteResult carries either the rebuilt majors page for a 422 or the 303
// target for a committed write.
type MajorWriteResult struct {
	Page   viewmodel.MajorsPage
	Result Result
}

// EditAnonymous serves POST /teacher/anonymous/:id (PAR-096…PAR-101). On a
// validation failure it returns the rebuilt form with the submitted values and
// the same *apperr.Error the adapter renders at 422; nothing is written.
func (s *Service) EditAnonymous(ctx context.Context, in AnonymousEditInput) (res AnonymousEditResult, err error) {
	if in.ID <= 0 {
		return res, apperr.NotFoundf().WithRule("PAR-096")
	}
	err = s.writeTx(ctx, func(tx *sql.Tx) error {
		if _, err := teacherActor(ctx, tx, in.Actor, "PAR-096"); err != nil {
			return err
		}

		var id int64
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM anonymous_participants
			WHERE id = ? AND deleted_at = ?
			FOR UPDATE`, in.ID, store.LiveAt).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return apperr.NotFoundf().WithRule("PAR-096")
		}
		if err != nil {
			return internal(err)
		}

		name, nameMsg := validateName(in.FullName)
		grade, gradeMsg := validateGrade(in.GradeLevel)
		majorID, majorMsg := validateMajorID(in.MajorID)
		fields := map[string]string{}
		if nameMsg != "" {
			fields["full_name"] = nameMsg
		}
		if gradeMsg != "" {
			fields["grade_level"] = gradeMsg
		}
		if majorMsg != "" {
			fields["major_id"] = majorMsg
		}
		if majorMsg == "" {
			// PAR-099: shared-lock the selected live major before writing it.
			_, ok, err := readMajorByID(ctx, tx, majorID, " LOCK IN SHARE MODE")
			if err != nil {
				return err
			}
			if !ok {
				fields["major_id"] = ErrMajor
			}
		}
		if len(fields) > 0 {
			form, err := readAnonymousForm(ctx, tx, in.ID)
			if err != nil {
				return err
			}
			overlayAnonymousForm(&form, in)
			res.Form = form
			return apperr.Validationf(fields)
		}

		// updated_at moves via the column's ON UPDATE; last_seen_at is not
		// touched because a teacher edit is not participant activity (PAR-099).
		if _, err := tx.ExecContext(ctx, `
			UPDATE anonymous_participants
			SET display_name = ?, grade_level = ?, major_id = ?
			WHERE id = ? AND deleted_at = ?`,
			name, grade, majorID, in.ID, store.LiveAt); err != nil {
			return internal(err)
		}
		res.Result = Result{Path: "/teacher/anonymous", Flash: "saved"}
		return nil
	})
	return res, err
}

// DeleteAnonymous serves POST /teacher/anonymous/:id/delete (PAR-102…PAR-108).
// The cascade locks root first and stamps one database timestamp on every row;
// only the identity root records the acting teacher.
func (s *Service) DeleteAnonymous(ctx context.Context, in AnonymousDeleteInput) (Result, error) {
	var out Result
	if in.ID <= 0 {
		return out, apperr.NotFoundf().WithRule("PAR-102")
	}
	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		actor, err := teacherActor(ctx, tx, in.Actor, "PAR-102")
		if err != nil {
			return err
		}
		ts, err := nowT(ctx, tx)
		if err != nil {
			return err
		}

		// PAR-104 lock order: identity → registration → attempt → answer →
		// selection, then the attempt-keyed focus events.
		var id int64
		err = tx.QueryRowContext(ctx, `
			SELECT id FROM anonymous_participants
			WHERE id = ? AND deleted_at = ?
			FOR UPDATE`, in.ID, store.LiveAt).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return apperr.NotFoundf().WithRule("PAR-102")
		}
		if err != nil {
			return internal(err)
		}

		locks := []string{
			`SELECT id FROM registrations
			 WHERE anonymous_participant_id = ? AND deleted_at = ? FOR UPDATE`,
			`SELECT qa.id
			 FROM quiz_attempts qa
			 JOIN registrations r ON r.id = qa.registration_id
			 WHERE r.anonymous_participant_id = ? AND r.deleted_at = ?
			   AND qa.deleted_at = ?
			 FOR UPDATE`,
			`SELECT aa.id
			 FROM attempt_answers aa
			 JOIN quiz_attempts qa ON qa.id = aa.attempt_id
			 JOIN registrations r ON r.id = qa.registration_id
			 WHERE r.anonymous_participant_id = ? AND r.deleted_at = ?
			   AND qa.deleted_at = ? AND aa.deleted_at = ?
			 FOR UPDATE`,
			`SELECT sel.id
			 FROM attempt_answer_selections sel
			 JOIN attempt_answers aa ON aa.id = sel.attempt_answer_id
			 JOIN quiz_attempts qa ON qa.id = aa.attempt_id
			 JOIN registrations r ON r.id = qa.registration_id
			 WHERE r.anonymous_participant_id = ? AND r.deleted_at = ?
			   AND qa.deleted_at = ? AND aa.deleted_at = ?
			   AND sel.deleted_at = ?
			 FOR UPDATE`,
			`SELECT fe.id
			 FROM focus_loss_events fe
			 JOIN quiz_attempts qa ON qa.id = fe.attempt_id
			 JOIN registrations r ON r.id = qa.registration_id
			 WHERE r.anonymous_participant_id = ? AND r.deleted_at = ?
			   AND qa.deleted_at = ? AND fe.deleted_at = ?
			 FOR UPDATE`,
		}
		lockArgs := [][]any{
			{in.ID, store.LiveAt},
			{in.ID, store.LiveAt, store.LiveAt},
			{in.ID, store.LiveAt, store.LiveAt, store.LiveAt},
			{in.ID, store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt},
			{in.ID, store.LiveAt, store.LiveAt, store.LiveAt},
		}
		for i, query := range locks {
			if err := lockRows(ctx, tx, query, lockArgs[i]...); err != nil {
				return err
			}
		}

		res, err := tx.ExecContext(ctx, `
			UPDATE anonymous_participants
			SET deleted_at = ?, deleted_by_account_id = ?
			WHERE id = ? AND deleted_at = ?`,
			ts, int64(actor.Account), in.ID, store.LiveAt)
		if err != nil {
			return internal(err)
		}
		if affected, err := res.RowsAffected(); err != nil {
			return internal(err)
		} else if affected != 1 {
			return apperr.NotFoundf().WithRule("PAR-102")
		}

		// One timestamp, root to leaf. Descendant rows never carry the actor
		// (PAR-108, INV-18).
		stamps := []struct {
			query string
			args  []any
		}{
			{`UPDATE registrations SET deleted_at = ?
			  WHERE anonymous_participant_id = ? AND deleted_at = ?`,
				[]any{ts, in.ID, store.LiveAt}},
			{`UPDATE quiz_attempts qa
			    JOIN registrations r ON r.id = qa.registration_id
			       SET qa.deleted_at = ?
			     WHERE r.anonymous_participant_id = ? AND r.deleted_at = ?
			       AND qa.deleted_at = ?`,
				[]any{ts, in.ID, ts, store.LiveAt}},
			{`UPDATE attempt_answers aa
			    JOIN quiz_attempts qa ON qa.id = aa.attempt_id
			    JOIN registrations r ON r.id = qa.registration_id
			       SET aa.deleted_at = ?
			     WHERE r.anonymous_participant_id = ? AND r.deleted_at = ?
			       AND qa.deleted_at = ? AND aa.deleted_at = ?`,
				[]any{ts, in.ID, ts, ts, store.LiveAt}},
			{`UPDATE attempt_answer_selections sel
			    JOIN attempt_answers aa ON aa.id = sel.attempt_answer_id
			    JOIN quiz_attempts qa ON qa.id = aa.attempt_id
			    JOIN registrations r ON r.id = qa.registration_id
			       SET sel.deleted_at = ?
			     WHERE r.anonymous_participant_id = ? AND r.deleted_at = ?
			       AND qa.deleted_at = ? AND aa.deleted_at = ?
			       AND sel.deleted_at = ?`,
				[]any{ts, in.ID, ts, ts, ts, store.LiveAt}},
			{`UPDATE focus_loss_events fe
			    JOIN quiz_attempts qa ON qa.id = fe.attempt_id
			    JOIN registrations r ON r.id = qa.registration_id
			       SET fe.deleted_at = ?
			     WHERE r.anonymous_participant_id = ? AND r.deleted_at = ?
			       AND qa.deleted_at = ? AND fe.deleted_at = ?`,
				[]any{ts, in.ID, ts, ts, store.LiveAt}},
		}
		for _, step := range stamps {
			if _, err := tx.ExecContext(ctx, step.query, step.args...); err != nil {
				return internal(err)
			}
		}
		out = Result{Path: "/teacher/anonymous", Flash: "deleted"}
		return nil
	})
	return out, err
}

// CreateMajor serves POST /teacher/majors (PAR-109…PAR-114).
func (s *Service) CreateMajor(ctx context.Context, in MajorCreateInput) (res MajorWriteResult, err error) {
	err = s.writeTx(ctx, func(tx *sql.Tx) error {
		if _, err := teacherActor(ctx, tx, in.Actor, "PAR-109"); err != nil {
			return err
		}
		name, msg := validateMajorName(in.Name)
		if msg != "" {
			return majorCreate422(ctx, tx, &res, msg)
		}
		if derr := majorTaken(ctx, tx, name, 0); derr != nil {
			if _, ok := FieldErrorsOf(derr); ok {
				page, perr := readMajorsPage(ctx, tx)
				if perr != nil {
					return perr
				}
				res.Page = page
			}
			return derr
		}
		code, err := nextMajorCode(ctx, tx, name)
		if err != nil {
			return err
		}
		insert := `INSERT INTO majors (name, code) VALUES (?, ?)`
		if _, err := tx.ExecContext(ctx, insert, name, code); err != nil {
			if !store.IsDuplicate(err) {
				return internal(err)
			}
			// The name check above may have lost a race, or the generated code
			// may have been taken between the probe and the insert (PAR-112).
			_, nameLive, nerr := readMajorByName(ctx, tx, name)
			if nerr != nil {
				return nerr
			}
			if nameLive {
				return majorCreate422(ctx, tx, &res, ErrMajorDup)
			}
			code, err = nextMajorCode(ctx, tx, name)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, insert, name, code); err != nil {
				return internal(err)
			}
		}
		res.Result = Result{Path: "/teacher/majors", Flash: "saved"}
		return nil
	})
	return res, err
}

// RenameMajor serves POST /teacher/majors/:id (PAR-115…PAR-119). The code is
// immutable; only the name moves.
func (s *Service) RenameMajor(ctx context.Context, in MajorRenameInput) (res MajorWriteResult, err error) {
	if in.ID <= 0 {
		return res, apperr.NotFoundf().WithRule("PAR-115")
	}
	err = s.writeTx(ctx, func(tx *sql.Tx) error {
		if _, err := teacherActor(ctx, tx, in.Actor, "PAR-115"); err != nil {
			return err
		}
		// PAR-115: a missing or soft-deleted target is the same 404 before any
		// form rule is considered.
		if _, ok, err := readMajorByID(ctx, tx, in.ID, " FOR UPDATE"); err != nil {
			return err
		} else if !ok {
			return apperr.NotFoundf().WithRule("PAR-115")
		}
		name, msg := validateMajorName(in.Name)
		if msg != "" {
			return majorRename422(ctx, tx, &res, in.ID, in.Name, msg)
		}
		if derr := majorTaken(ctx, tx, name, in.ID); derr != nil {
			if _, ok := FieldErrorsOf(derr); ok {
				return majorRename422(ctx, tx, &res, in.ID, in.Name, ErrMajorDup)
			}
			return derr
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE majors SET name = ?
			WHERE id = ? AND deleted_at = ?`,
			name, in.ID, store.LiveAt); err != nil {
			return internal(err)
		}
		res.Result = Result{Path: "/teacher/majors", Flash: "saved"}
		return nil
	})
	return res, err
}

// DeleteMajor serves POST /teacher/majors/:id/delete (PAR-120…PAR-125). A
// referenced major is a committed no-op that redirects with the
// `major-in-use` flash: a state conflict, not a field error.
func (s *Service) DeleteMajor(ctx context.Context, in MajorDeleteInput) (Result, error) {
	var out Result
	if in.ID <= 0 {
		return out, apperr.NotFoundf().WithRule("PAR-120")
	}
	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		actor, err := teacherActor(ctx, tx, in.Actor, "PAR-120")
		if err != nil {
			return err
		}
		ts, err := nowT(ctx, tx)
		if err != nil {
			return err
		}
		if _, ok, err := readMajorByID(ctx, tx, in.ID, " FOR UPDATE"); err != nil {
			return err
		} else if !ok {
			return apperr.NotFoundf().WithRule("PAR-120")
		}
		students, anons, quizzes, err := majorReferenceCounts(ctx, tx, in.ID, "")
		if err != nil {
			return err
		}
		if students > 0 || anons > 0 || quizzes > 0 {
			out = Result{Path: "/teacher/majors", Flash: "major-in-use"}
			return nil
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE majors
			SET deleted_at = ?, deleted_by_account_id = ?
			WHERE id = ? AND deleted_at = ?`,
			ts, int64(actor.Account), in.ID, store.LiveAt)
		if err != nil {
			return internal(err)
		}
		if affected, err := res.RowsAffected(); err != nil {
			return internal(err)
		} else if affected != 1 {
			return apperr.NotFoundf().WithRule("PAR-120")
		}
		out = Result{Path: "/teacher/majors", Flash: "deleted"}
		return nil
	})
	return out, err
}

// readAnonymousForm loads one live identity and the live major list. It is
// shared by the GET form and the in-transaction 422 rebuild (PAR-175).
func readAnonymousForm(ctx context.Context, q store.Querier, id int64) (viewmodel.AnonymousForm, error) {
	var form viewmodel.AnonymousForm
	if id <= 0 {
		return form, apperr.NotFoundf().WithRule("PAR-173")
	}
	var grade int
	var majorID int64
	err := q.QueryRowContext(ctx, `
		SELECT id, display_name, grade_level, major_id
		FROM anonymous_participants
		WHERE id = ? AND deleted_at = ?`,
		id, store.LiveAt).Scan(&form.ID, &form.FullName, &grade, &majorID)
	if errors.Is(err, sql.ErrNoRows) {
		return form, apperr.NotFoundf().WithRule("PAR-173")
	}
	if err != nil {
		return form, internal(err)
	}
	majors, err := readMajors(ctx, q)
	if err != nil {
		return form, err
	}
	form.GradeLevel = grade
	form.MajorID = majorID
	form.Majors = majors
	form.Action = anonymousPath(uint32(form.ID))
	form.CancelHref = "/teacher/anonymous"
	form.IsNew = false
	return form, nil
}

// overlayAnonymousForm echoes the submitted values that parse, keeping the
// current values for the keys that do not (PAR-098, IDN-083 precedent).
func overlayAnonymousForm(form *viewmodel.AnonymousForm, in AnonymousEditInput) {
	form.FullName = strings.TrimSpace(in.FullName)
	if grade, msg := validateGrade(in.GradeLevel); msg == "" {
		form.GradeLevel = int(grade)
	}
	if id, msg := validateMajorID(in.MajorID); msg == "" {
		for _, m := range form.Majors {
			if m.ID == id {
				form.MajorID = id
				break
			}
		}
	}
}

// majorCreate422 rebuilds the catalogue for a rejected create. The row model
// has no create-value echo slot, so the submitted name cannot be preserved
// here; the template gap is recorded in the report.
func majorCreate422(ctx context.Context, tx *sql.Tx, res *MajorWriteResult, msg string) error {
	page, err := readMajorsPage(ctx, tx)
	if err != nil {
		return err
	}
	res.Page = page
	return apperr.Field("name", msg)
}

// majorRename422 rebuilds the catalogue and overlays the submitted value on the
// failing row, which is the only echo slot MajorRow offers (PAR-116).
func majorRename422(ctx context.Context, tx *sql.Tx, res *MajorWriteResult, id int64, submitted, msg string) error {
	page, err := readMajorsPage(ctx, tx)
	if err != nil {
		return err
	}
	for i := range page.Majors {
		if page.Majors[i].ID == id {
			page.Majors[i].Name = strings.TrimSpace(submitted)
			break
		}
	}
	res.Page = page
	return apperr.Field("name", msg)
}

// lockRows executes one SELECT … FOR UPDATE and drains its ids. The ids are not
// used: taking the row locks in a fixed order is the point (PAR-104, §9.1).
func lockRows(ctx context.Context, tx *sql.Tx, query string, args ...any) error {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return internal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return internal(err)
		}
	}
	if err := rows.Err(); err != nil {
		return internal(err)
	}
	return nil
}

// majorCodeAlphabet is the PAR-111 code alphabet: uppercase ASCII letters and
// digits, never a separator or a lowercase letter.
const majorCodeAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// majorCodeBase derives the candidate code from the name: ASCII letters
// uppercased, digits kept, everything else dropped, first ten characters.
// An empty result falls back to JUR.
func majorCodeBase(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteByte(byte(r - 'a' + 'A'))
		case (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteByte(byte(r))
		default:
			continue
		}
		if b.Len() == 10 {
			break
		}
	}
	if b.Len() == 0 {
		return "JUR"
	}
	return b.String()
}

// fitMajorCode appends suffix within the ten-character ceiling, shortening the
// base rather than overflowing the column.
func fitMajorCode(base, suffix string) string {
	maxBase := 10 - len(suffix)
	if maxBase < 0 {
		maxBase = 0
	}
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	return base + suffix
}

// nextMajorCode finds the first free live code: the base, then numeric
// suffixes, then crypto-random four-character suffixes (PAR-111).
func nextMajorCode(ctx context.Context, q store.Querier, name string) (string, error) {
	base := majorCodeBase(name)
	taken, err := majorCodeLive(ctx, q, base)
	if err != nil {
		return "", err
	}
	if !taken {
		return base, nil
	}
	for n := 2; n <= 9999; n++ {
		candidate := fitMajorCode(base, strconv.Itoa(n))
		taken, err := majorCodeLive(ctx, q, candidate)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
	for attempt := 0; attempt < 8; attempt++ {
		suffix, err := randomMajorSuffix(4)
		if err != nil {
			return "", err
		}
		candidate := fitMajorCode(base, suffix)
		taken, err := majorCodeLive(ctx, q, candidate)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
	return "", internal(errors.New("participation: major code space exhausted"))
}

// majorCodeLive reports whether a live major already owns the code. The unique
// key includes deleted_at, so soft-deleted codes are reusable (INV-19).
func majorCodeLive(ctx context.Context, q store.Querier, code string) (bool, error) {
	var one int
	err := q.QueryRowContext(ctx,
		"SELECT 1 FROM majors WHERE code = ? AND deleted_at = ?",
		code, store.LiveAt).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, internal(err)
	}
	return true, nil
}

// randomMajorSuffix returns n crypto-random code characters.
func randomMajorSuffix(n int) (string, error) {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		return "", internal(err)
	}
	out := make([]byte, n)
	for i, b := range raw {
		out[i] = majorCodeAlphabet[int(b)%len(majorCodeAlphabet)]
	}
	return string(out), nil
}
