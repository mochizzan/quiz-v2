package httpserver_test

// avatar_render_test.go is the render half of AVT-081/AVT-083: every
// teacher-facing surface that shows a student renders the shared avatar
// partial with that row's model. A row with a photo carries one <img
// src="/avatars/…"> and no initials; a row without one carries the initials
// and no image — never both, and never a client-side fallback.
//
// The pages render directly from ./templates, like the other page contract
// tests. Base.User is deliberately nil on the row surfaces so the only avatar
// badges in the document are the ones the page rows emit; the profile tests
// set it to prove the chrome and the header card agree.

import (
	"strings"
	"testing"

	"quiz-v2/internal/viewmodel"
)

// renderPhotoName is a name the avatar grammar accepts (32 lowercase hex plus
// .webp). The render layer does not validate it; the value is realistic so a
// failing assertion prints the same URL the app produces.
const (
	renderPhotoName = "0123456789abcdef0123456789abcdef.webp"
	renderPhotoURL  = "/avatars/" + renderPhotoName
)

// photoAvatar is one person with a stored photo. Initials stay populated: the
// partial must choose the photo, not fall back to them.
func photoAvatar(class string) viewmodel.Avatar {
	return viewmodel.Avatar{URL: renderPhotoURL, Initials: "AL", Class: class}
}

// initialsAvatar is the same person before an upload: no URL, initials only.
func initialsAvatar(class string) viewmodel.Avatar {
	return viewmodel.Avatar{Initials: "AL", Class: class}
}

// avatarBadges returns the markup of every shared avatar badge in document
// order. The partial's body is text or one <img>, so the first closing </span>
// after the opening tag is the badge's own close.
func avatarBadges(html string) []string {
	const marker = `<span class="app-avatar`
	var badges []string
	for {
		start := strings.Index(html, marker)
		if start < 0 {
			return badges
		}
		rest := html[start:]
		end := strings.Index(rest, "</span>")
		if end < 0 {
			return badges
		}
		badges = append(badges, rest[:end])
		html = rest[end+len("</span>"):]
	}
}

// checkPhotoBadge asserts one badge renders the photo, the surface's class,
// and no initials.
func checkPhotoBadge(t *testing.T, badge, class string) {
	t.Helper()
	if !strings.Contains(badge, "app-avatar--photo") {
		t.Errorf("photo badge missing app-avatar--photo: %s", badge)
	}
	if !strings.Contains(badge, `src="`+renderPhotoURL+`"`) {
		t.Errorf("photo badge missing src=%q: %s", renderPhotoURL, badge)
	}
	if strings.Contains(badge, ">AL") {
		t.Errorf("photo badge also rendered initials: %s", badge)
	}
	if class == "" {
		if strings.Contains(badge, "app-avatar--sm") || strings.Contains(badge, "monitor-avatar") {
			t.Errorf("badge carries an unexpected size class: %s", badge)
		}
	} else if !strings.Contains(badge, class) {
		t.Errorf("badge missing class %q: %s", class, badge)
	}
}

// checkInitialsBadge asserts one badge renders the initials and no image.
func checkInitialsBadge(t *testing.T, badge, class string) {
	t.Helper()
	if strings.Contains(badge, "<img") || strings.Contains(badge, "app-avatar--photo") {
		t.Errorf("initials badge rendered a photo: %s", badge)
	}
	if !strings.HasSuffix(badge, ">AL") {
		t.Errorf("initials badge missing the model's initials: %s", badge)
	}
	if class == "" {
		if strings.Contains(badge, "app-avatar--sm") || strings.Contains(badge, "monitor-avatar") {
			t.Errorf("badge carries an unexpected size class: %s", badge)
		}
	} else if !strings.Contains(badge, class) {
		t.Errorf("badge missing class %q: %s", class, badge)
	}
}

