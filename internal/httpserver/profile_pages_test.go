package httpserver_test

// profile_pages_test.go renders the two account screens directly from
// ./templates and pins the web contract that DESIGN §7 defines for them: back
// is a quiet link labelled with its destination, first in the reading order
// above the page header; each form posts to its own role's endpoint; every
// button that does not submit is an outline link. Rendering through
// NewViewLoader (not the app) keeps the test free of DB, session and CSRF
// plumbing — it only covers the template, which is the surface that has no
// transport test of its own.

import (
	"strings"
	"testing"

	"quiz-v2/internal/httpserver"
	"quiz-v2/internal/viewmodel"
)

// renderPage executes one page template from the repository's ./templates
// directory. TestMain chdirs to the repository root, so the path resolves.
func renderPage(t *testing.T, name string, data viewmodel.PageData) string {
	t.Helper()
	var buf strings.Builder
	if err := httpserver.NewViewLoader("templates").Render(&buf, name, data); err != nil {
		t.Fatalf("Render(%s): %v", name, err)
	}
	return buf.String()
}

// checkBackAffordance asserts the DESIGN §7 navigation rule: the back
// affordance is a link-quiet element carrying an arrow icon and the
// destination's name, it sits between the page-stack opening and the page
// header, and it is never a button.
func checkBackAffordance(t *testing.T, html, href, label string) {
	t.Helper()
	stack := strings.Index(html, `<div class="page-stack">`)
	head := strings.Index(html, `<header class="page-head">`)
	if stack < 0 || head < 0 || head < stack {
		t.Fatalf("page must open .page-stack before .page-head (stack=%d head=%d)", stack, head)
	}
	before := html[stack:head]
	if !strings.Contains(before, `href="`+href+`"`) {
		t.Errorf("back link to %s missing above the page header", href)
	}
	if !strings.Contains(before, "link-quiet") {
		t.Error("back affordance must use .link-quiet, not a button or bare text")
	}
	if !strings.Contains(before, "#i-arrow-left") {
		t.Error("back affordance must carry the 16px i-arrow-left icon")
	}
	if !strings.Contains(before, label) {
		t.Errorf("back link must be labelled %q", label)
	}
	if strings.Contains(before, "<button") {
		t.Error("back affordance must be a link, never a button")
	}
	if strings.Contains(html, ">Kembali<") {
		t.Error("back label must name its destination, never a bare \"Kembali\"")
	}
}

// checkOnePrimaryButton asserts a page region holds exactly one solid button.
// Scope is <main> (the page surface) so the app chrome — the navbar and
// drawer call-to-action plus the confirm dialog — is not counted; the rule is
// one primary action per screen region, not per document.
func checkOnePrimaryButton(t *testing.T, html string) {
	t.Helper()
	start := strings.Index(html, "<main>")
	end := strings.Index(html, "</main>")
	if start < 0 || end < start {
		t.Fatalf("rendered document has no <main> region (start=%d end=%d)", start, end)
	}
	if n := strings.Count(html[start:end], "btn-solid"); n != 1 {
		t.Errorf("<main> has %d solid buttons, want exactly 1 primary action", n)
	}
}

