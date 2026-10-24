package identity

// Unit tests for the identity domain's pure logic: form validation, normalised
// parsers, token envelopes, copy mapping, and error classification. Everything
// here runs without a database (the storage-shaped Rules live in
// integration_test.go) and completes in constant time except the two bcrypt
// seams, which this file deliberately avoids.

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/secure"
	"quiz-v2/internal/viewmodel"
)

const testPassword = "SandiRahasia1"

// ---- Name ------------------------------------------------------------------

func TestValidateName(t *testing.T) {
	long := strings.Repeat("a", MaxNameRunes)
	tooLong := strings.Repeat("a", MaxNameRunes+1)
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr error
	}{
		{name: "empty after trim", raw: "   ", wantErr: ErrNameRequired},
		{name: "far too long", raw: strings.Repeat("a", MaxNameRunes*5), wantErr: ErrNameLong},
		{name: "exactly at ceiling", raw: long, want: long},
		{name: "one over the ceiling", raw: tooLong, wantErr: ErrNameLong},
		{name: "collapses internal whitespace", raw: "  Budi \t Santoso \n ", want: "Budi Santoso"},
		{name: "control whitespace becomes a space", raw: "De\vLa\x0bCruz", want: "De La Cruz"},
		{name: "non-space controls are dropped", raw: "A\x00B\x01C", want: "ABC"},
		{name: "normal single name", raw: "Sinta", want: "Sinta"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateName(tt.raw)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ValidateName(%q) error = %v, want %v", tt.raw, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateName(%q) unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("ValidateName(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

// ---- Email -----------------------------------------------------------------

func TestValidateEmail(t *testing.T) {
	local64 := strings.Repeat("a", 64)
	// local(64) + @ + domain(190) is one byte over the IDN-053 ceiling.
	domain190 := strings.Repeat("b.", 94) + "co"
	domain189 := strings.Repeat("b.", 94) + "c"

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr error
	}{
		{name: "normalises casing and trims", raw: "  Guru@Example.COM  ", want: "guru@example.com"},
		{name: "empty", raw: "   ", wantErr: secure.ErrEmailRequired},
		{name: "interior space", raw: "bu di@x.test", wantErr: secure.ErrEmailInvalid},
		{name: "non-ascii", raw: "budi@x säítø.test", wantErr: secure.ErrEmailInvalid},
		{name: "no at", raw: "example.com", wantErr: secure.ErrEmailInvalid},
		{name: "at first", raw: "@example.com", wantErr: secure.ErrEmailInvalid},
		{name: "at last", raw: "budi@", wantErr: secure.ErrEmailInvalid},
		{name: "two at", raw: "a@b@c.test", wantErr: secure.ErrEmailInvalid},
		{name: "domain without dot", raw: "budi@localhost", wantErr: secure.ErrEmailInvalid},
		{name: "leading local dot", raw: ".budi@x.test", wantErr: secure.ErrEmailInvalid},
		{name: "trailing local dot", raw: "budi.@x.test", wantErr: secure.ErrEmailInvalid},
		{name: "double local dot", raw: "bu..di@x.test", wantErr: secure.ErrEmailInvalid},
		{name: "leading domain dot", raw: "budi@.x.test", wantErr: secure.ErrEmailInvalid},
		{name: "trailing domain dot", raw: "budi@x.test.", wantErr: secure.ErrEmailInvalid},
		{name: "double domain dot", raw: "budi@x..test", wantErr: secure.ErrEmailInvalid},
		{name: "local part over 64", raw: local64 + "a@example.com", wantErr: secure.ErrEmailInvalid},
		{name: "local part exactly 64", raw: local64 + "@example.com", want: local64 + "@example.com"},
		{name: "one over 254 bytes", raw: local64 + "@" + domain190, wantErr: secure.ErrEmailInvalid},
		{name: "exactly 254 bytes", raw: local64 + "@" + domain189, want: local64 + "@" + domain189},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateEmail(tt.raw)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ValidateEmail(%q) error = %v, want %v", tt.raw, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateEmail(%q) unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("ValidateEmail(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

// ---- Password ---------------------------------------------------------------

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr error
	}{
		{name: "empty", raw: "", wantErr: ErrPasswordRequired},
		{name: "under the floor", raw: "Aa1b2c3", wantErr: secure.ErrPasswordPolicy},
		{name: "no digit", raw: "SandiRahasia", wantErr: secure.ErrPasswordPolicy},
		{name: "no uppercase", raw: "sandirahasia1", wantErr: secure.ErrPasswordPolicy},
		{name: "contains a space", raw: "Sandi Rahasia 1", wantErr: secure.ErrPasswordPolicy},
		{name: "at policy floor", raw: "SandiRaha1"},
		{name: "72 bytes is storable", raw: strings.Repeat("a", 70) + "A1"},
		{name: "73 bytes", raw: strings.Repeat("a", 71) + "A1", wantErr: secure.ErrPasswordTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.raw)
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidatePassword(%q) = %v, want %v", tt.raw, err, tt.wantErr)
			}
			if tt.wantErr == nil && err != nil {
				t.Fatalf("ValidatePassword(%q) unexpected error: %v", tt.raw, err)
			}
		})
	}
}

