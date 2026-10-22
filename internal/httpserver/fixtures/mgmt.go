package fixtures

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"quiz-v2/internal/viewmodel"
)

// Management fixtures. Every screen below is a projection of the one demo
// world in world.go: student attempts, anonymous registrations and quiz
// targets are counted from the same data the quizzes and results pages use.
// Nothing here mutates the world.

// DemoResetToken is the one-time password-reset token the students list shows
// after POST /teacher/students/:id/reset-token. It is demo data only.
const DemoResetToken = "TW-8F2K-9Q1M"

// ManagementPageSize is the page size for both management lists.
const ManagementPageSize = 8

// StudentFilter is the neutral filter the students route parses from the
// query string. The fixture package cannot import httpserver, so the route
// hands over plain values instead of a PageRequest.
type StudentFilter struct {
	Query   string
	Grade   int
	MajorID int64
	Sort    viewmodel.SortOrder
	Page    int
}

// StudentListPage builds /teacher/students: grade tabs, search and major
// filters, the page of rows, and the matching pagination links. The reset
// token panel is appended by the route only when the token query is present.
func StudentListPage(f StudentFilter) viewmodel.StudentList {
	w := Default()
	sortOrder := normalizeStudentSort(f.Sort)
	query := strings.TrimSpace(f.Query)

	attempts := make(map[int64]int, len(w.Students))
	for _, r := range w.Registrations {
		if r.StudentID != 0 {
			attempts[r.StudentID] += len(r.Attempts)
		}
	}

	rows := make([]viewmodel.StudentRow, 0, len(w.Students))
	for _, s := range w.Students {
		if f.Grade != 0 && s.GradeLevel != f.Grade {
			continue
		}
		if f.MajorID != 0 && s.MajorID != f.MajorID {
			continue
		}
		if query != "" &&
			!strings.Contains(strings.ToLower(s.FullName), strings.ToLower(query)) &&
			!strings.Contains(strings.ToLower(s.Email), strings.ToLower(query)) {
			continue
		}
		rows = append(rows, viewmodel.StudentRow{
			ID:         s.ID,
			FullName:   s.FullName,
			Email:      s.Email,
			ClassLabel: w.ClassOf(s.GradeLevel, s.MajorID),
			GradeLevel: s.GradeLevel,
			MajorID:    s.MajorID,
			Attempts:   attempts[s.ID],
			EditHref:   "/teacher/students/" + strconv.FormatInt(s.ID, 10),
			ResetHref:  "/teacher/students/" + strconv.FormatInt(s.ID, 10) + "/reset-token",
			DeleteHref: "/teacher/students/" + strconv.FormatInt(s.ID, 10) + "/delete",
		})
	}
	sortStudents(rows, sortOrder)

	baseQuery := studentQuery(f, false)
	resetAction := viewmodel.Outline("Atur ulang filter", "/teacher/students")
	empty := viewmodel.EmptyState{
		Title:  "Tidak ada siswa yang cocok.",
		Body:   "Ubah kata kunci atau filter kelas dan jurusan.",
		Icon:   "users",
		Action: &resetAction,
	}

	return viewmodel.StudentList{
		Students:   viewmodel.NewList(rows, f.Page, ManagementPageSize),
		Pagination: viewmodel.NewPagination("/teacher/students", baseQuery, f.Page, ManagementPageSize, len(rows)),
		Tabs:       studentTabs(f),
		Majors:     Majors(),
		Query: viewmodel.QueryState{
			Q:    query,
			Tab:  majorTab(f.MajorID),
			Sort: sortOrder,
		},
		Empty: empty,
	}
}

// ResetToken returns the one-time token panel shown after a reset-token POST.
// The token expires 24 hours after the final activity in the demo world.
func ResetToken() *viewmodel.ResetToken {
	return &viewmodel.ResetToken{
		Token:     DemoResetToken,
		ExpiresAt: viewmodel.LocalDate(2026, time.October, 24, 9, 15),
	}
}

// StudentFormPage builds /teacher/students/:id. An unknown id falls back to
// the first demo student so the screen always renders a coherent account.
func StudentFormPage(id int64) viewmodel.StudentForm {
	w := Default()
	s := w.Student(id)
	if s == nil {
		s = w.Student(StudentAyu)
	}
	return viewmodel.StudentForm{
		ID:         s.ID,
		FullName:   s.FullName,
		Email:      s.Email,
		GradeLevel: s.GradeLevel,
		MajorID:    s.MajorID,
		Majors:     Majors(),
		Action:     "/teacher/students/" + strconv.FormatInt(s.ID, 10),
		CancelHref: "/teacher/students",
	}
}

// AnonymousFilter is the search state for the anonymous list.
type AnonymousFilter struct {
	Query string
	Page  int
}

