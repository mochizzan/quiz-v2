package httpserver

import (
	"testing"
)

// TestRootRedirectsToLogin pins the one static redirect in the surface.
func TestRootRedirectsToLogin(t *testing.T) {
	var root *PageRoute
	for _, p := range RegisteredPages() {
		if p.Method == "GET" && p.Path == "/" {
			route := p
			root = &route
			break
		}
	}
	if root == nil {
		t.Fatal("GET / is not registered")
	}
	if root.Redirect != "/login" {
		t.Errorf("GET / redirect = %q, want /login", root.Redirect)
	}
}

// TestRegisteredPagesIsCopy pins the defensive copy: mutating the returned
// slice must not corrupt the package registry seen by later calls.
func TestRegisteredPagesIsCopy(t *testing.T) {
	first := RegisteredPages()
	if len(first) == 0 {
		t.Fatal("RegisteredPages() is empty")
	}
	original := first[0]
	originalLen := len(first)
	first[0] = PageRoute{Method: "POST", Path: "/__mutated"}
	second := RegisteredPages()
	if len(second) != originalLen {
		t.Fatalf("len = %d after mutating copy, want %d", len(second), originalLen)
	}
	if second[0].Method != original.Method || second[0].Path != original.Path || second[0].Template != original.Template {
		t.Errorf("registry mutated through the copy: got %s %s (%s), want %s %s (%s)",
			second[0].Method, second[0].Path, second[0].Template,
			original.Method, original.Path, original.Template)
	}
}

// TestRouteLessOrdersStaticBeforeParams guards the Fiber registration order:
// static paths must beat parameterised paths, and deeper parameterised paths
// beat their prefixes.
func TestRouteLessOrdersStaticBeforeParams(t *testing.T) {
	cases := []struct {
		name string
		a, b PageRoute
	}{
		{
			name: "static new before id",
			a:    PageRoute{Method: "GET", Path: "/teacher/quizzes/new"},
			b:    PageRoute{Method: "GET", Path: "/teacher/quizzes/:id"},
		},
		{
			name: "results before id",
			a:    PageRoute{Method: "GET", Path: "/teacher/quizzes/:id/results"},
			b:    PageRoute{Method: "GET", Path: "/teacher/quizzes/:id"},
		},
		{
			name: "questions qid before questions new? static wins",
			a:    PageRoute{Method: "GET", Path: "/teacher/quizzes/:id/questions/new"},
			b:    PageRoute{Method: "GET", Path: "/teacher/quizzes/:id/questions/:qid"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !routeLess(tc.a, tc.b) {
				t.Errorf("routeLess(%q, %q) = false, want true", tc.a.Path, tc.b.Path)
			}
			if routeLess(tc.b, tc.a) {
				t.Errorf("routeLess(%q, %q) = true, want false", tc.b.Path, tc.a.Path)
			}
		})
	}
}

// TestWithParamSubstitutesAllTokens pins the PRG helper.
func TestWithParamSubstitutesAllTokens(t *testing.T) {
	got := withParam("/teacher/quizzes/:id/questions/:qid", PageRequest{
		Params: map[string]string{"id": "7", "qid": "702"},
	})
	if want := "/teacher/quizzes/7/questions/702"; got != want {
		t.Errorf("withParam = %q, want %q", got, want)
	}
}

// TestActionAppendsFlash covers the query separator cases.
func TestActionAppendsFlash(t *testing.T) {
	if got := action("/student", "welcome")(PageRequest{}); got != "/student?flash=welcome" {
		t.Errorf("action = %q, want /student?flash=welcome", got)
	}
	if got := action("/join?state=unknown", "x")(PageRequest{}); got != "/join?state=unknown&flash=x" {
		t.Errorf("action = %q, want /join?state=unknown&flash=x", got)
	}
}