func TestValidateConfirmAnd(t *testing.T) {
	if err := ValidateConfirm("AaaBbb11", ""); !errors.Is(err, ErrConfirmRequired) {
		t.Fatalf("blank confirm error = %v, want ErrConfirmRequired", err)
	}
	if err := ValidateConfirm("AaaBbb11", "AaaBbb12"); !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("mismatch error = %v, want ErrPasswordMismatch", err)
	}
	if err := ValidateConfirm("AaaBbb11", "AaaBbb11"); err != nil {
		t.Fatalf("equal confirm error = %v, want nil", err)
	}
	if err := ValidateCurrentPassword(""); !errors.Is(err, ErrCurrentRequired) {
		t.Fatalf("blank current error = %v, want ErrCurrentRequired", err)
	}
	if err := ValidateCurrentPassword("anything"); err != nil {
		t.Fatalf("presence-only current error = %v, want nil", err)
	}
}

// ---- Roster query parsers ---------------------------------------------------

func TestParseGradeLevel(t *testing.T) {
	tests := map[string]int{"10": 10, "11": 11, "12": 12, " 11 ": 11, " 12": 12}
	for raw, want := range tests {
		got, err := ParseGradeLevel(raw)
		if err != nil {
			t.Fatalf("ParseGradeLevel(%q) error = %v", raw, err)
		}
		if got != want {
			t.Fatalf("ParseGradeLevel(%q) = %d, want %d", raw, got, want)
		}
	}
	for _, raw := range []string{"", "x", "X", "13", "9", "10.0", "12L"} {
		if _, err := ParseGradeLevel(raw); !errors.Is(err, ErrGradeRequired) {
			t.Fatalf("ParseGradeLevel(%q) error = %v, want ErrGradeRequired", raw, err)
		}
	}
}

func TestParseGradeFilter(t *testing.T) {
	tests := map[string]int{
		"10": 10, "X": 10, "x": 10,
		"11": 11, "XI": 11, "xi": 11,
		"12": 12, "XII": 12, "xii": 12,
		"kelas 10": 10, "KELAS XII": 12, " kelas x ": 10,
		"": 0, "kelas": 0, "nine": 0, "13": 0,
		"kelasx": 10, // TrimPrefix runs before TrimSpace, so the solid spelling parses too
	}
	for raw, want := range tests {
		if got := ParseGradeFilter(raw); got != want {
			t.Fatalf("ParseGradeFilter(%q) = %d, want %d", raw, got, want)
		}
	}
}

func TestParseID(t *testing.T) {
	if id, ok := ParseID(" 42 "); !ok || id != 42 {
		t.Fatalf("ParseID(\" 42 \") = %d, %v", id, ok)
	}
	for _, raw := range []string{"", "0", "-7", "abc", "1e3", "92233720368547758080"} {
		if _, ok := ParseID(raw); ok {
			t.Fatalf("ParseID(%q) accepted", raw)
		}
	}
}

func TestParseSortAndPage(t *testing.T) {
	tests := map[string]viewmodel.SortOrder{
		"oldest": viewmodel.SortOldest, "name_asc": viewmodel.SortNameAsc,
		"name_desc": viewmodel.SortNameDesc, "newest": viewmodel.SortNewest,
		"": viewmodel.SortNewest, "NAME_ASC": viewmodel.SortNewest, "zzz": viewmodel.SortNewest,
	}
	for raw, want := range tests {
		if got := ParseSort(raw); got != want {
			t.Fatalf("ParseSort(%q) = %v, want %v", raw, got, want)
		}
	}
	if got := ParsePage("7"); got != 7 {
		t.Fatalf("ParsePage(7) = %d", got)
	}
	for _, raw := range []string{"0", "-2", "abc", ""} {
		if got := ParsePage(raw); got != 1 {
			t.Fatalf("ParsePage(%q) = %d, want 1", raw, got)
		}
	}
}