// AnonymousListPage builds /teacher/anonymous. Registrations are grouped by
// participant identity (name, grade and major) so one person appears once even
// after joining several quizzes.
func AnonymousListPage(f AnonymousFilter) viewmodel.AnonymousList {
	w := Default()
	query := strings.TrimSpace(f.Query)

	type group struct {
		id       int64
		fullName string
		grade    int
		majorID  int64
		quizzes  map[int64]bool
		last     *viewmodel.LocalTime
	}

	var groups []*group
	byIdentity := map[string]*group{}
	for _, r := range w.Registrations {
		if !r.Anonymous {
			continue
		}
		key := r.FullName + "|" + strconv.Itoa(r.GradeLevel) + "|" + strconv.FormatInt(r.MajorID, 10)
		g := byIdentity[key]
		if g == nil {
			g = &group{
				id:       r.ID,
				fullName: r.FullName,
				grade:    r.GradeLevel,
				majorID:  r.MajorID,
				quizzes:  map[int64]bool{},
			}
			byIdentity[key] = g
			groups = append(groups, g)
		}
		g.quizzes[r.QuizID] = true
		if last := latestActivity(r); last != nil && (g.last == nil || last.Time().After(g.last.Time())) {
			g.last = last
		}
	}

	rows := make([]viewmodel.AnonymousRow, 0, len(groups))
	for _, g := range groups {
		if query != "" && !strings.Contains(strings.ToLower(g.fullName), strings.ToLower(query)) {
			continue
		}
		rows = append(rows, viewmodel.AnonymousRow{
			ID:            g.id,
			FullName:      g.fullName,
			ClassLabel:    w.ClassOf(g.grade, g.majorID),
			GradeLevel:    g.grade,
			MajorName:     w.MajorName(g.majorID),
			QuizzesJoined: len(g.quizzes),
			LastActivity:  g.last,
			EditHref:      "/teacher/anonymous/" + strconv.FormatInt(g.id, 10),
			DeleteHref:    "/teacher/anonymous/" + strconv.FormatInt(g.id, 10) + "/delete",
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].LastActivity, rows[j].LastActivity
		switch {
		case a == nil && b == nil:
			return rows[i].FullName < rows[j].FullName
		case a == nil:
			return false
		case b == nil:
			return true
		default:
			return a.Time().After(b.Time())
		}
	})

	baseQuery := url.Values{}
	if query != "" {
		baseQuery.Set("q", query)
	}
	resetAction := viewmodel.Outline("Atur ulang pencarian", "/teacher/anonymous")
	empty := viewmodel.EmptyState{
		Title:  "Belum ada peserta anonim.",
		Body:   "Peserta yang bergabung tanpa akun akan muncul di sini.",
		Icon:   "users",
		Action: &resetAction,
	}

	return viewmodel.AnonymousList{
		Rows:       viewmodel.NewList(rows, f.Page, ManagementPageSize),
		Pagination: viewmodel.NewPagination("/teacher/anonymous", baseQuery, f.Page, ManagementPageSize, len(rows)),
		Query:      viewmodel.QueryState{Q: query},
		Empty:      empty,
	}
}

// AnonymousFormPage builds /teacher/anonymous/:id. The id is a registration
// id; an unknown or non-anonymous id falls back to the first anonymous
// registration in the world.
func AnonymousFormPage(id int64) viewmodel.AnonymousForm {
	w := Default()
	r := w.Registration(id)
	if r == nil || !r.Anonymous {
		r = nil
		for _, candidate := range w.Registrations {
			if candidate.Anonymous {
				r = candidate
				break
			}
		}
	}
	form := viewmodel.AnonymousForm{
		Majors:     Majors(),
		CancelHref: "/teacher/anonymous",
	}
	if r != nil {
		form.ID = r.ID
		form.FullName = r.FullName
		form.GradeLevel = r.GradeLevel
		form.MajorID = r.MajorID
		form.Action = "/teacher/anonymous/" + strconv.FormatInt(r.ID, 10)
	}
	return form
}

// MajorsPage builds /teacher/majors. A major can be deleted only when no
// student, anonymous participant or quiz target references it.
func MajorsPage() viewmodel.MajorsPage {
	w := Default()

	anonymous := make(map[int64]int, len(w.Majors))
	for _, r := range w.Registrations {
		if r.Anonymous {
			anonymous[r.MajorID]++
		}
	}

	rows := make([]viewmodel.MajorRow, 0, len(w.Majors))
	for _, m := range w.Majors {
		students := 0
		for _, s := range w.Students {
			if s.MajorID == m.ID {
				students++
			}
		}
		quizzes := 0
		for _, q := range w.Quizzes {
			if q.TargetMajorID == m.ID {
				quizzes++
			}
		}
		rows = append(rows, viewmodel.MajorRow{
			ID:           m.ID,
			Name:         m.Name,
			StudentCount: students,
			QuizCount:    quizzes,
			CanDelete:    students == 0 && quizzes == 0 && anonymous[m.ID] == 0,
			RenameAction: "/teacher/majors/" + strconv.FormatInt(m.ID, 10),
			DeleteAction: "/teacher/majors/" + strconv.FormatInt(m.ID, 10) + "/delete",
		})
	}

	return viewmodel.MajorsPage{
		Majors:       rows,
		CreateAction: "/teacher/majors",
		Empty: viewmodel.EmptyState{
			Title: "Belum ada jurusan.",
			Body:  "Tambahkan jurusan pertama agar kelas dan sasaran kuis dapat diatur.",
			Icon:  "bank",
		},
	}
}

