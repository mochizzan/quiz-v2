package identity

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strconv"
	"time"

	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// RosterInput is the parsed /teacher/students query plus the caller's identity
// and the optional one-shot reveal payload. Every value is already clamped:
// invalid filters fall back rather than becoming a 422 (IDN-091, IDN-I12).
type RosterInput struct {
	Query   string
	Grade   int
	MajorID int64
	Sort    viewmodel.SortOrder
	Page    int
	ActorID int64
	Reveal  *Reveal
}

// Students reads the filtered roster, its total, the tabs, the live majors, and
// the optional revalidated reset panel from one read-only snapshot (IDN-094,
// IDN-105).
func (s *Service) Students(ctx context.Context, in RosterInput) (viewmodel.StudentList, error) {
	var list viewmodel.StudentList
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		search := NormalizeSearch(in.Query)
		grade := in.Grade
		if grade != 10 && grade != 11 && grade != 12 {
			grade = 0
		}
		majorID := in.MajorID
		if majorID > 0 {
			live, err := liveMajor(ctx, tx, majorID)
			if err != nil {
				return err
			}
			if !live {
				majorID = 0
			}
		}
		sortOrder := in.Sort
		switch sortOrder {
		case viewmodel.SortOldest, viewmodel.SortNameAsc, viewmodel.SortNameDesc, viewmodel.SortNewest:
		default:
			sortOrder = viewmodel.SortNewest
		}

		where, whereArgs := rosterWhere(search, grade, majorID)
		joinArgs := []any{store.LiveAt, store.LiveAt}

		var total int
		countArgs := append(append([]any{}, joinArgs...), whereArgs...)
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM accounts a
			JOIN students s ON s.account_id = a.id AND s.deleted_at = ?
			LEFT JOIN majors m ON m.id = s.major_id AND m.deleted_at = ?
			WHERE `+where, countArgs...).Scan(&total); err != nil {
			return err
		}

		page := ClampPage(in.Page, total, PageSize)
		offset := (page - 1) * PageSize

		rowArgs := []any{store.LiveAt, store.LiveAt}
		rowArgs = append(rowArgs, joinArgs...)
		rowArgs = append(rowArgs, whereArgs...)
		rowArgs = append(rowArgs, PageSize, offset)

		rows, err := tx.QueryContext(ctx, `
			SELECT a.id, a.full_name, a.email, a.avatar_name,
			       s.id, s.grade_level, m.name,
			       (SELECT COUNT(*)
			          FROM quiz_attempts qa
			          JOIN registrations r ON r.id = qa.registration_id
			         WHERE r.student_id = s.id
			           AND r.deleted_at = ?
			           AND qa.deleted_at = ?) AS attempts
			FROM accounts a
			JOIN students s ON s.account_id = a.id AND s.deleted_at = ?
			LEFT JOIN majors m ON m.id = s.major_id AND m.deleted_at = ?
			WHERE `+where+`
			ORDER BY `+rosterOrder(sortOrder)+`
			LIMIT ? OFFSET ?`, rowArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		items := []viewmodel.StudentRow{}
		for rows.Next() {
			var (
				row    viewmodel.StudentRow
				grade  int
				major  sql.NullString
				avatar sql.NullString
				accID  int64
				stID   int64
			)
			if err := rows.Scan(&accID, &row.FullName, &row.Email, &avatar, &stID, &grade, &major, &row.Attempts); err != nil {
				return err
			}
			row.ID = stID
			row.GradeLevel = grade
			row.Avatar = viewmodel.NewAvatar(avatar.String, row.FullName, "app-avatar--sm")
			if major.Valid && major.String != "" {
				row.ClassLabel = viewmodel.ClassLabel(grade, major.String)
			} else {
				// A drifted major is shown as a dash, never by dropping the
				// student from the roster (IDN-090, IDN-121).
				row.ClassLabel = viewmodel.ClassLabel(0, "")
			}
			row.EditHref = studentPath(stID)
			row.ResetHref = studentPath(stID) + "/reset-token"
			row.DeleteHref = studentPath(stID) + "/delete"
			items = append(items, row)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		majors, err := LiveMajors(ctx, tx)
		if err != nil {
			return err
		}

		panel, err := loadReveal(ctx, tx, in.ActorID, in.Reveal)
		if err != nil {
			return err
		}

		filtered := search != "" || grade != 0 || majorID != 0
		empty := viewmodel.EmptyState{
			Title: "Belum ada siswa.",
			Body:  "Siswa yang mendaftar akan muncul di sini.",
			Icon:  "users",
		}
		if filtered {
			action := viewmodel.Outline("Hapus filter", "/teacher/students")
			empty = viewmodel.EmptyState{
				Title:  "Tidak ada siswa yang cocok.",
				Body:   "Ubah kata kunci atau filter kelas dan jurusan.",
				Icon:   "users",
				Action: &action,
			}
		}

		query := rosterQuery(search, grade, majorID, sortOrder)
		list = viewmodel.StudentList{
			Students: viewmodel.List[viewmodel.StudentRow]{
				Items:   items,
				Total:   total,
				Page:    page,
				PerPage: PageSize,
			},
			Pagination: viewmodel.NewPagination("/teacher/students", query, page, PageSize, total),
			Tabs:       rosterTabs(search, grade, majorID, sortOrder),
			Majors:     majors,
			Query: viewmodel.QueryState{
				Q:    search,
				Tab:  majorTab(majorID),
				Sort: sortOrder,
			},
			Empty:      empty,
			ResetToken: panel,
		}
		return nil
	})
	return list, err
}

// rosterWhere builds the shared predicate and its arguments.
func rosterWhere(search string, grade int, majorID int64) (string, []any) {
	where := "a.deleted_at = ? AND a.role = 'student'"
	args := []any{store.LiveAt}
	if grade != 0 {
		where += " AND s.grade_level = ?"
		args = append(args, grade)
	}
	if majorID != 0 {
		where += " AND s.major_id = ?"
		args = append(args, majorID)
	}
	if search != "" {
		pattern := "%" + EscapeLike(search) + "%"
		where += ` AND (a.full_name LIKE ? ESCAPE '\\' OR a.email LIKE ? ESCAPE '\\')`
		args = append(args, pattern, pattern)
	}
	return where, args
}

// rosterOrder returns the pinned ORDER BY for one sort. The tie-break is always
// id ASC so a page boundary is stable (IDN-092).
func rosterOrder(sortOrder viewmodel.SortOrder) string {
	switch sortOrder {
	case viewmodel.SortOldest:
		return "a.created_at ASC, a.id ASC"
	case viewmodel.SortNameAsc:
		return "a.full_name ASC, a.id ASC"
	case viewmodel.SortNameDesc:
		return "a.full_name DESC, a.id ASC"
	default:
		return "a.created_at DESC, a.id ASC"
	}
}

// rosterQuery echoes only effective filter values into pagination links
// (IDN-108).
func rosterQuery(search string, grade int, majorID int64, sortOrder viewmodel.SortOrder) url.Values {
	q := url.Values{}
	if search != "" {
		q.Set("q", search)
	}
	if grade != 0 {
		q.Set("grade", strconv.Itoa(grade))
	}
	if majorID != 0 {
		q.Set("major_id", strconv.FormatInt(majorID, 10))
	}
	if sortOrder != viewmodel.SortNewest {
		q.Set("sort", string(sortOrder))
	}
	return q
}

// rosterTabs builds the four grade links, preserving the other filters.
func rosterTabs(search string, grade int, majorID int64, sortOrder viewmodel.SortOrder) []viewmodel.FilterTab {
	defs := []struct {
		grade int
		label string
	}{
		{0, "Semua kelas"},
		{10, "Kelas X"},
		{11, "Kelas XI"},
		{12, "Kelas XII"},
	}
	tabs := make([]viewmodel.FilterTab, 0, len(defs))
	for _, def := range defs {
		q := rosterQuery(search, grade, majorID, sortOrder)
		q.Del("grade")
		if def.grade != 0 {
			q.Set("grade", strconv.Itoa(def.grade))
		}
		href := "/teacher/students"
		if encoded := q.Encode(); encoded != "" {
			href += "?" + encoded
		}
		tabs = append(tabs, viewmodel.FilterTab{
			Label:  def.label,
			Href:   href,
			Active: grade == def.grade,
		})
	}
	return tabs
}

// majorTab is the major id the filter select echoes.
func majorTab(id int64) string {
	if id <= 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}

// studentPath is the canonical edit URL for a students.id.
func studentPath(id int64) string {
	return "/teacher/students/" + intString(id)
}

// intString renders a positive id for a path. It is never user-controlled.
func intString(n int64) string { return strconv.FormatInt(n, 10) }

// liveMajor reports whether an id names a live major (IDN-082, IDN-091).
func liveMajor(ctx context.Context, q store.Querier, id int64) (bool, error) {
	var one int
	err := q.QueryRowContext(ctx, `
		SELECT 1 FROM majors WHERE id = ? AND deleted_at = ?`, id, store.LiveAt).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// loadReveal revalidates the signed five-minute panel payload against a live
// unused unexpired row issued by this teacher (IDN-105). A mismatch is simply
// no panel; it is never an error or an oracle.
func loadReveal(ctx context.Context, q store.Querier, actorID int64, reveal *Reveal) (*viewmodel.ResetToken, error) {
	if reveal == nil || actorID <= 0 || reveal.Teacher != actorID ||
		reveal.Student <= 0 || reveal.Token == "" {
		return nil, nil
	}
	var expires time.Time
	err := q.QueryRowContext(ctx, `
		SELECT t.expires_at
		FROM password_reset_tokens t
		JOIN students s ON s.id = t.student_id
		JOIN accounts a ON a.id = s.account_id
		WHERE t.token_hash = ?
		  AND t.student_id = ?
		  AND t.issued_by_account_id = ?
		  AND t.used_at IS NULL
		  AND t.deleted_at = ?
		  AND t.expires_at > UTC_TIMESTAMP()
		  AND s.deleted_at = ?
		  AND a.deleted_at = ?
		  AND a.role = 'student'`,
		secure.HashToken(reveal.Token), reveal.Student, actorID, store.LiveAt,
		store.LiveAt, store.LiveAt).Scan(&expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &viewmodel.ResetToken{
		Token:     reveal.Token,
		ExpiresAt: viewmodel.NewLocalTime(expires),
	}, nil
}

// StudentInput is the teacher's edit form.
type StudentInput struct {
	FullName string
	Email    string
	Grade    string
	MajorID  string
}

// StudentForm reads one live student for the edit screen (IDN-C17). Only live
// majors are offered; a drifted current major is not selectable.
func (s *Service) StudentForm(ctx context.Context, id int64) (viewmodel.StudentForm, error) {
	var form viewmodel.StudentForm
	if id <= 0 {
		return form, ErrNotFound
	}
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		var (
			email   string
			grade   int
			majorID int64
			avatar  sql.NullString
		)
		err := tx.QueryRowContext(ctx, `
			SELECT a.full_name, a.email, a.avatar_name, s.grade_level, s.major_id
			FROM students s
			JOIN accounts a ON a.id = s.account_id
			WHERE s.id = ?
			  AND s.deleted_at = ?
			  AND a.deleted_at = ?
			  AND a.role = 'student'`,
			id, store.LiveAt, store.LiveAt,
		).Scan(&form.FullName, &email, &avatar, &grade, &majorID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		majors, err := LiveMajors(ctx, tx)
		if err != nil {
			return err
		}
		form.ID = id
		form.Email = email
		form.GradeLevel = grade
		form.MajorID = majorID
		form.Majors = majors
		form.Avatar = viewmodel.NewAvatar(avatar.String, form.FullName, "")
		form.Action = studentPath(id)
		form.CancelHref = "/teacher/students"
		form.IsNew = false
		return nil
	})
	return form, err
}

// UpdateStudent edits one live student under the student/account row lock
// (IDN-C18). Name/email and grade/major commit together; a duplicate email is
// `email_taken`, a dead major `major_required`, and a missing target 404.
func (s *Service) UpdateStudent(ctx context.Context, id int64, in StudentInput) error {
	if id <= 0 {
		return ErrNotFound
	}
	name, email, school, err := ValidateStudentEdit(in.FullName, in.Email, in.Grade, in.MajorID)
	if err != nil {
		return err
	}
	return s.writeTx(ctx, func(tx *sql.Tx) error {
		var accountID int64
		err := tx.QueryRowContext(ctx, `
			SELECT a.id
			FROM students s
			JOIN accounts a ON a.id = s.account_id
			WHERE s.id = ?
			  AND s.deleted_at = ?
			  AND a.deleted_at = ?
			  AND a.role = 'student'
			FOR UPDATE`, id, store.LiveAt, store.LiveAt).Scan(&accountID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		live, err := LockMajor(ctx, tx, school.MajorID)
		if err != nil {
			return err
		}
		if !live {
			return ErrMajorRequired
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE accounts SET full_name = ?, email = ?
			WHERE id = ? AND deleted_at = ?`,
			name, email, accountID, store.LiveAt)
		if store.IsDuplicate(err) {
			return ErrEmailTaken
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE students SET grade_level = ?, major_id = ?
			WHERE id = ? AND deleted_at = ?`,
			school.GradeLevel, school.MajorID, id, store.LiveAt)
		return err
	})
}

// DeleteStudent performs the IDN-C19 soft-delete cascade with one database
// timestamp. Only the account row carries the acting teacher; every child is
// scoped by the parent's just-stamped timestamp so a second transaction's
// same-second rows are never touched.
func (s *Service) DeleteStudent(ctx context.Context, actorID, studentID int64) error {
	if studentID <= 0 {
		return ErrNotFound
	}
	return s.writeTx(ctx, func(tx *sql.Tx) error {
		var ts time.Time
		if err := tx.QueryRowContext(ctx, "SELECT UTC_TIMESTAMP()").Scan(&ts); err != nil {
			return err
		}
		var accountID int64
		err := tx.QueryRowContext(ctx, `
			SELECT a.id
			FROM accounts a
			JOIN students s ON s.account_id = a.id
			WHERE s.id = ?
			  AND a.role = 'student'
			  AND a.deleted_at = ?
			  AND s.deleted_at = ?
			FOR UPDATE`, studentID, store.LiveAt, store.LiveAt).Scan(&accountID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		res, err := tx.ExecContext(ctx, `
			UPDATE accounts SET deleted_at = ?, deleted_by_account_id = ?
			WHERE id = ? AND deleted_at = ?`,
			ts, actorID, accountID, store.LiveAt)
		if err != nil {
			return err
		}
		if affected, err := res.RowsAffected(); err != nil {
			return err
		} else if affected != 1 {
			return ErrNotFound
		}

		stamps := []struct {
			query string
			args  []any
		}{
			{`UPDATE students SET deleted_at = ? WHERE account_id = ? AND deleted_at = ?`,
				[]any{ts, accountID, store.LiveAt}},
			{`UPDATE sessions SET deleted_at = ? WHERE account_id = ? AND deleted_at = ?`,
				[]any{ts, accountID, store.LiveAt}},
			{`UPDATE password_reset_tokens SET deleted_at = ? WHERE student_id = ? AND deleted_at = ?`,
				[]any{ts, studentID, store.LiveAt}},
			{`UPDATE registrations SET deleted_at = ? WHERE student_id = ? AND deleted_at = ?`,
				[]any{ts, studentID, store.LiveAt}},
			{`UPDATE quiz_attempts qa
			    JOIN registrations r ON r.id = qa.registration_id
			       SET qa.deleted_at = ?
			     WHERE r.student_id = ? AND r.deleted_at = ? AND qa.deleted_at = ?`,
				[]any{ts, studentID, ts, store.LiveAt}},
			{`UPDATE attempt_answers aa
			    JOIN quiz_attempts qa ON qa.id = aa.attempt_id
			    JOIN registrations r ON r.id = qa.registration_id
			       SET aa.deleted_at = ?
			     WHERE r.student_id = ? AND r.deleted_at = ?
			       AND qa.deleted_at = ? AND aa.deleted_at = ?`,
				[]any{ts, studentID, ts, ts, store.LiveAt}},
			{`UPDATE attempt_answer_selections sel
			    JOIN attempt_answers aa ON aa.id = sel.attempt_answer_id
			    JOIN quiz_attempts qa ON qa.id = aa.attempt_id
			    JOIN registrations r ON r.id = qa.registration_id
			       SET sel.deleted_at = ?
			     WHERE r.student_id = ? AND r.deleted_at = ?
			       AND qa.deleted_at = ? AND aa.deleted_at = ?
			       AND sel.deleted_at = ?`,
				[]any{ts, studentID, ts, ts, ts, store.LiveAt}},
			{`UPDATE focus_loss_events fe
			    JOIN quiz_attempts qa ON qa.id = fe.attempt_id
			    JOIN registrations r ON r.id = qa.registration_id
			       SET fe.deleted_at = ?
			     WHERE r.student_id = ? AND r.deleted_at = ?
			       AND qa.deleted_at = ? AND fe.deleted_at = ?`,
				[]any{ts, studentID, ts, ts, store.LiveAt}},
		}
		for _, step := range stamps {
			if _, err := tx.ExecContext(ctx, step.query, step.args...); err != nil {
				return err
			}
		}
		return nil
	})
}

// IssuedReset is the raw token and its database expiry for the one-shot panel.
type IssuedReset struct {
	StudentID int64
	Token     string
	ExpiresAt time.Time
}

// IssueReset locks the live student, supersedes every live unused token, and
// inserts one new 24-hour token for the acting teacher (IDN-C20).
func (s *Service) IssueReset(ctx context.Context, actorID, studentID int64) (*IssuedReset, error) {
	if studentID <= 0 {
		return nil, ErrNotFound
	}
	raw, err := secure.NewToken()
	if err != nil {
		return nil, internalErr(err)
	}
	issued := &IssuedReset{StudentID: studentID, Token: raw}
	err = s.writeTx(ctx, func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, `
			SELECT s.id
			FROM students s
			JOIN accounts a ON a.id = s.account_id
			WHERE s.id = ?
			  AND s.deleted_at = ?
			  AND a.deleted_at = ?
			  AND a.role = 'student'
			FOR UPDATE`, studentID, store.LiveAt, store.LiveAt).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE password_reset_tokens SET deleted_at = UTC_TIMESTAMP()
			WHERE student_id = ? AND used_at IS NULL AND deleted_at = ?`,
			studentID, store.LiveAt); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO password_reset_tokens
				(student_id, issued_by_account_id, token_hash, expires_at)
			VALUES (?, ?, ?, DATE_ADD(UTC_TIMESTAMP(), INTERVAL 24 HOUR))`,
			studentID, actorID, secure.HashToken(raw))
		if err != nil {
			return err
		}
		tokenID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `
			SELECT expires_at FROM password_reset_tokens WHERE id = ?`,
			tokenID).Scan(&issued.ExpiresAt)
	})
	if err != nil {
		return nil, err
	}
	return issued, nil
}
