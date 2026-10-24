package identity

import (
	"context"
	"database/sql"
	"errors"

	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// Teachers reads the teacher directory: pending registrations oldest first,
// then active teachers by name (IDN-C21, IDN-100). Both queries run in one
// read-only snapshot so a row cannot appear in both sections.
func (s *Service) Teachers(ctx context.Context) (viewmodel.TeachersPage, error) {
	var page viewmodel.TeachersPage
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		pending, err := teacherRows(ctx, tx, viewmodel.AccountPending)
		if err != nil {
			return err
		}
		active, err := teacherRows(ctx, tx, viewmodel.AccountActive)
		if err != nil {
			return err
		}
		page = viewmodel.TeachersPage{
			Pending: pending,
			Active:  active,
			Empty: viewmodel.EmptyState{
				Title: "Tidak ada pendaftaran guru yang menunggu.",
				Body:  "Pendaftaran guru baru akan muncul di sini.",
				Icon:  "check",
			},
		}
		return nil
	})
	return page, err
}

// teacherRows loads one status section with the approver's current name, not a
// snapshot (IDN-153).
func teacherRows(ctx context.Context, q store.Querier, status viewmodel.AccountStatus) ([]viewmodel.TeacherRow, error) {
	order := "a.full_name ASC, a.id ASC"
	if status == viewmodel.AccountPending {
		order = "a.created_at ASC, a.id ASC"
	}
	rows, err := q.QueryContext(ctx, `
		SELECT a.id, a.full_name, a.email, a.status,
		       approver.full_name, a.approved_at
		FROM accounts a
		LEFT JOIN accounts approver ON approver.id = a.approved_by_account_id
		WHERE a.role = 'teacher'
		  AND a.status = ?
		  AND a.deleted_at = ?
		ORDER BY `+order, status, store.LiveAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []viewmodel.TeacherRow{}
	for rows.Next() {
		var (
			row       viewmodel.TeacherRow
			rawStatus string
			approver  sql.NullString
			approved  sql.NullTime
		)
		if err := rows.Scan(&row.ID, &row.FullName, &row.Email, &rawStatus, &approver, &approved); err != nil {
			return nil, err
		}
		row.Initials = viewmodel.Initials(row.FullName)
		row.Status = viewmodel.AccountStatus(rawStatus)
		if approver.Valid {
			row.ApprovedBy = approver.String
		}
		if approved.Valid {
			stamp := viewmodel.NewLocalTime(approved.Time)
			row.ApprovedAt = &stamp
		}
		row.ApproveHref = "/teacher/teachers/" + intString(row.ID) + "/approve"
		row.CanApprove = row.Status == viewmodel.AccountPending
		out = append(out, row)
	}
	return out, rows.Err()
}

// ApproveTeacher is the monotonic approval command (IDN-C22). The conditional
// UPDATE is the lock; zero rows is resolved to idempotent success, a wrong-role
// or missing target, or 404 rather than a generic conflict.
func (s *Service) ApproveTeacher(ctx context.Context, actorID, targetID int64) (string, error) {
	if targetID <= 0 {
		return "", ErrNotFound
	}
	flash := ""
	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE accounts
			SET status = 'active',
			    approved_at = UTC_TIMESTAMP(),
			    approved_by_account_id = ?
			WHERE id = ?
			  AND role = 'teacher'
			  AND status = 'pending'
			  AND deleted_at = ?`,
			actorID, targetID, store.LiveAt)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 1 {
			flash = FlashApproved
			return nil
		}
		var (
			role   string
			status string
		)
		err = tx.QueryRowContext(ctx, `
			SELECT role, status FROM accounts
			WHERE id = ? AND deleted_at = ?`, targetID, store.LiveAt).Scan(&role, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if role != string(viewmodel.RoleTeacher) {
			return ErrNotFound
		}
		if status == string(viewmodel.AccountActive) {
			flash = FlashAlreadyApproved
			return nil
		}
		return ErrNotFound
	})
	if err != nil {
		return "", err
	}
	return flash, nil
}
