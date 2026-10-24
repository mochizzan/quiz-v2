package identity

import (
	"errors"
	"strings"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/secure"
)

// FormError is a validation outcome carrying the exact input-name keys the
// templates render (IDN-079). The route adapter turns it into a 422 with field
// copy; the domain never renders or chooses a status.
//
// Secrets are never added to Fields. Password keys hold copy only, and the
// route's re-render leaves the input blank (IDN-061, IDN-083).
type FormError struct {
	Fields map[string]string
}

// Error implements error. The text is for logs; it is never rendered.
func (e *FormError) Error() string {
	if e == nil {
		return "identity: form invalid"
	}
	return "identity: form invalid"
}

// Err returns the FormError as an error, or nil when nothing failed. It keeps
// callers from returning a typed-nil *FormError inside an error interface.
func (e *FormError) Err() error {
	if e == nil || len(e.Fields) == 0 {
		return nil
	}
	return e
}

// add records the first message for a field. One message per input is the
// template contract; later checks do not overwrite the first failure.
func (e *FormError) add(field, message string) {
	if field == "" || message == "" {
		return
	}
	if _, exists := e.Fields[field]; exists {
		return
	}
	e.Fields[field] = message
}

// addError maps a validation sentinel through FieldMessage. Errors that are
// not field-scoped are ignored: the command's own branch handles them.
func (e *FormError) addError(err error) {
	if field, message, ok := FieldMessage(err); ok {
		e.add(field, message)
	}
}

// addPasswordPolicy maps a policy failure to the field the form renders. The
// reset form calls its new-password input `password`; change-password calls it
// `new_password`, so the field is a parameter rather than a constant.
func (e *FormError) addPasswordPolicy(field string, err error) {
	switch {
	case errors.Is(err, secure.ErrPasswordPolicy):
		e.add(field, apperr.MsgPasswordPolicy)
	case errors.Is(err, secure.ErrPasswordTooLong):
		e.add(field, apperr.MsgPasswordTooLong)
	default:
		e.addError(err)
	}
}

// fieldError builds a one-field FormError.
func fieldError(field, message string) *FormError {
	return &FormError{Fields: map[string]string{field: message}}
}

// FieldErrorsOf returns the field-keyed copy a 422 re-render carries (IDN-079).
// A FormError supplies its own fields; any other sentinel is translated through
// FieldMessage. ok is false when nothing is field-scoped, so the route adapter
// can fall through to its command-specific branch instead of re-rendering an
// empty form.
func FieldErrorsOf(err error) (map[string]string, bool) {
	var fe *FormError
	if errors.As(err, &fe) {
		return fe.Fields, true
	}
	fields := map[string]string{}
	AddFieldError(fields, err)
	if len(fields) == 0 {
		return nil, false
	}
	return fields, true
}

// ValidateLogin checks the two required fields only (IDN-C03). A malformed
// email is deliberately not a field error: it is an ordinary credential
// failure on the lookup path, so the form cannot be used as a format oracle.
func ValidateLogin(email, password string) error {
	f := &FormError{Fields: map[string]string{}}
	if strings.TrimSpace(email) == "" {
		f.add("email", apperr.MsgEmailRequired)
	}
	if password == "" {
		f.add("password", MsgPasswordRequired)
	}
	return f.Err()
}

// ValidateRegister validates the shared teacher form (IDN-C05): name, email,
// policy, and byte-equal confirmation. It returns the normalised name and email
// the insert uses.
func ValidateRegister(fullName, email, password, confirm string) (string, string, error) {
	f := &FormError{Fields: map[string]string{}}
	name, err := ValidateName(fullName)
	if err != nil {
		f.addError(err)
	}
	normalized, err := ValidateEmail(email)
	if err != nil {
		f.addError(err)
	}
	if err := ValidatePassword(password); err != nil {
		f.addPasswordPolicy("password", err)
	}
	if err := ValidateConfirm(password, confirm); err != nil {
		f.addError(err)
	}
	if err := f.Err(); err != nil {
		return "", "", err
	}
	return name, normalized, nil
}

// StudentSchool is the parsed grade/major pair a registration or edit writes.
type StudentSchool struct {
	GradeLevel int
	MajorID    int64
}