func TestNormalizeSearch(t *testing.T) {
	if got := NormalizeSearch("  budi  "); got != "budi" {
		t.Fatalf("NormalizeSearch = %q", got)
	}
	if got := NormalizeSearch("  "); got != "" {
		t.Fatalf("NormalizeSearch(blank) = %q", got)
	}
	if got := NormalizeSearch(strings.Repeat("q", MaxSearchRunes+1)); got != "" {
		t.Fatalf("NormalizeSearch(over ceiling) = %q, want dropped", got)
	}
	if got := NormalizeSearch(strings.Repeat("q", MaxSearchRunes)); len(got) != MaxSearchRunes {
		t.Fatalf("NormalizeSearch(at ceiling) = %d runes", len(got))
	}
}

func TestEscapeLike(t *testing.T) {
	tests := map[string]string{
		`budi`:      `budi`,
		`100% ok`:   `100\% ok`,
		`a_b`:       `a\_b`,
		`c:\path`:   `c:\\path`,
		`%\_`:       `\%\\\_`,
		`already\%`: `already\\\%`,
	}
	for raw, want := range tests {
		if got := EscapeLike(raw); got != want {
			t.Fatalf("EscapeLike(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestPageMath(t *testing.T) {
	if got := PageCount(0, 0); got != 1 {
		t.Fatalf("PageCount(0,0) = %d, want 1", got)
	}
	if got := PageCount(10, 10); got != 1 {
		t.Fatalf("PageCount(10,10) = %d, want 1", got)
	}
	if got := PageCount(11, 10); got != 2 {
		t.Fatalf("PageCount(11,10) = %d, want 2", got)
	}
	if got := PageCount(95, 10); got != 10 {
		t.Fatalf("PageCount(95,10) = %d, want 10", got)
	}
	if got := PageCount(7, 0); got != PageCount(7, PageSize) {
		t.Fatalf("PageCount(7,0) must fall back to PageSize")
	}
	if got := ClampPage(-5, 25, 10); got != 1 {
		t.Fatalf("ClampPage(-5,…) = %d, want 1", got)
	}
	if got := ClampPage(3, 25, 10); got != 3 {
		t.Fatalf("ClampPage(3,…) = %d, want 3", got)
	}
	if got := ClampPage(9, 25, 10); got != 3 {
		t.Fatalf("ClampPage(9,25,10) = %d, want 3", got)
	}
	if got := ClampPage(4, 0, 0); got != 1 {
		t.Fatalf("ClampPage(4,0) = %d, want 1", got)
	}
}

// ---- Field message mapping --------------------------------------------------

func TestFieldMessage(t *testing.T) {
	tests := []struct {
		err   error
		field string
		msg   string
	}{
		{ErrNameRequired, "full_name", MsgNameRequired},
		{ErrNameLong, "full_name", MsgNameLong},
		{secure.ErrEmailRequired, "email", apperr.MsgEmailRequired},
		{secure.ErrEmailInvalid, "email", apperr.MsgEmailInvalid},
		{ErrPasswordRequired, "password", MsgPasswordRequired},
		{secure.ErrPasswordPolicy, "password", apperr.MsgPasswordPolicy},
		{secure.ErrPasswordTooLong, "password", apperr.MsgPasswordTooLong},
		{ErrConfirmRequired, "password_confirm", MsgConfirmRequired},
		{ErrPasswordMismatch, "password_confirm", apperr.MsgPasswordMismatch},
		{ErrCurrentRequired, "current_password", MsgCurrentRequired},
		{ErrGradeRequired, "grade_level", MsgGradeRequired},
		{ErrMajorRequired, "major_id", MsgMajorRequired},
		{ErrTokenRequired, "token", MsgTokenRequired},
		{ErrTokenInvalid, "token", apperr.MsgInvalidToken},
		{ErrTokenUsed, "token", apperr.MsgTokenUsed},
		{ErrEmailTaken, "email", apperr.MsgEmailTaken},
		{ErrWrongPassword, "current_password", apperr.MsgCurrentPassword},
		{ErrSamePassword, "new_password", apperr.MsgSamePassword},
	}
	for _, tt := range tests {
		field, msg, ok := FieldMessage(tt.err)
		if !ok {
			t.Fatalf("FieldMessage(%v) not mapped", tt.err)
		}
		if field != tt.field || msg != tt.msg {
			t.Fatalf("FieldMessage(%v) = (%q, %q), want (%q, %q)", tt.err, field, msg, tt.field, tt.msg)
		}
	}
	if _, _, ok := FieldMessage(errors.New("unmapped")); ok {
		t.Fatal("FieldMessage(unmapped) claimed ok")
	}
	if _, _, ok := FieldMessage(ErrInvalidCredentials); ok {
		t.Fatal("FieldMessage(invalid credentials) must not be field-scoped")
	}
}

func TestAddFieldErrorIgnoresUnmapped(t *testing.T) {
	fields := map[string]string{}
	AddFieldError(fields, errors.New("boom"))
	AddFieldError(fields, ErrEmailTaken)
	if len(fields) != 1 || fields["email"] != apperr.MsgEmailTaken {
		t.Fatalf("AddFieldError = %v", fields)
	}
}

// ---- FormError --------------------------------------------------------------

func TestFormErrorErrAndNilSafety(t *testing.T) {
	var e *FormError
	if e.Err() != nil {
		t.Fatal("nil FormError.Err() must be nil (typed-nil guard)")
	}
	if got := e.Error(); got == "" {
		t.Fatal("nil FormError.Error() must not panic")
	}
	if (&FormError{}).Err() != nil {
		t.Fatal("empty FormError.Err() must be nil")
	}
	fe := &FormError{Fields: map[string]string{"full_name": MsgNameRequired}}
	fe.add("full_name", MsgNameLong) // first message wins
	if fe.Fields["full_name"] != MsgNameRequired {
		t.Fatalf("later add overwrote the first failure: %v", fe.Fields)
	}
	fe.add("", "x") // silent no-op
	if len(fe.Fields) != 1 {
		t.Fatalf("blank field add recorded: %v", fe.Fields)
	}
}

func TestFieldErrorsOf(t *testing.T) {
	fe := &FormError{Fields: map[string]string{"email": apperr.MsgEmailTaken}}
	wrapped := fmt.Errorf("register: %w", fe)
	fields, ok := FieldErrorsOf(wrapped)
	if !ok || fields["email"] != apperr.MsgEmailTaken {
		t.Fatalf("FieldErrorsOf(wrapped FormError) = %v, %v", fields, ok)
	}

	fields, ok = FieldErrorsOf(ErrEmailTaken)
	if !ok || fields["email"] != apperr.MsgEmailTaken {
		t.Fatalf("FieldErrorsOf(ErrEmailTaken) = %v, %v", fields, ok)
	}

	if fields, ok := FieldErrorsOf(ErrInvalidCredentials); ok || fields != nil {
		t.Fatalf("FieldErrorsOf(invalid credentials) = %v, %v; want nil,false", fields, ok)
	}
	if fields, ok := FieldErrorsOf(errors.New("db down")); ok || fields != nil {
		t.Fatalf("FieldErrorsOf(unknown) = %v, %v; want nil,false", fields, ok)
	}
	if fields, ok := FieldErrorsOf(nil); ok || fields != nil {
		t.Fatalf("FieldErrorsOf(nil) = %v, %v; want nil,false", fields, ok)
	}
}

// ---- Form validators --------------------------------------------------------

func requireFormError(t *testing.T, err error) *FormError {
	t.Helper()
	if err == nil {
		t.Fatal("expected a FormError, got nil")
	}
	var fe *FormError
	if !errors.As(err, &fe) {
		t.Fatalf("error %v (%T) is not a *FormError", err, err)
	}
	return fe
}

func TestValidateLogin(t *testing.T) {
	if err := ValidateLogin("budi@x.test", "secret"); err != nil {
		t.Fatalf("valid login error = %v", err)
	}
	fe := requireFormError(t, ValidateLogin("", ""))
	if fe.Fields["email"] != apperr.MsgEmailRequired || fe.Fields["password"] != MsgPasswordRequired {
		t.Fatalf("ValidateLogin(empty) = %v", fe.Fields)
	}
	// A malformed address is deliberately not a field error (IDN-060): the
	// lookup path, not the form, owns that verdict.
	if err := ValidateLogin("not an email", "secret"); err != nil {
		t.Fatalf("malformed email must pass form validation: %v", err)
	}
}

func TestValidateRegister(t *testing.T) {
	name, email, err := ValidateRegister("  Budi  ", "Budi@X.test", testPassword, testPassword)
	if err != nil {
		t.Fatalf("valid register error = %v", err)
	}
	if name != "Budi" || email != "budi@x.test" {
		t.Fatalf("normalisation lost: %q %q", name, email)
	}

	fe := requireFormError(t, func() error { _, _, err := ValidateRegister("", "", "", ""); return err }())
	for _, field := range []string{"full_name", "email", "password", "password_confirm"} {
		if fe.Fields[field] == "" {
			t.Fatalf("ValidateRegister(empty) missing %q: %v", field, fe.Fields)
		}
	}
	if fe := requireFormError(t, func() error { _, _, err := ValidateRegister("Budi", "budi@x.test", "Short", ""); return err }()); fe.Fields["password"] == "" {
		t.Fatalf("weak password not field-scoped: %v", fe)
	}
	// The over-long password keys policy copy on `password`; a simultaneous
	// confirm mismatch stays on its own field (first-wins per field).
	p80 := strings.Repeat("a", 80)
	fe = requireFormError(t, func() error {
		_, _, err := ValidateRegister("Budi", "budi@x.test", p80, p80)
		return err
	}())
	if fe.Fields["password"] != apperr.MsgPasswordTooLong {
		t.Fatalf("over-long password must carry policy copy: %v", fe.Fields)
	}
	fe = requireFormError(t, func() error {
		_, _, err := ValidateRegister("Budi", "budi@x.test", p80, "OtherPass1")
		return err
	}())
	if fe.Fields["password"] != apperr.MsgPasswordTooLong || fe.Fields["password_confirm"] == "" {
		t.Fatalf("policy and mismatch are per-field: %v", fe.Fields)
	}
}

func TestValidateStudentRegister(t *testing.T) {
	name, email, school, err := ValidateStudentRegister("Budi", "budi@x.test", testPassword, testPassword, "10", "7")
	if err != nil {
		t.Fatalf("valid student register error = %v", err)
	}
	if name != "Budi" || email != "budi@x.test" || school != (StudentSchool{GradeLevel: 10, MajorID: 7}) {
		t.Fatalf("student register parse: %q %q %v", name, email, school)
	}

	fe := requireFormError(t, func() error {
		_, _, _, err := ValidateStudentRegister("", "", "", "", "", "")
		return err
	}())
	for _, field := range []string{"full_name", "email", "password", "password_confirm", "grade_level", "major_id"} {
		if fe.Fields[field] == "" {
			t.Fatalf("empty student form missing %q: %v", field, fe.Fields)
		}
	}
	fe = requireFormError(t, func() error {
		_, _, _, err := ValidateStudentRegister("Budi", "budi@x.test", testPassword, testPassword, "9", "7")
		return err
	}())
	if fe.Fields["grade_level"] != MsgGradeRequired {
		t.Fatalf("grade 9 must be rejected at grade_level: %v", fe.Fields)
	}
}

func TestValidateStudentEdit(t *testing.T) {
	_, _, school, err := ValidateStudentEdit("Budi", "budi@x.test", "12", "3")
	if err != nil || school != (StudentSchool{GradeLevel: 12, MajorID: 3}) {
		t.Fatalf("valid edit: %v %v", school, err)
	}
	fe := requireFormError(t, func() error {
		_, _, _, err := ValidateStudentEdit("", "bad email", "x", "")
		return err
	}())
	for _, field := range []string{"full_name", "email", "grade_level", "major_id"} {
		if fe.Fields[field] == "" {
			t.Fatalf("empty edit missing %q: %v", field, fe.Fields)
		}
	}
}

func TestValidateProfile(t *testing.T) {
	name, email, err := ValidateProfile("Budi", "budi@x.test")
	if err != nil || name != "Budi" || email != "budi@x.test" {
		t.Fatalf("valid profile: %q %q %v", name, email, err)
	}
	fe := requireFormError(t, func() error { _, _, err := ValidateProfile("", "no-at"); return err }())
	if fe.Fields["full_name"] == "" || fe.Fields["email"] == "" {
		t.Fatalf("profile fields: %v", fe.Fields)
	}
}

func TestValidateChangePassword(t *testing.T) {
	valid := ChangePasswordInput{Current: "OldPass1", New: testPassword, Confirm: testPassword}
	if err := ValidateChangePassword(valid); err != nil {
		t.Fatalf("valid change error = %v", err)
	}
	tests := []struct {
		name  string
		in    ChangePasswordInput
		field string
	}{
		{name: "blank current", in: ChangePasswordInput{New: testPassword, Confirm: testPassword}, field: "current_password"},
		{name: "weak new", in: ChangePasswordInput{Current: "OldPass1", New: "weak", Confirm: "weak"}, field: "new_password"},
		{name: "over-long new", in: ChangePasswordInput{Current: "OldPass1", New: strings.Repeat("a", 73), Confirm: strings.Repeat("a", 73)}, field: "new_password"},
		{name: "blank confirm", in: ChangePasswordInput{Current: "OldPass1", New: testPassword}, field: "password_confirm"},
		{name: "mismatch", in: ChangePasswordInput{Current: "OldPass1", New: testPassword, Confirm: "OtherPass1"}, field: "password_confirm"},
	}
	for _, tt := range tests {
		fe := requireFormError(t, ValidateChangePassword(tt.in))
		if fe.Fields[tt.field] == "" {
			t.Fatalf("%s: expected %q in %v", tt.name, tt.field, fe.Fields)
		}
	}
}

func TestValidateResetPassword(t *testing.T) {
	longToken := strings.Repeat("z", MaxTokenLength+1)
	token, err := ValidateResetPassword("  tok-123  ", testPassword, testPassword)
	if err != nil || token != "tok-123" {
		t.Fatalf("valid reset: %q %v", token, err)
	}
	tests := []struct {
		name  string
		token string
		field string
	}{
		{name: "blank token", token: "  ", field: "token"},
		{name: "over-long token", token: longToken, field: "token"},
	}
	for _, tt := range tests {
		fe := requireFormError(t, func() error {
			_, err := ValidateResetPassword(tt.token, testPassword, testPassword)
			return err
		}())
		if fe.Fields[tt.field] == "" {
			t.Fatalf("%s: expected %q in %v", tt.name, tt.field, fe.Fields)
		}
	}
	if fe := requireFormError(t, func() error {
		_, err := ValidateResetPassword("tok", "weak", "weak")
		return err
	}()); fe.Fields["password"] == "" {
		t.Fatalf("weak reset password not field-scoped under `password`: %v", fe)
	}
}

func TestLoginEmailBucket(t *testing.T) {
	valid := "Foo@Bar.Com"
	if got := LoginEmailBucket(valid); got != "foo@bar.com" {
		t.Fatalf("valid bucket = %q", got)
	}
	if LoginEmailBucket("foo@bar.com") != LoginEmailBucket(valid) {
		t.Fatal("casing variants must share one bucket")
	}
	junk := " NOT AN EMAIL "
	if got := LoginEmailBucket(junk); got != "not an email" {
		t.Fatalf("junk bucket = %q", got)
	}
	over := strings.Repeat("x", 300)
	if got := LoginEmailBucket(over); len(got) > 254 {
		t.Fatalf("junk bucket leaked %d bytes", len(got))
	}
}

// ---- Tokens -----------------------------------------------------------------

func mustSigner(t *testing.T) *secure.Signer {
	t.Helper()
	signer, err := secure.NewSigner([]byte("identity-test-signing-key-at-least-32b"))
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	return signer
}

func TestPendingTokenRoundTrip(t *testing.T) {
	signer := mustSigner(t)
	now := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)

	raw, err := NewPendingToken(signer, 42, now)
	if err != nil {
		t.Fatalf("NewPendingToken: %v", err)
	}
	id, err := ParsePendingToken(signer, raw, now)
	if err != nil || id != 42 {
		t.Fatalf("ParsePendingToken = %d, %v", id, err)
	}

	// One byte under the window parses; at the window it has expired.
	if _, err := ParsePendingToken(signer, raw, now.Add(PendingTokenTTL-time.Second)); err != nil {
		t.Fatalf("within-window parse: %v", err)
	}
	if _, err := ParsePendingToken(signer, raw, now.Add(PendingTokenTTL)); !errors.Is(err, secure.ErrTokenExpired) {
		t.Fatalf("boundary parse error = %v, want ErrTokenExpired", err)
	}

	// Foreign key, foreign purpose, truncated signature, garbage: all invalid.
	other, _ := secure.NewSigner([]byte("another-signing-key-still-32-bytes-!"))
	if _, err := ParsePendingToken(other, raw, now); !errors.Is(err, secure.ErrTokenInvalid) {
		t.Fatalf("foreign-key parse = %v", err)
	}
	reveal, _ := NewRevealToken(signer, Reveal{Teacher: 1, Student: 2, Token: raw, ExpiresAt: now.Unix()}, now)
	if _, err := ParsePendingToken(signer, reveal, now); !errors.Is(err, secure.ErrTokenInvalid) {
		t.Fatalf("cross-purpose parse = %v, want invalid", err)
	}
	if _, err := ParsePendingToken(signer, raw[:len(raw)-2], now); err == nil {
		t.Fatal("truncated signature accepted")
	}

	// accountID transports as exactly 8 big-endian bytes; zero is rejected.
	if raw, err = signer.Token(pendingTokenPurpose, []byte{1}, now); err != nil {
		t.Fatalf("short payload token: %v", err)
	}
	if _, err := ParsePendingToken(signer, raw, now); !errors.Is(err, secure.ErrTokenInvalid) {
		t.Fatalf("short payload parse = %v", err)
	}
	if raw, err = NewPendingToken(signer, 0, now); err != nil {
		t.Fatalf("zero-id token: %v", err)
	}
	if _, err := ParsePendingToken(signer, raw, now); !errors.Is(err, secure.ErrTokenInvalid) {
		t.Fatalf("zero-id parse = %v", err)
	}
}

func TestRevealTokenRoundTrip(t *testing.T) {
	signer := mustSigner(t)
	now := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	original := Reveal{Purpose: revealTokenPurpose, Teacher: 7, Student: 13, Token: "tok-value", ExpiresAt: now.Add(time.Hour).Unix()}

	raw, err := NewRevealToken(signer, original, now)
	if err != nil {
		t.Fatalf("NewRevealToken: %v", err)
	}
	got, err := ParseRevealToken(signer, raw, now)
	if err != nil || *got != original {
		t.Fatalf("ParseRevealToken = %+v, %v", got, err)
	}

	// Inside the five-minute window; expired at it.
	if _, err := ParseRevealToken(signer, raw, now.Add(RevealTokenTTL-time.Second)); err != nil {
		t.Fatalf("within-window parse: %v", err)
	}
	if _, err := ParseRevealToken(signer, raw, now.Add(RevealTokenTTL)); err == nil {
		t.Fatal("expired reveal accepted")
	}

	// Malformed JSON behind a valid signature is invalid, not a panic.
	bad, _ := signer.Token(revealTokenPurpose, []byte("not json"), now)
	if _, err := ParseRevealToken(signer, bad, now); !errors.Is(err, secure.ErrTokenInvalid) {
		t.Fatalf("bad-JSON reveal = %v", err)
	}

	// A payload whose inner purpose disagrees is rejected (R-23 domain split).
	inner, _ := json.Marshal(Reveal{Purpose: "pending", Teacher: 1, Student: 2, Token: "t", ExpiresAt: 1})
	foreign, _ := signer.Token(revealTokenPurpose, inner, now)
	if _, err := ParseRevealToken(signer, foreign, now); !errors.Is(err, secure.ErrTokenInvalid) {
		t.Fatalf("foreign inner purpose = %v", err)
	}

	// Every zero field rejects.
	for _, broken := range []Reveal{
		{Teacher: 0, Student: 1, Token: "t", ExpiresAt: 1},
		{Teacher: 1, Student: 0, Token: "t", ExpiresAt: 1},
		{Teacher: 1, Student: 1, Token: "", ExpiresAt: 1},
		{Teacher: 1, Student: 1, Token: "t", ExpiresAt: 0},
	} {
		payload, _ := json.Marshal(broken)
		raw, err := signer.Token(revealTokenPurpose, payload, now)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		if _, err := ParseRevealToken(signer, raw, now); !errors.Is(err, secure.ErrTokenInvalid) {
			t.Fatalf("reveal %+v accepted", broken)
		}
	}
}

// ---- Classification and copy ------------------------------------------------

func TestClassified(t *testing.T) {
	business := []error{
		ErrInvalidCredentials, ErrPendingTeacher, ErrNotFound,
		ErrTokenInvalid, ErrTokenUsed, ErrWrongPassword, ErrSamePassword,
		ErrSessionGone, ErrEmailTaken,
		secure.ErrEmailRequired, secure.ErrEmailInvalid,
		secure.ErrPasswordPolicy, secure.ErrPasswordTooLong,
	}
	for _, err := range business {
		got := classified(err)
		if !errors.Is(got, err) {
			t.Fatalf("classified(%v) lost the sentinel: %v", err, got)
		}
	}

	formErr := fieldError("email", apperr.MsgEmailTaken)
	var form *FormError
	if !errors.As(classified(formErr), &form) {
		t.Fatal("classified lost a FormError")
	}
	if !errors.As(classified(fmt.Errorf("wrap: %w", formErr)), &form) {
		t.Fatal("classified lost a wrapped FormError")
	}

	existing := apperr.NotFoundf()
	if !errors.As(classified(existing), new(*apperr.Error)) {
		t.Fatal("classified lost an apperr.Error")
	}

	unknown := errors.New("driver text")
	got, ok := classified(unknown).(*apperr.Error)
	if !ok || got.Kind() != apperr.Internal {
		t.Fatalf("classified(unknown) = %v, want internal apperr", classified(unknown))
	}
	if errors.Is(classified(unknown), ErrNotFound) {
		t.Fatal("internal wrapping must not claim business sentinels")
	}
	if classified(nil) != nil {
		t.Fatal("classified(nil) must be nil")
	}
}

func TestInternalErrCarriesRule(t *testing.T) {
	got := internalErr(errors.New("x")).WithRule("OPS-215")
	if got.Rule() != "OPS-215" || got.Kind() != apperr.Internal {
		t.Fatalf("internalErr rule/kind = %q/%v", got.Rule(), got.Kind())
	}
}

func TestFlashForKey(t *testing.T) {
	if f := FlashForKey(FlashDeletedStudent); f == nil || f.Title != MsgDeletedStudent || f.Kind != "success" {
		t.Fatalf("deleted_student flash = %+v", f)
	}
	if f := FlashForKey(FlashAlreadyApproved); f == nil || f.Title != MsgAlreadyApproved || f.Kind != "info" {
		t.Fatalf("already_approved flash = %+v", f)
	}
	if f := FlashForKey("welcome"); f == nil || f.Title != "Berhasil masuk." {
		t.Fatalf("welcome flash = %+v", f)
	}
	if f := FlashForKey("made-up-key"); f != nil {
		t.Fatalf("unknown flash key reflected: %+v", f)
	}
}

// ---- Seams, keys, constants -------------------------------------------------

func TestSpendDummyCompareSeam(t *testing.T) {
	// Production binds the shared bcrypt helper (IDN-060); integration tests
	// swap the seam to a counter.
	if reflect.ValueOf(spendDummyCompare).Pointer() != reflect.ValueOf(secure.SpendDummyCompare).Pointer() {
		t.Fatal("spendDummyCompare must default to secure.SpendDummyCompare")
	}
}

func TestRateBudgets(t *testing.T) {
	if RateWindow != 15*time.Minute {
		t.Fatalf("RateWindow = %v", RateWindow)
	}
	if LoginIPLimit != 30 || LoginEmailLimit != 5 || RegisterLimit != 5 ||
		ResetConsumeLimit != 10 || ChangePasswordLimit != 5 || ResetIssueLimit != 10 {
		t.Fatal("rate budgets drifted from IDN-063")
	}
}

func TestRateKeysAreNamespaced(t *testing.T) {
	keys := []string{
		LoginIPKey("1.2.3.4"),
		LoginEmailKey("a@b.test", "1.2.3.4"),
		RegisterTeacherKey("1.2.3.4"),
		RegisterStudentKey("1.2.3.4"),
		ResetConsumeKey("1.2.3.4"),
		ChangePasswordKey(9),
		ResetIssueKey(9),
	}
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] {
			t.Fatalf("colliding key %q", k)
		}
		seen[k] = true
	}
	if !strings.HasPrefix(ChangePasswordKey(12), "change-password:12") {
		t.Fatalf("account key format: %q", ChangePasswordKey(12))
	}
	if !strings.HasPrefix(ResetIssueKey(3), "reset-issue:3") {
		t.Fatalf("issuer key format: %q", ResetIssueKey(3))
	}
}

