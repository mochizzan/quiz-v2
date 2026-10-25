package identity

import (
	"context"
	"database/sql"
	"errors"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/viewmodel"
)

// errNoProfile is a data fault: a live student account without a live profile
// (INV-02). It is always a 500, never an invented class label.
var errNoProfile = errors.New("identity: student has no live profile")

// errNoTeacherAccount is a data fault: a teacher session whose account row is
// gone or no longer has role='teacher'. It is a 500, never a student form.
var errNoTeacherAccount = errors.New("identity: teacher account is missing")

// Profile reads the student's own account, profile, and the live major list
// (IDN-C14, IDN-098). Grade and major are read-only on this screen.
func (s *Service) Profile(ctx context.Context, accountID int64) (viewmodel.ProfilePage, error) {
	var page viewmodel.ProfilePage
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		profile, err := LoadProfile(ctx, tx, accountID)
		if err != nil {
			return err
		}
		if profile == nil {
			return internalErr(errNoProfile)
		}
		majors, err := LiveMajors(ctx, tx)
		if err != nil {
			return err
		}
		page = viewmodel.ProfilePage{
			FullName:   profile.FullName,
			Email:      profile.Email,
			GradeLevel: profile.GradeLevel,
			MajorID:    profile.MajorID,
			Majors:     majors,
		}
		return nil
	})
	return page, err
}

// TeacherProfile reads the teacher's own account row (IDN-C23). R-05 made
// teacher identity immutable in v1; this screen removes that exception for name
// and email only — the password still changes at IDN-C11.
func (s *Service) TeacherProfile(ctx context.Context, accountID int64) (viewmodel.TeacherProfilePage, error) {
	var page viewmodel.TeacherProfilePage
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		account, err := LoadTeacherAccount(ctx, tx, accountID)
		if err != nil {
			return err
		}
		if account == nil {
			return internalErr(errNoTeacherAccount)
		}
		page = viewmodel.TeacherProfilePage{
			FullName: account.FullName,
			Email:    account.Email,
		}
		return nil
	})
	return page, err
}

// SaveProfile writes the account's own name and email under the account row
// lock (IDN-C15, IDN-C24). The UPDATE is role-agnostic: a teacher has no
// students row, so the same statement serves both account screens. Grade,
// major, password, and must-change are not writable from this form. A revoked
// session is session_expired, not a silent write.
func (s *Service) SaveProfile(ctx context.Context, accountID int64, in ProfileInput) error {
	name, email, err := ValidateProfile(in.FullName, in.Email)
	if err != nil {
		return err
	}
	err = s.writeTx(ctx, func(tx *sql.Tx) error {
		return UpdateProfile(ctx, tx, accountID, name, email)
	})
	if errors.Is(err, ErrEmailTaken) {
		return fieldError("email", apperr.MsgEmailTaken)
	}
	return err
}

// ProfileInput is the student's own editable pair.
type ProfileInput struct {
	FullName string
	Email    string
}