// TestTeacherStudentRowsRenderAvatar walks every teacher-facing surface that
// carries a student row or header. Each case renders the page twice — once
// with a photo, once with initials — so a template that drops the Avatar field
// or hard-codes one variant fails.
func TestTeacherStudentRowsRenderAvatar(t *testing.T) {
	type surface struct {
		name  string
		page  string
		class string // the class the query layer pins for this surface
		data  func(viewmodel.Avatar) viewmodel.PageData
	}
	base := func(title string) viewmodel.Base { return viewmodel.Base{Title: title} }

	surfaces := []surface{
		{
			name: "roster", page: "pages/teacher/students.html", class: "app-avatar--sm",
			data: func(a viewmodel.Avatar) viewmodel.PageData {
				return viewmodel.PageData{
					Base: base("Siswa"),
					Data: viewmodel.StudentList{
						Students: viewmodel.NewList([]viewmodel.StudentRow{{
							ID: 7, FullName: "Ayu Lestari", Email: "ayu@example.sch.id",
							ClassLabel: "XI RPL 1", Attempts: 3, Avatar: a,
						}}, 1, 10),
					},
				}
			},
		},
		{
			name: "student detail", page: "pages/teacher/student-edit.html",
			data: func(a viewmodel.Avatar) viewmodel.PageData {
				return viewmodel.PageData{
					Base: base("Ubah siswa"),
					Data: viewmodel.StudentForm{
						ID: 7, FullName: "Ayu Lestari", Email: "ayu@example.sch.id",
						GradeLevel: 11, Action: "/teacher/students/7",
						CancelHref: "/teacher/students", Avatar: a,
					},
				}
			},
		},
		{
			name: "monitor", page: "pages/teacher/monitor.html", class: "monitor-avatar",
			data: func(a viewmodel.Avatar) viewmodel.PageData {
				return viewmodel.PageData{
					Base: base("Monitor kuis"),
					Data: viewmodel.MonitorPage{
						Quiz:     viewmodel.QuizRef{ID: 9, Title: "Ulangan Bab 3"},
						Progress: viewmodel.MonitorProgress{Registered: 1, InProgress: 1, Total: 5, Answered: 2},
						Rows: []viewmodel.ParticipantRow{{
							RegistrationID: 501, StudentName: "Ayu Lestari", Initials: "AL",
							ClassLabel: "XI RPL 1", State: viewmodel.ParticipationInProgress,
							Answered: 2, Total: 5, Avatar: a,
						}},
						QuizHref:    "/teacher/quizzes/9",
						GeneratedAt: viewmodel.LocalDate(2026, 10, 25, 9, 0),
					},
				}
			},
		},
		{
			name: "results", page: "pages/teacher/results.html", class: "app-avatar--sm",
			data: func(a viewmodel.Avatar) viewmodel.PageData {
				return viewmodel.PageData{
					Base: base("Hasil kuis"),
					Data: viewmodel.QuizResults{
						Quiz:   viewmodel.QuizRef{ID: 9, Title: "Ulangan Bab 3"},
						Counts: viewmodel.ResultsCounts{Submitted: 1},
						Rows: viewmodel.NewList([]viewmodel.ResultsRow{{
							AttemptID: 44, StudentName: "Ayu Lestari", ClassLabel: "XI RPL 1",
							AttemptNo: 1, MaxAttempts: 3,
							SubmittedAt: viewmodel.LocalDatePtr(2026, 10, 24, 8, 0),
							RecapHref:   "/teacher/recap/44", Avatar: a,
						}}, 1, 10),
						BackHref:    "/teacher/quizzes/9",
						GradingHref: "/teacher/grading?quiz=9",
						ReviewMode:  viewmodel.ReviewQuestionsWithKey,
					},
				}
			},
		},
		{
			name: "grading queue", page: "pages/teacher/grading.html", class: "app-avatar--sm",
			data: func(a viewmodel.Avatar) viewmodel.PageData {
				return viewmodel.PageData{
					Base: base("Penilaian esai"),
					Data: viewmodel.GradingPage{
						Queue: viewmodel.GradingQueue{
							Counts: viewmodel.GradingCounts{Pending: 1},
							Rows: viewmodel.NewList([]viewmodel.GradingRow{{
								AnswerID: 88, StudentName: "Ayu Lestari", ClassLabel: "XI RPL 1",
								QuizTitle: "Ulangan Bab 3", QuestionExcerpt: "Jelaskan proses fotosintesis.",
								WaitingLabel: "Menunggu 2 hari", Href: "/teacher/grading/88", Avatar: a,
							}}, 1, 10),
						},
					},
				}
			},
		},
		{
			name: "grading detail", page: "pages/teacher/grading.html",
			data: func(a viewmodel.Avatar) viewmodel.PageData {
				return viewmodel.PageData{
					Base: base("Penilaian esai"),
					Data: viewmodel.GradingPage{
						Selected: &viewmodel.GradingDetail{
							Answer: viewmodel.EssayAnswer{
								ID: 88, StudentName: "Ayu Lestari", ClassLabel: "XI RPL 1",
								QuizTitle: "Ulangan Bab 3", QuestionBody: "Jelaskan proses fotosintesis.",
								Text: "Tumbuhan menyerap cahaya.", Avatar: a,
							},
							Attempt: viewmodel.AttemptHeader{AttemptNo: 1, MaxAttempts: 3},
						},
					},
				}
			},
		},
		{
			name: "directory", page: "pages/teacher/teachers.html", class: "app-avatar--sm",
			data: func(a viewmodel.Avatar) viewmodel.PageData {
				return viewmodel.PageData{
					Base: base("Guru"),
					Data: viewmodel.TeachersPage{
						Active: []viewmodel.TeacherRow{{
							ID: 3, FullName: "Ayu Lestari", Email: "ayu@school.id",
							Status: viewmodel.AccountActive, Avatar: a,
						}},
					},
				}
			},
		},
		{
			name: "recap", page: "pages/teacher/recap.html",
			data: func(a viewmodel.Avatar) viewmodel.PageData {
				return viewmodel.PageData{
					Base: base("Rekap percobaan"),
					Data: viewmodel.Recap{
						Attempt: viewmodel.RecapAttempt{
							ID: 44, StudentName: "Ayu Lestari", ClassLabel: "XI RPL 1",
							QuizTitle: "Ulangan Bab 3", AttemptNo: 1, MaxAttempts: 3,
							Avatar: a,
						},
						BackHref: "/teacher/quizzes/9/results",
					},
				}
			},
		},
		{
			name: "top scores", page: "pages/teacher/dashboard.html", class: "app-avatar--sm",
			data: func(a viewmodel.Avatar) viewmodel.PageData {
				return viewmodel.PageData{
					Base: base("Dasbor guru"),
					Data: viewmodel.TeacherDashboard{
						GreetingName: "Bu Ayu",
						TopScores: []viewmodel.QuizTopScore{{
							Quiz:        viewmodel.QuizRef{ID: 9, Title: "Ulangan Bab 3"},
							StudentName: "Ayu Lestari", ClassLabel: "XI RPL 1",
							Score: viewmodel.NewScore(84), Href: "/teacher/quizzes/9/results",
							Avatar: a,
						}},
					},
				}
			},
		},
	}

	for _, s := range surfaces {
		t.Run(s.name, func(t *testing.T) {
			t.Run("photo", func(t *testing.T) {
				html := renderPage(t, s.page, s.data(photoAvatar(s.class)))
				badges := avatarBadges(html)
				if len(badges) != 1 {
					t.Fatalf("avatar badges = %d, want exactly the one row badge", len(badges))
				}
				checkPhotoBadge(t, badges[0], s.class)
			})
			t.Run("initials", func(t *testing.T) {
				html := renderPage(t, s.page, s.data(initialsAvatar(s.class)))
				badges := avatarBadges(html)
				if len(badges) != 1 {
					t.Fatalf("avatar badges = %d, want exactly the one row badge", len(badges))
				}
				checkInitialsBadge(t, badges[0], s.class)
			})
		})
	}
}