func TestClosedConstants(t *testing.T) {
	if CookiePending != "quiz_pending" || CookieReveal != "quiz_reset_reveal" {
		t.Fatal("cookie names drifted from IDN-067")
	}
	if PendingCookieMaxAge != 86400 || RevealCookieMaxAge != 300 {
		t.Fatal("cookie lifetimes drifted from R-23/IDN-105")
	}
	if PendingCookiePath != "/" || RevealCookiePath != "/teacher/students" {
		t.Fatal("cookie paths drifted")
	}
	if MinPasswordLength != 8 || MaxTokenLength != 128 || PageSize != 10 {
		t.Fatal("fixed limits drifted")
	}
	if PendingTokenTTL != 24*time.Hour || RevealTokenTTL != 5*time.Minute {
		t.Fatal("token windows drifted")
	}
	if FlashApproved != "approved" || FlashAlreadyApproved != "already_approved" ||
		FlashDeletedStudent != "deleted_student" || FlashSaved != "saved" ||
		FlashToken != "token" || FlashPasswordReset != "password-reset" ||
		FlashLoggedOut != "logout" || FlashWelcome != "welcome" ||
		FlashMustChange != "must_change" || FlashSessionExpired != "session_expired" {
		t.Fatal("flash keys drifted from IDN-080")
	}
}