func TestTeacherProfilePageRenders(t *testing.T) {
	const (
		name  = "Budi Santoso"
		email = "guru@sekolah.sch.id"
		csrf  = "csrf-token-123"
	)
	html := renderPage(t, "pages/teacher/profile.html", viewmodel.PageData{
		Base: viewmodel.Base{
			Title:  "Profil",
			Active: "profil",
			User: &viewmodel.CurrentUser{
				ID:       1,
				FullName: name,
				Email:    email,
				Role:     viewmodel.RoleTeacher,
				Initials: "BS",
			},
			CSRF: csrf,
		},
		Data: viewmodel.TeacherProfilePage{FullName: name, Email: email},
	})

	// Teacher chrome: the role badge, the teacher nav, and the avatar pill
	// marked as the account link's current page.
	for _, want := range []string{
		`<title>Profil — Twisty Quiz</title>`,
		`<span class="app-badge-role">Guru</span>`,
		`href="/teacher/quizzes"`,
		`aria-label="Profil akun"`,
		`href="/teacher/profile" aria-current="page"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("teacher profile missing %q", want)
		}
	}

	checkBackAffordance(t, html, "/teacher", "Kembali ke dasbor")

	// The form posts to the teacher endpoint only, carries the CSRF token, and
	// echoes the current values so a 422 re-render loses nothing.
	for _, want := range []string{
		`method="post" action="/teacher/profile"`,
		`name="csrf_token" value="` + csrf + `"`,
		`name="full_name" value="` + name + `"`,
		`name="email" value="` + email + `"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("teacher profile form missing %q", want)
		}
	}
	if strings.Contains(html, `action="/profile"`) {
		t.Error("teacher profile must never post to the student /profile endpoint")
	}

	// Security panel: password change is an outline link (it navigates), and
	// the only submit in that card is the destructive logout button.
	for _, want := range []string{
		`class="btn btn-outline theme-primary" href="/teacher/profile/password"`,
		`class="btn btn-outline theme-danger w-100"`,
		`action="/logout"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("teacher profile security card missing %q", want)
		}
	}

	// One primary action on the page surface: the save button.
	checkOnePrimaryButton(t, html)
	// The read-only role fields are still spelled out for the teacher.
	for _, want := range []string{"Peran dan akses", "Kuis, penilaian, dan data siswa"} {
		if !strings.Contains(html, want) {
			t.Errorf("teacher profile missing role copy %q", want)
		}
	}
}

func TestStudentProfilePageRenders(t *testing.T) {
	const (
		name  = "Siti Aminah"
		email = "siti@example.sch.id"
	)
	html := renderPage(t, "pages/profile.html", viewmodel.PageData{
		Base: viewmodel.Base{
			Title:  "Profil",
			Active: "profil",
			User: &viewmodel.CurrentUser{
				ID:         2,
				FullName:   name,
				Email:      email,
				Role:       viewmodel.RoleStudent,
				GradeLevel: 11,
				MajorName:  "Rekayasa Perangkat Lunak",
				Initials:   "SA",
			},
			CSRF: "csrf-token-456",
		},
		Data: viewmodel.ProfilePage{
			FullName:   name,
			Email:      email,
			GradeLevel: 11,
			MajorID:    3,
		},
	})

	checkBackAffordance(t, html, "/student", "Kembali ke dasbor")

	if !strings.Contains(html, `method="post" action="/profile"`) {
		t.Error("student profile must post to /profile")
	}
	if !strings.Contains(html, `name="full_name" value="`+name+`"`) {
		t.Error("student profile must echo the current name")
	}
	if !strings.Contains(html, `class="btn btn-outline theme-primary" href="/profile/password"`) {
		t.Error("student profile must link to /change-password as an outline link")
	}
	// Class and major are teacher-owned, so they render read-only.
	if !strings.Contains(html, "Kelas dan jurusan") {
		t.Error("student profile must keep the read-only class/major section")
	}
	checkOnePrimaryButton(t, html)
}

// TestProfilePagesWirePhotoEditorAndDirtyGate pins the client contract the
// redesign added: both account screens carry every hook photo-editor.js and
// app.js look up, plus the page-scoped module. A typo in a data attribute
// fails silently in the browser — the server render stays valid and the
// interaction simply never appears — so the template is the right place to
// catch it.
func TestProfilePagesWirePhotoEditorAndDirtyGate(t *testing.T) {
	pages := []struct {
		name string
		page string
		data viewmodel.PageData
	}{
		{
			name: "student",
			page: "pages/profile.html",
			data: viewmodel.PageData{
				Base: viewmodel.Base{
					Title: "Profil", Active: "profil", CSRF: "csrf-token",
					User: &viewmodel.CurrentUser{
						ID: 7, FullName: "Ayu Lestari", Email: "ayu@example.sch.id",
						Role: viewmodel.RoleStudent, GradeLevel: 11,
						MajorName: "Rekayasa Perangkat Lunak", Initials: "AL",
					},
				},
				Data: viewmodel.ProfilePage{
					FullName: "Ayu Lestari", Email: "ayu@example.sch.id",
					GradeLevel: 11, MajorID: 3,
				},
			},
		},
		{
			name: "teacher",
			page: "pages/teacher/profile.html",
			data: viewmodel.PageData{
				Base: viewmodel.Base{
					Title: "Profil", Active: "profil", CSRF: "csrf-token",
					User: &viewmodel.CurrentUser{
						ID: 1, FullName: "Budi Santoso", Email: "guru@sekolah.sch.id",
						Role: viewmodel.RoleTeacher, Initials: "BS",
					},
				},
				Data: viewmodel.TeacherProfilePage{
					FullName: "Budi Santoso", Email: "guru@sekolah.sch.id",
				},
			},
		},
	}

	for _, tc := range pages {
		t.Run(tc.name, func(t *testing.T) {
			html := renderPage(t, tc.page, tc.data)
			for _, want := range []string{
				`data-photo-form`,
				`data-photo-input`,
				`data-photo-drop`,
				`data-photo-editor`,
				`data-photo-stage`,
				`data-photo-zoom`,
				`data-photo-actions`,
				`data-photo-cancel`,
				`data-photo-upload`,
				`data-dirty-form`,
				`data-dirty-actions`,
				`data-dirty-submit`,
				`<script type="module" src="/static/js/photo-editor.js"></script>`,
			} {
				if !strings.Contains(html, want) {
					t.Errorf("%s missing %q", tc.page, want)
				}
			}
		})
	}
}

// TestChangePasswordHelpIsOnDemand pins the redesign of the signed-in auth
// screen: the standing explanation is gone, one labelled icon button beside
// the title opens it in a native dialog, and the dialog sits outside the form
// so it never joins the form's reading order.
func TestChangePasswordHelpIsOnDemand(t *testing.T) {
	const csrf = "csrf-token-789"
	html := renderPage(t, "pages/auth/change-password.html", viewmodel.PageData{
		Base: viewmodel.Base{
			Title: "Ubah kata sandi", Active: "profil", CSRF: csrf,
			User: &viewmodel.CurrentUser{
				ID: 7, FullName: "Ayu Lestari", Email: "ayu@example.sch.id",
				Role: viewmodel.RoleStudent, Initials: "AL",
			},
		},
		Data: viewmodel.ChangePasswordPage{BackHref: "/profile"},
	})

	if strings.Contains(html, `<div class="auth-note">`) {
		t.Error("the standing password note must not sit above the form")
	}

	trigger := strings.Index(html, `data-bs-target="#password-help"`)
	form := strings.Index(html, `<form class="auth-form"`)
	dialog := strings.Index(html, `<dialog class="dialog" id="password-help"`)
	if trigger < 0 || form < 0 || dialog < 0 {
		t.Fatalf("password help wiring missing (trigger=%d form=%d dialog=%d)", trigger, form, dialog)
	}
	if trigger > form {
		t.Error("the info trigger must sit beside the title, before the form")
	}
	if dialog < form {
		t.Error("the help dialog must follow the form, not interrupt it")
	}

	for _, want := range []string{
		`class="btn btn-text auth-info-btn"`,
		`type="button"`,
		`data-bs-toggle="dialog"`,
		`aria-label="Penjelasan halaman ini"`,
		`aria-haspopup="dialog"`,
		`aria-controls="password-help"`,
		`aria-labelledby="password-help-title"`,
		`aria-describedby="password-help-body"`,
		`id="password-help-title">Bukan halaman lupa kata sandi</h2>`,
		`>Mengerti<`,
		`action="/change-password" method="post"`,
		`name="csrf_token" value="` + csrf + `"`,
		`name="current_password"`,
		`name="new_password"`,
		`name="password_confirm"`,
		`href="/profile"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("change-password page missing %q", want)
		}
	}
}