// TestProfilePagesRenderPhotoOrInitials pins AVT-083 on both account screens:
// the header card and every chrome copy show the same variant, the photo form
// is always present, and the remove form appears only while a photo exists.
func TestProfilePagesRenderPhotoOrInitials(t *testing.T) {
	surfaces := []struct {
		name string
		page string
		role viewmodel.Role
		data func(viewmodel.CurrentUser) viewmodel.PageData
	}{
		{
			name: "student", page: "pages/profile.html", role: viewmodel.RoleStudent,
			data: func(u viewmodel.CurrentUser) viewmodel.PageData {
				return viewmodel.PageData{
					Base: viewmodel.Base{Title: "Profil", Active: "profil", User: &u, CSRF: "csrf-token"},
					Data: viewmodel.ProfilePage{
						FullName: u.FullName, Email: u.Email, GradeLevel: 11, MajorID: 3,
					},
				}
			},
		},
		{
			name: "teacher", page: "pages/teacher/profile.html", role: viewmodel.RoleTeacher,
			data: func(u viewmodel.CurrentUser) viewmodel.PageData {
				return viewmodel.PageData{
					Base: viewmodel.Base{Title: "Profil", Active: "profil", User: &u, CSRF: "csrf-token"},
					Data: viewmodel.TeacherProfilePage{FullName: u.FullName, Email: u.Email},
				}
			},
		},
	}

	for _, s := range surfaces {
		t.Run(s.name, func(t *testing.T) {
			user := func(photo bool) viewmodel.CurrentUser {
				u := viewmodel.CurrentUser{
					ID: 7, FullName: "Ayu Lestari", Email: "ayu@example.sch.id",
					Initials: "AL", Role: s.role,
				}
				if photo {
					u.AvatarURL = renderPhotoURL
				}
				return u
			}

			t.Run("photo", func(t *testing.T) {
				html := renderPage(t, s.page, s.data(user(true)))

				// The header card is one badge; the chrome (navbar + drawer)
				// repeats the same variant because it shares CurrentUser.
				body := pageBody(t, html)
				bodyBadges := avatarBadges(body)
				if len(bodyBadges) != 1 {
					t.Fatalf("page-body avatar badges = %d, want 1", len(bodyBadges))
				}
				checkPhotoBadge(t, bodyBadges[0], "")
				for _, badge := range avatarBadges(html) {
					checkPhotoBadge(t, badge, "")
				}

				form := between(t, body, `action="/profile/photo" enctype="multipart/form-data">`, "</form>")
				if !strings.Contains(form, `name="photo"`) {
					t.Error("photo form missing the file field")
				}
				if !strings.Contains(form, "Maksimal 200 MB.") {
					t.Error("photo form missing the 200 MB help copy")
				}
				if strings.Contains(form, "data-validate") {
					t.Error("photo form must not opt into client-side validation")
				}
				if !strings.HasPrefix(strings.TrimSpace(form), `<input type="hidden" name="csrf_token"`) {
					t.Error("csrf_token must be the first field in the photo form (AVT-090)")
				}
				if !strings.Contains(body, `action="/profile/photo/remove"`) ||
					!strings.Contains(body, "Hapus foto") {
					t.Error("a stored photo must render the remove form")
				}
			})

			t.Run("initials", func(t *testing.T) {
				html := renderPage(t, s.page, s.data(user(false)))

				body := pageBody(t, html)
				bodyBadges := avatarBadges(body)
				if len(bodyBadges) != 1 {
					t.Fatalf("page-body avatar badges = %d, want 1", len(bodyBadges))
				}
				checkInitialsBadge(t, bodyBadges[0], "")
				for _, badge := range avatarBadges(html) {
					checkInitialsBadge(t, badge, "")
				}

				if strings.Contains(body, `action="/profile/photo/remove"`) ||
					strings.Contains(body, "Hapus foto") {
					t.Error("the remove form must not render without a photo (AVT-092)")
				}
			})
		})
	}
}