// TeachersPage builds /teacher/teachers: pending registrations first, then
// the active teachers. Only pending rows carry an approval action.
func TeachersPage() viewmodel.TeachersPage {
	w := Default()
	pending := make([]viewmodel.TeacherRow, 0, len(w.Teachers))
	active := make([]viewmodel.TeacherRow, 0, len(w.Teachers))
	for _, t := range w.Teachers {
		row := viewmodel.TeacherRow{
			ID:          t.ID,
			FullName:    t.FullName,
			Email:       t.Email,
			Initials:    viewmodel.Initials(t.FullName),
			Status:      t.Status,
			ApprovedBy:  t.ApprovedBy,
			ApprovedAt:  t.ApprovedAt,
			ApproveHref: "/teacher/teachers/" + strconv.FormatInt(t.ID, 10) + "/approve",
		}
		switch t.Status {
		case viewmodel.AccountPending:
			row.CanApprove = true
			pending = append(pending, row)
		case viewmodel.AccountActive:
			active = append(active, row)
		}
	}

	return viewmodel.TeachersPage{
		Pending: pending,
		Active:  active,
		Empty: viewmodel.EmptyState{
			Title: "Tidak ada pendaftaran guru yang menunggu.",
			Body:  "Pendaftaran guru baru akan muncul di sini untuk disetujui.",
			Icon:  "check",
		},
	}
}

// normalizeStudentSort keeps the students list to the two orders that make
// sense for accounts without a creation time.
func normalizeStudentSort(s viewmodel.SortOrder) viewmodel.SortOrder {
	if s == viewmodel.SortNameDesc {
		return s
	}
	return viewmodel.SortNameAsc
}

func sortStudents(rows []viewmodel.StudentRow, order viewmodel.SortOrder) {
	sort.SliceStable(rows, func(i, j int) bool {
		if order == viewmodel.SortNameDesc {
			return strings.ToLower(rows[i].FullName) > strings.ToLower(rows[j].FullName)
		}
		return strings.ToLower(rows[i].FullName) < strings.ToLower(rows[j].FullName)
	})
}

// studentTabs builds the grade filter tabs, preserving the other active
// filters in each link.
func studentTabs(f StudentFilter) []viewmodel.FilterTab {
	w := Default()
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
	for _, d := range defs {
		count := 0
		for _, s := range w.Students {
			if d.grade == 0 || s.GradeLevel == d.grade {
				count++
			}
		}
		q := studentQuery(f, false)
		q.Del("grade")
		if d.grade != 0 {
			q.Set("grade", strconv.Itoa(d.grade))
		}
		tabs = append(tabs, viewmodel.FilterTab{
			Label:  d.label,
			Href:   hrefWithQuery("/teacher/students", q),
			Active: f.Grade == d.grade,
			Count:  count,
		})
	}
	return tabs
}

func studentQuery(f StudentFilter, withPage bool) url.Values {
	q := url.Values{}
	if strings.TrimSpace(f.Query) != "" {
		q.Set("q", strings.TrimSpace(f.Query))
	}
	if f.Grade != 0 {
		q.Set("grade", strconv.Itoa(f.Grade))
	}
	if f.MajorID != 0 {
		q.Set("major_id", strconv.FormatInt(f.MajorID, 10))
	}
	if f.Sort != "" && f.Sort != viewmodel.SortNameAsc {
		q.Set("sort", string(f.Sort))
	}
	if withPage && f.Page > 1 {
		q.Set("page", strconv.Itoa(f.Page))
	}
	return q
}

func majorTab(id int64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}

// latestActivity is the newest timestamp on one registration: joining,
// starting an attempt, or submitting one.
func latestActivity(r *Registration) *viewmodel.LocalTime {
	var last *viewmodel.LocalTime
	consider := func(t viewmodel.LocalTime) {
		if t.IsZero() {
			return
		}
		if last == nil || t.Time().After(last.Time()) {
			v := t
			last = &v
		}
	}
	consider(r.JoinedAt)
	for _, a := range r.Attempts {
		consider(a.StartedAt)
		if a.SubmittedAt != nil {
			consider(*a.SubmittedAt)
		}
	}
	return last
}

func hrefWithQuery(base string, q url.Values) string {
	if enc := q.Encode(); enc != "" {
		return base + "?" + enc
	}
	return base
}
