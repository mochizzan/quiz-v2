package apperr_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"quiz-v2/internal/apperr"
)

func TestKindStatus(t *testing.T) {
	cases := map[apperr.Kind]int{
		apperr.NotFound:    http.StatusNotFound,
		apperr.Forbidden:   http.StatusForbidden,
		apperr.Validation:  http.StatusUnprocessableEntity,
		apperr.RateLimited: http.StatusTooManyRequests,
		apperr.Internal:    http.StatusInternalServerError,
	}
	for kind, want := range cases {
		if got := kind.Status(); got != want {
			t.Errorf("%v.Status() = %d, want %d", kind, got, want)
		}
	}
}

func TestConstructorsCarryKindAndCopy(t *testing.T) {
	if e := apperr.NotFoundf(); e.Kind() != apperr.NotFound || e.Message() != apperr.MsgNotFound {
		t.Errorf("NotFoundf = %v/%q", e.Kind(), e.Message())
	}
	if e := apperr.Forbiddenf(""); e.Kind() != apperr.Forbidden || e.Message() != apperr.MsgCSRF {
		t.Errorf("Forbiddenf(\"\") = %v/%q, want the CSRF copy", e.Kind(), e.Message())
	}
	if e := apperr.Forbiddenf("custom"); e.Message() != "custom" {
		t.Errorf("Forbiddenf(custom) message = %q", e.Message())
	}
	if e := apperr.Field("email", "bad"); e.Kind() != apperr.Validation || e.Fields()["email"] != "bad" {
		t.Errorf("Field = %v/%v", e.Kind(), e.Fields())
	}
	if e := apperr.Internalf(errors.New("boom")); !strings.Contains(e.Error(), "internal") {
		t.Errorf("Internalf error = %q", e.Error())
	}
}

func TestRateLimitedClampsRetryAfter(t *testing.T) {
	for _, in := range []int{-5, 0} {
		if got := apperr.RateLimitedf(in).RetryAfter(); got != 1 {
			t.Errorf("RateLimitedf(%d).RetryAfter() = %d, want 1", in, got)
		}
	}
	if got := apperr.RateLimitedf(42).RetryAfter(); got != 42 {
		t.Errorf("RetryAfter() = %d, want 42", got)
	}
}

func TestWithRuleIsVisibleInError(t *testing.T) {
	e := apperr.NotFoundf().WithRule("OPS-204")
	if e.Rule() != "OPS-204" {
		t.Fatalf("Rule = %q", e.Rule())
	}
	if !strings.Contains(e.Error(), "OPS-204") {
		t.Errorf("Error() = %q, want the rule id", e.Error())
	}
}

func TestFromNormalises(t *testing.T) {
	if apperr.From(nil) != nil {
		t.Error("From(nil) must be nil")
	}
	notFound := apperr.NotFoundf()
	if got := apperr.From(notFound); got != notFound {
		t.Error("From must return a classified error unchanged")
	}
	wrapped := errors.Join(errors.New("db"), apperr.Field("x", "y"))
	if got := apperr.From(wrapped); got.Kind() != apperr.Validation {
		t.Errorf("From(wrapped) kind = %v, want validation", got.Kind())
	}
	if got := apperr.From(errors.New("boom")); got.Kind() != apperr.Internal {
		t.Errorf("From(plain) kind = %v, want internal", got.Kind())
	}
}

func TestIsNotFound(t *testing.T) {
	if !apperr.IsNotFound(apperr.NotFoundf()) {
		t.Error("IsNotFound(NotFoundf()) = false")
	}
	if apperr.IsNotFound(apperr.Forbiddenf("")) {
		t.Error("IsNotFound(Forbiddenf()) = true")
	}
}