// TestStudentDashboardQuizCardRendersTeacherAvatar pins AVT-083 on the student
// dashboard: each of the three quiz lists renders the owning teacher's badge
// through the shared avatar partial — a photo when the account has one, the
// initials when it does not. The card is the only badge on the screen, so a
// template that drops .TeacherAvatar fails both variants on every list.
func TestStudentDashboardQuizCardRendersTeacherAvatar(t *testing.T) {
	surfaces := []struct {
		name string
		list func(viewmodel.QuizCard) viewmodel.StudentDashboard
	}{
		{"live", func(c viewmodel.QuizCard) viewmodel.StudentDashboard {
			return viewmodel.StudentDashboard{GreetingName: "Ayu", DateLine: "Senin", Live: []viewmodel.QuizCard{c}}
		}},
		{"ready", func(c viewmodel.QuizCard) viewmodel.StudentDashboard {
			return viewmodel.StudentDashboard{GreetingName: "Ayu", DateLine: "Senin", Ready: []viewmodel.QuizCard{c}}
		}},
		{"latest", func(c viewmodel.QuizCard) viewmodel.StudentDashboard {
			return viewmodel.StudentDashboard{GreetingName: "Ayu", DateLine: "Senin", Latest: []viewmodel.QuizCard{c}}
		}},
	}

	card := func(a viewmodel.Avatar) viewmodel.QuizCard {
		return viewmodel.QuizCard{
			ID: 7, Href: "/quiz/7", Title: "Ulangan Bab 3", TeacherName: "Ayu Lestari",
			TeacherAvatar: a, QuestionCount: 10, State: viewmodel.QuizOpen,
			Primary: viewmodel.Primary("Mulai kuis", "/quiz/7"),
		}
	}

	for _, s := range surfaces {
		t.Run(s.name, func(t *testing.T) {
			render := func(a viewmodel.Avatar) string {
				return renderPage(t, "pages/student/dashboard.html", viewmodel.PageData{
					Base: viewmodel.Base{Title: "Dasbor siswa"},
					Data: s.list(card(a)),
				})
			}

			t.Run("photo", func(t *testing.T) {
				badges := avatarBadges(render(photoAvatar("app-avatar--sm")))
				if len(badges) != 1 {
					t.Fatalf("avatar badges = %d, want exactly the one card badge", len(badges))
				}
				checkPhotoBadge(t, badges[0], "app-avatar--sm")
			})
			t.Run("initials", func(t *testing.T) {
				badges := avatarBadges(render(initialsAvatar("app-avatar--sm")))
				if len(badges) != 1 {
					t.Fatalf("avatar badges = %d, want exactly the one card badge", len(badges))
				}
				checkInitialsBadge(t, badges[0], "app-avatar--sm")
			})
		})
	}
}