// ValidateStudentRegister validates the student form (IDN-C07): the shared
// account fields plus grade and major syntax. Whether the major is live is
// decided under the registration lock, not here.
func ValidateStudentRegister(fullName, email, password, confirm, gradeRaw, majorRaw string) (string, string, StudentSchool, error) {
	f := &FormError{Fields: map[string]string{}}
	name, err := ValidateName(fullName)
	if err != nil {
		f.addError(err)
	}
	normalized, err := ValidateEmail(email)
	if err != nil {
		f.addError(err)
	}
	if err := ValidatePassword(password); err != nil {
		f.addPasswordPolicy("password", err)
	}
	if err := ValidateConfirm(password, confirm); err != nil {
		f.addError(err)
	}
	grade, err := ParseGradeLevel(gradeRaw)
	if err != nil {
		f.addError(err)
	}
	majorID, ok := ParseID(majorRaw)
	if !ok {
		f.addError(ErrMajorRequired)
	}
	if err := f.Err(); err != nil {
		return "", "", StudentSchool{}, err
	}
	return name, normalized, StudentSchool{GradeLevel: grade, MajorID: majorID}, nil
}

// ValidateStudentEdit validates the teacher's student edit (IDN-C18). There is
// no password: a teacher can never set a student's secret.
func ValidateStudentEdit(fullName, email, gradeRaw, majorRaw string) (string, string, StudentSchool, error) {
	f := &FormError{Fields: map[string]string{}}
	name, err := ValidateName(fullName)
	if err != nil {
		f.addError(err)
	}
	normalized, err := ValidateEmail(email)
	if err != nil {
		f.addError(err)
	}
	grade, err := ParseGradeLevel(gradeRaw)
	if err != nil {
		f.addError(err)
	}
	majorID, ok := ParseID(majorRaw)
	if !ok {
		f.addError(ErrMajorRequired)
	}
	if err := f.Err(); err != nil {
		return "", "", StudentSchool{}, err
	}
	return name, normalized, StudentSchool{GradeLevel: grade, MajorID: majorID}, nil
}

// ValidateProfile validates the student's own name and email (IDN-C15). Grade
// and major are teacher-owned and are not read from this form at all.
func ValidateProfile(fullName, email string) (string, string, error) {
	f := &FormError{Fields: map[string]string{}}
	name, err := ValidateName(fullName)
	if err != nil {
		f.addError(err)
	}
	normalized, err := ValidateEmail(email)
	if err != nil {
		f.addError(err)
	}
	if err := f.Err(); err != nil {
		return "", "", err
	}
	return name, normalized, nil
}

// ChangePasswordInput is the raw change-password form.
type ChangePasswordInput struct {
	Current string
	New     string
	Confirm string
}

// ValidateChangePassword enforces presence, the policy on `new_password`, and
// byte equality (IDN-C11). The current secret is only checked for presence; an
// existing credential is not held to the current policy.
func ValidateChangePassword(in ChangePasswordInput) error {
	f := &FormError{Fields: map[string]string{}}
	if err := ValidateCurrentPassword(in.Current); err != nil {
		f.addError(err)
	}
	if err := ValidatePassword(in.New); err != nil {
		f.addPasswordPolicy("new_password", err)
	}
	if err := ValidateConfirm(in.New, in.Confirm); err != nil {
		f.addError(err)
	}
	return f.Err()
}

// ValidateResetPassword validates the token form (IDN-C09). The template names
// the new-password input `password`, so policy copy is keyed there; the spec's
// `new_password` name is a template delta recorded in the report.
func ValidateResetPassword(token, password, confirm string) (string, error) {
	f := &FormError{Fields: map[string]string{}}
	trimmed := strings.TrimSpace(token)
	switch {
	case trimmed == "":
		f.addError(ErrTokenRequired)
	case len(trimmed) > MaxTokenLength:
		f.add("token", apperr.MsgInvalidToken)
	}
	if err := ValidatePassword(password); err != nil {
		f.addPasswordPolicy("password", err)
	}
	if err := ValidateConfirm(password, confirm); err != nil {
		f.addError(err)
	}
	if err := f.Err(); err != nil {
		return "", err
	}
	return trimmed, nil
}

// LoginEmailBucket derives the rate-limit key identity (IDN-063, IDN-144).
// A valid address is normalised so casing variants share one bucket; a
// malformed one is lowered and capped so variants of the same junk share one
// bucket too. The value is never rendered.
func LoginEmailBucket(raw string) string {
	if email, err := secure.NormalizeEmail(raw); err == nil {
		return email
	}
	v := strings.ToLower(strings.TrimSpace(raw))
	if len(v) > 254 {
		v = v[:254]
	}
	return v
}
