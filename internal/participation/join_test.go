package participation

// join_test.go covers the entry flow (PAR-021..060) and the participation
// commands that follow it (PAR-061..086). Every test here needs the real
// schema; storetest.DB skips when TEST_DB_DSN is unset. The seeding helpers in
// studentread_test.go are reused so the fixtures stay one reset database with
// the same constraints the production schema enforces.

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/internal/store/storetest"
	"quiz-v2/internal/viewmodel"
)

const joinTestKey = "join-test-signing-key"

type joinFixture struct {
	t   *testing.T
	db  *store.DB
	svc *Service
	ctx context.Context

	teacher        int64
	studentAccount int64
	studentID      int64
	majorID        int64
	otherMajorID   int64

	publicQuiz  int64
	draftQuiz   int64
	closedQuiz  int64
	privateQuiz int64
	emptyQuiz   int64
	oneShotQuiz int64
	cancelQuiz  int64
	switchQuiz  int64
}

func newJoinFixture(t *testing.T) *joinFixture {
	t.Helper()
	db := storetest.DB(t)
	storetest.Reset(t, db)
	f := &joinFixture{
		t:   t,
		db:  db,
		svc: New(db, []byte(joinTestKey)),
		ctx: context.Background(),
	}
	f.svc.nowFn = func() time.Time { return readTestNow }

	f.teacher = seedReadAccount(t, db, "guru@join.test", "Bu Guru", "teacher")
	f.majorID = seedReadMajor(t, db, "Rekayasa Perangkat Lunak", "RPL")
	f.otherMajorID = seedReadMajor(t, db, "Desain Komunikasi Visual", "DKV")
	f.studentAccount = seedReadAccount(t, db, "siswa@join.test", "Siswa Uji", "student")
	f.studentID = seedReadStudent(t, db, f.studentAccount, f.majorID, 11)

	f.publicQuiz = seedReadQuiz(t, db, readQuizSeed{
		owner: f.teacher, title: "Kuis Publik", visibility: "public", state: "open",
		code: "123456", invite: "invite-public-01", published: readTestAgo(24 * time.Hour),
		allowRetake: false, maxAttempts: 0, showScore: true,
	})
	seedReadQuestions(t, db, f.publicQuiz, 2)

	f.draftQuiz = seedReadQuiz(t, db, readQuizSeed{
		owner: f.teacher, title: "Kuis Draf", visibility: "public", state: "draft",
		code: "654321", invite: "invite-draft-01",
	})
	seedReadQuestions(t, db, f.draftQuiz, 1)

	f.closedQuiz = seedReadQuiz(t, db, readQuizSeed{
		owner: f.teacher, title: "Kuis Tutup", visibility: "public", state: "closed",
		code: "111111", invite: "invite-closed-1", published: readTestAgo(48 * time.Hour),
		closed: readTestAgo(2 * time.Hour),
	})
	seedReadQuestions(t, db, f.closedQuiz, 1)

	f.privateQuiz = seedReadQuiz(t, db, readQuizSeed{
		owner: f.teacher, title: "Kuis Privat", visibility: "private", state: "open",
		invite: "invite-private-1", targetGrade: 11, targetMajor: f.majorID,
		published: readTestAgo(24 * time.Hour),
	})
	seedReadQuestions(t, db, f.privateQuiz, 1)

	f.emptyQuiz = seedReadQuiz(t, db, readQuizSeed{
		owner: f.teacher, title: "Kuis Kosong", visibility: "public", state: "open",
		code: "222222", invite: "invite-empty-01", published: readTestAgo(24 * time.Hour),
	})

	f.oneShotQuiz = seedReadQuiz(t, db, readQuizSeed{
		owner: f.teacher, title: "Kuis Sekali", visibility: "public", state: "open",
		code: "777777", invite: "invite-oneshot-1", published: readTestAgo(24 * time.Hour),
		allowRetake: false, maxAttempts: 0, showScore: true,
	})
	seedReadQuestions(t, db, f.oneShotQuiz, 1)

	f.cancelQuiz = seedReadQuiz(t, db, readQuizSeed{
		owner: f.teacher, title: "Kuis Batal", visibility: "public", state: "open",
		code: "888888", invite: "invite-cancel-1", published: readTestAgo(24 * time.Hour),
		allowRetake: true, maxAttempts: 1,
	})
	seedReadQuestions(t, db, f.cancelQuiz, 1)

	f.switchQuiz = seedReadQuiz(t, db, readQuizSeed{
		owner: f.teacher, title: "Kuis Ganti", visibility: "public", state: "open",
		code: "333333", invite: "invite-switch-1", published: readTestAgo(24 * time.Hour),
	})
	seedReadQuestions(t, db, f.switchQuiz, 1)

	return f
}

func (f *joinFixture) studentActor() ActorInput {
	return ActorInput{AccountID: f.studentAccount, Role: viewmodel.RoleStudent, FullName: "Siswa Uji"}
}

func (f *joinFixture) teacherActor() ActorInput {
	return ActorInput{AccountID: f.teacher, Role: viewmodel.RoleTeacher, FullName: "Bu Guru"}
}

func (f *joinFixture) identityToken(seed byte) string {
	return "v1." + strings.Repeat(string(rune('A'+seed)), 43)
}

func (f *joinFixture) seedIdentity(token, name string, grade int, majorID int64) int64 {
	f.t.Helper()
	res, err := f.db.Exec(`
		INSERT INTO anonymous_participants
			(display_name, grade_level, major_id, cookie_token_hash)
		VALUES (?, ?, ?, ?)`,
		name, grade, majorID, secure.HashToken(token))
	if err != nil {
		f.t.Fatalf("seed identity %s: %v", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		f.t.Fatalf("seed identity id: %v", err)
	}
	return id
}

func (f *joinFixture) seedAnonRegistration(quizID, anonID int64) int64 {
	f.t.Helper()
	res, err := f.db.Exec(
		"INSERT INTO registrations (quiz_id, anonymous_participant_id) VALUES (?, ?)",
		quizID, anonID)
	if err != nil {
		f.t.Fatalf("seed anonymous registration: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		f.t.Fatalf("seed anonymous registration id: %v", err)
	}
	return id
}

func (f *joinFixture) ticket(quizID int64, method, secret string) string {
	f.t.Helper()
	value, err := mintTicket(f.svc.key, uint32(quizID), method, secret, readTestNow)
	if err != nil {
		f.t.Fatalf("mint ticket: %v", err)
	}
	return value
}

func (f *joinFixture) count(query string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.db.QueryRow(query, args...).Scan(&n); err != nil {
		f.t.Fatalf("count: %v", err)
	}
	return n
}

func (f *joinFixture) identityIDByName(name string) int64 {
	f.t.Helper()
	var id int64
	err := f.db.QueryRow(
		"SELECT id FROM anonymous_participants WHERE display_name = ? AND deleted_at = ?",
		name, store.LiveAt).Scan(&id)
	if err != nil {
		f.t.Fatalf("identity %q: %v", name, err)
	}
	return id
}

// redirectOf unwraps the *Redirect control-flow error the domain returns for a
// 303. A nil error or another type is a test failure.
func redirectOf(t *testing.T, err error) *Redirect {
	t.Helper()
	var r *Redirect
	if !errors.As(err, &r) {
		t.Fatalf("error = %v, want *Redirect", err)
	}
	return r
}

func TestJoinResolveCodeIntegration(t *testing.T) {
	f := newJoinFixture(t)

	t.Run("teacher goes to the teacher area", func(t *testing.T) {
		out, err := f.svc.ResolveCode(f.ctx, ResolveCodeInput{Actor: f.teacherActor(), Code: "123456"})
		if err != nil {
			t.Fatalf("ResolveCode: %v", err)
		}
		if out.Result.Path != "/teacher" || out.Result.Flash != "" {
			t.Fatalf("result = %+v, want /teacher without flash", out.Result)
		}
		if out.Cookies.TicketValue != "" {
			t.Fatalf("teacher received a ticket")
		}
	})

	t.Run("malformed and unknown codes share one outcome", func(t *testing.T) {
		for _, code := range []string{"", "12345", "1234567", "12 456", "abcdef", "１２３４５６", "999999"} {
			out, err := f.svc.ResolveCode(f.ctx, ResolveCodeInput{Actor: ActorInput{}, Code: code})
			if err != nil {
				t.Fatalf("ResolveCode(%q): %v", code, err)
			}
			if out.Result.Path != "/join?error=unknown_code" {
				t.Errorf("code %q path = %q, want unknown_code", code, out.Result.Path)
			}
			if out.Cookies.TicketValue != "" {
				t.Errorf("code %q minted a ticket", code)
			}
		}
	})

	t.Run("draft and closed are state outcomes", func(t *testing.T) {
		for _, code := range []string{"654321", "111111"} {
			out, err := f.svc.ResolveCode(f.ctx, ResolveCodeInput{Actor: ActorInput{}, Code: code})
			if err != nil {
				t.Fatalf("ResolveCode(%q): %v", code, err)
			}
			if out.Result.Path != "/join?error=not_open" {
				t.Errorf("code %q path = %q, want not_open", code, out.Result.Path)
			}
		}
	})

	t.Run("visitor receives a code ticket", func(t *testing.T) {
		out, err := f.svc.ResolveCode(f.ctx, ResolveCodeInput{Actor: ActorInput{}, Code: " 123456 "})
		if err != nil {
			t.Fatalf("ResolveCode: %v", err)
		}
		want := "/join/identify?quiz=" + itoa(uint64(f.publicQuiz))
		if out.Result.Path != want {
			t.Fatalf("path = %q, want %q", out.Result.Path, want)
		}
		tk, err := parseTicket(f.svc.key, out.Cookies.TicketValue, readTestNow)
		if err != nil {
			t.Fatalf("parse ticket: %v", err)
		}
		if tk.QuizID != uint32(f.publicQuiz) || tk.Method != MethodCode || tk.Secret != "123456" {
			t.Fatalf("ticket = %+v, want public code ticket", tk)
		}
	})

	t.Run("student without a registration lands on the intro", func(t *testing.T) {
		out, err := f.svc.ResolveCode(f.ctx, ResolveCodeInput{Actor: f.studentActor(), Code: "123456"})
		if err != nil {
			t.Fatalf("ResolveCode: %v", err)
		}
		if out.Result.Path != quizPath(uint32(f.publicQuiz)) || out.Result.Flash != "" {
			t.Fatalf("result = %+v, want intro without flash", out.Result)
		}
		if out.Cookies.TicketValue != "" {
			t.Fatalf("student received a ticket")
		}
	})

	t.Run("existing registration wins", func(t *testing.T) {
		seedReadRegistration(t, f.db, f.publicQuiz, f.studentID, readTestAgo(time.Hour))
		out, err := f.svc.ResolveCode(f.ctx, ResolveCodeInput{Actor: f.studentActor(), Code: "123456"})
		if err != nil {
			t.Fatalf("ResolveCode: %v", err)
		}
		if out.Result.Path != quizPath(uint32(f.publicQuiz)) || out.Result.Flash != FlashAlreadyRegistered {
			t.Fatalf("result = %+v, want already_registered", out.Result)
		}
	})

	t.Run("anonymous identity with a registration is sent to the intro", func(t *testing.T) {
		token := f.identityToken(1)
		id := f.seedIdentity(token, "Tamu Lama", 10, f.majorID)
		f.seedAnonRegistration(f.publicQuiz, id)
		out, err := f.svc.ResolveCode(f.ctx, ResolveCodeInput{
			Actor: ActorInput{IdentityToken: token}, Code: "123456",
		})
		if err != nil {
			t.Fatalf("ResolveCode: %v", err)
		}
		if out.Result.Path != quizPath(uint32(f.publicQuiz)) || out.Result.Flash != FlashAlreadyRegistered {
			t.Fatalf("result = %+v, want already_registered", out.Result)
		}
	})

	t.Run("anonymous identity without a registration receives a ticket", func(t *testing.T) {
		token := f.identityToken(2)
		f.seedIdentity(token, "Tamu Baru", 10, f.majorID)
		out, err := f.svc.ResolveCode(f.ctx, ResolveCodeInput{
			Actor: ActorInput{IdentityToken: token}, Code: "123456",
		})
		if err != nil {
			t.Fatalf("ResolveCode: %v", err)
		}
		want := "/join/identify?quiz=" + itoa(uint64(f.publicQuiz))
		if out.Result.Path != want || out.Cookies.TicketValue == "" {
			t.Fatalf("result = %+v ticket = %q, want identify redirect with a ticket", out.Result, out.Cookies.TicketValue)
		}
	})
}

func TestJoinInviteIntegration(t *testing.T) {
	f := newJoinFixture(t)

	t.Run("unknown tokens render the landing", func(t *testing.T) {
		for _, token := range []string{"bad token!", "nope-token"} {
			out, err := f.svc.Invite(f.ctx, InviteInput{Actor: ActorInput{}, Token: token})
			if err != nil {
				t.Fatalf("Invite(%q): %v", token, err)
			}
			if out.Page.Outcome == nil || *out.Page.Outcome != viewmodel.JoinUnknownCode {
				t.Errorf("token %q outcome = %v, want unknown_code", token, out.Page.Outcome)
			}
			if out.Page.Quiz != nil {
				t.Errorf("token %q leaked a quiz", token)
			}
		}
	})

	t.Run("closed public quiz explains not_open", func(t *testing.T) {
		out, err := f.svc.Invite(f.ctx, InviteInput{Actor: ActorInput{}, Token: "invite-closed-1"})
		if err != nil {
			t.Fatalf("Invite: %v", err)
		}
		if out.Page.Outcome == nil || *out.Page.Outcome != viewmodel.JoinNotOpen {
			t.Fatalf("outcome = %v, want not_open", out.Page.Outcome)
		}
		if out.Page.Quiz == nil || out.Page.Quiz.Title != "Kuis Tutup" {
			t.Fatalf("quiz = %+v, want the closed quiz title", out.Page.Quiz)
		}
	})

	t.Run("visitor on a public invite receives an invite ticket", func(t *testing.T) {
		out, err := f.svc.Invite(f.ctx, InviteInput{Actor: ActorInput{}, Token: "invite-public-01"})
		r := redirectOf(t, err)
		want := "/join/identify?quiz=" + itoa(uint64(f.publicQuiz))
		if r.Path != want {
			t.Fatalf("redirect = %q, want %q", r.Path, want)
		}
		tk, err := parseTicket(f.svc.key, out.Cookies.TicketValue, readTestNow)
		if err != nil {
			t.Fatalf("parse ticket: %v", err)
		}
		if tk.QuizID != uint32(f.publicQuiz) || tk.Method != MethodInvite || tk.Secret != "invite-public-01" {
			t.Fatalf("ticket = %+v, want public invite ticket", tk)
		}
	})

	t.Run("public invite auto-registers an eligible student once", func(t *testing.T) {
		before := f.count("SELECT COUNT(*) FROM registrations WHERE quiz_id = ? AND deleted_at = ?", f.publicQuiz, store.LiveAt)
		_, err := f.svc.Invite(f.ctx, InviteInput{Actor: f.studentActor(), Token: "invite-public-01"})
		r := redirectOf(t, err)
		if r.Path != quizPath(uint32(f.publicQuiz)) || r.Flash != FlashRegistered {
			t.Fatalf("redirect = %+v, want intro registered", r)
		}
		after := f.count("SELECT COUNT(*) FROM registrations WHERE quiz_id = ? AND deleted_at = ?", f.publicQuiz, store.LiveAt)
		if after != before+1 {
			t.Fatalf("registrations = %d, want %d", after, before+1)
		}

		_, err = f.svc.Invite(f.ctx, InviteInput{Actor: f.studentActor(), Token: "invite-public-01"})
		r = redirectOf(t, err)
		if r.Flash != FlashAlreadyRegistered {
			t.Fatalf("second invite flash = %q, want already_registered", r.Flash)
		}
		if again := f.count("SELECT COUNT(*) FROM registrations WHERE quiz_id = ? AND deleted_at = ?", f.publicQuiz, store.LiveAt); again != after {
			t.Fatalf("second invite changed the count to %d", again)
		}
	})

	t.Run("private invite checks audience", func(t *testing.T) {
		eligible := f.count("SELECT COUNT(*) FROM registrations WHERE quiz_id = ? AND deleted_at = ?", f.privateQuiz, store.LiveAt)
		_, err := f.svc.Invite(f.ctx, InviteInput{Actor: f.studentActor(), Token: "invite-private-1"})
		r := redirectOf(t, err)
		if r.Path != quizPath(uint32(f.privateQuiz)) || r.Flash != FlashRegistered {
			t.Fatalf("eligible redirect = %+v, want registered", r)
		}
		if got := f.count("SELECT COUNT(*) FROM registrations WHERE quiz_id = ? AND deleted_at = ?", f.privateQuiz, store.LiveAt); got != eligible+1 {
			t.Fatalf("eligible registrations = %d, want %d", got, eligible+1)
		}

		otherAccount := seedReadAccount(t, f.db, "siswa10@join.test", "Siswa Sepuluh", "student")
		seedReadStudent(t, f.db, otherAccount, f.majorID, 10)
		out, err := f.svc.Invite(f.ctx, InviteInput{
			Actor: ActorInput{AccountID: otherAccount, Role: viewmodel.RoleStudent, FullName: "Siswa Sepuluh"},
			Token: "invite-private-1",
		})
		if err != nil {
			t.Fatalf("Invite ineligible: %v", err)
		}
		if out.Page.Outcome == nil || *out.Page.Outcome != viewmodel.JoinWrongAudience {
			t.Fatalf("outcome = %v, want wrong_audience", out.Page.Outcome)
		}
	})

	t.Run("private invite refuses anonymous viewers", func(t *testing.T) {
		for name, actor := range map[string]ActorInput{
			"visitor": {},
			"anonymous": {IdentityToken: func() string {
				token := f.identityToken(3)
				f.seedIdentity(token, "Tamu Privat", 11, f.majorID)
				return token
			}()},
		} {
			out, err := f.svc.Invite(f.ctx, InviteInput{Actor: actor, Token: "invite-private-1"})
			if err != nil {
				t.Fatalf("%s Invite: %v", name, err)
			}
			if out.Page.Outcome == nil || *out.Page.Outcome != viewmodel.JoinAnonymousNotAllowed {
				t.Errorf("%s outcome = %v, want anonymous_not_allowed", name, out.Page.Outcome)
			}
		}
	})

	t.Run("teacher sees the owned quiz or a 404", func(t *testing.T) {
		_, err := f.svc.Invite(f.ctx, InviteInput{Actor: f.teacherActor(), Token: "invite-private-1"})
		r := redirectOf(t, err)
		want := "/teacher/quizzes/" + itoa(uint64(f.privateQuiz))
		if r.Path != want {
			t.Fatalf("owner redirect = %q, want %q", r.Path, want)
		}

		foreign := seedReadAccount(t, f.db, "guru2@join.test", "Pak Guru", "teacher")
		_, err = f.svc.Invite(f.ctx, InviteInput{
			Actor: ActorInput{AccountID: foreign, Role: viewmodel.RoleTeacher, FullName: "Pak Guru"},
			Token: "invite-private-1",
		})
		if !apperr.IsNotFound(err) {
			t.Fatalf("foreign teacher error = %v, want 404", err)
		}
	})

	t.Run("registration wins over a closed state", func(t *testing.T) {
		seedReadRegistration(t, f.db, f.closedQuiz, f.studentID, readTestAgo(3*time.Hour))
		_, err := f.svc.Invite(f.ctx, InviteInput{Actor: f.studentActor(), Token: "invite-closed-1"})
		r := redirectOf(t, err)
		if r.Path != quizPath(uint32(f.closedQuiz)) || r.Flash != FlashAlreadyRegistered {
			t.Fatalf("redirect = %+v, want closed intro already_registered", r)
		}
	})
}

func TestIdentifyPageIntegration(t *testing.T) {
	f := newJoinFixture(t)
	openTicket := f.ticket(f.publicQuiz, MethodCode, "123456")

	t.Run("no ticket is 404", func(t *testing.T) {
		_, err := f.svc.IdentifyPage(f.ctx, IdentifyPageInput{Actor: ActorInput{}, QuizID: f.publicQuiz})
		if !apperr.IsNotFound(err) {
			t.Fatalf("error = %v, want 404", err)
		}
	})

	t.Run("mismatched quiz redirects to the ticket quiz", func(t *testing.T) {
		_, err := f.svc.IdentifyPage(f.ctx, IdentifyPageInput{
			Actor: ActorInput{TicketValue: openTicket}, QuizID: f.emptyQuiz,
		})
		r := redirectOf(t, err)
		want := "/join/identify?quiz=" + itoa(uint64(f.publicQuiz))
		if r.Path != want {
			t.Fatalf("redirect = %q, want %q", r.Path, want)
		}
	})

	t.Run("student with a ticket goes to the intro", func(t *testing.T) {
		actor := f.studentActor()
		actor.TicketValue = openTicket
		_, err := f.svc.IdentifyPage(f.ctx, IdentifyPageInput{Actor: actor, QuizID: f.publicQuiz})
		r := redirectOf(t, err)
		if r.Path != quizPath(uint32(f.publicQuiz)) {
			t.Fatalf("redirect = %q, want intro", r.Path)
		}
	})

	t.Run("visitor renders the empty form with live majors", func(t *testing.T) {
		out, err := f.svc.IdentifyPage(f.ctx, IdentifyPageInput{
			Actor: ActorInput{TicketValue: openTicket}, QuizID: f.publicQuiz,
		})
		if err != nil {
			t.Fatalf("IdentifyPage: %v", err)
		}
		if out.Page.Quiz.ID != f.publicQuiz || out.Page.Quiz.Title != "Kuis Publik" {
			t.Fatalf("page quiz = %+v", out.Page.Quiz)
		}
		if len(out.Page.Majors) != 2 {
			t.Fatalf("majors = %d, want 2", len(out.Page.Majors))
		}
		if out.Page.Majors[0].Name != "Desain Komunikasi Visual" || out.Page.Majors[1].Name != "Rekayasa Perangkat Lunak" {
			t.Fatalf("majors not alphabetical: %+v", out.Page.Majors)
		}
		if out.Page.FullName != "" || out.Page.GradeLevel != 0 || out.Page.MajorID != 0 {
			t.Fatalf("visitor form was prefilled: %+v", out.Page)
		}
		if out.Cookies.IdentityClear {
			t.Fatalf("visitor had its identity cleared")
		}
	})

	t.Run("stale identity is cleared on the form", func(t *testing.T) {
		out, err := f.svc.IdentifyPage(f.ctx, IdentifyPageInput{
			Actor:  ActorInput{IdentityToken: f.identityToken(9), TicketValue: openTicket},
			QuizID: f.publicQuiz,
		})
		if err != nil {
			t.Fatalf("IdentifyPage: %v", err)
		}
		if !out.Cookies.IdentityClear {
			t.Fatalf("stale identity was not cleared")
		}
		if out.Page.Quiz.ID != f.publicQuiz {
			t.Fatalf("page quiz = %+v", out.Page.Quiz)
		}
	})

	t.Run("anonymous identity prefills the form", func(t *testing.T) {
		token := f.identityToken(4)
		f.seedIdentity(token, "Tamu Isi", 12, f.otherMajorID)
		out, err := f.svc.IdentifyPage(f.ctx, IdentifyPageInput{
			Actor: ActorInput{IdentityToken: token, TicketValue: openTicket}, QuizID: f.publicQuiz,
		})
		if err != nil {
			t.Fatalf("IdentifyPage: %v", err)
		}
		if out.Page.FullName != "Tamu Isi" || out.Page.GradeLevel != 12 || out.Page.MajorID != f.otherMajorID {
			t.Fatalf("prefill = %+v", out.Page)
		}
	})

	t.Run("closed quiz without a registration clears the ticket", func(t *testing.T) {
		closedTicket := f.ticket(f.closedQuiz, MethodCode, "111111")
		out, err := f.svc.IdentifyPage(f.ctx, IdentifyPageInput{
			Actor: ActorInput{TicketValue: closedTicket}, QuizID: f.closedQuiz,
		})
		r := redirectOf(t, err)
		if r.Path != "/join?error=not_open" {
			t.Fatalf("redirect = %q, want not_open", r.Path)
		}
		if !out.Cookies.TicketClear {
			t.Fatalf("ticket was not cleared")
		}
	})

	t.Run("closed quiz with a registration goes to the intro", func(t *testing.T) {
		token := f.identityToken(5)
		id := f.seedIdentity(token, "Tamu Tutup", 10, f.majorID)
		f.seedAnonRegistration(f.closedQuiz, id)
		closedTicket := f.ticket(f.closedQuiz, MethodCode, "111111")
		out, err := f.svc.IdentifyPage(f.ctx, IdentifyPageInput{
			Actor: ActorInput{IdentityToken: token, TicketValue: closedTicket}, QuizID: f.closedQuiz,
		})
		r := redirectOf(t, err)
		if r.Path != quizPath(uint32(f.closedQuiz)) {
			t.Fatalf("redirect = %q, want intro", r.Path)
		}
		if out.Cookies.TicketClear {
			t.Fatalf("ticket cleared despite a live registration")
		}
	})

	t.Run("switch clears the identity and rotates the ticket nonce", func(t *testing.T) {
		token := f.identityToken(6)
		id := f.seedIdentity(token, "Tamu Ganti", 10, f.majorID)
		tk := f.ticket(f.switchQuiz, MethodCode, "333333")
		before, err := parseTicket(f.svc.key, tk, readTestNow)
		if err != nil {
			t.Fatalf("parse old ticket: %v", err)
		}

		out, err := f.svc.IdentifyPage(f.ctx, IdentifyPageInput{
			Actor:  ActorInput{IdentityToken: token, TicketValue: tk},
			QuizID: f.switchQuiz,
			Switch: true,
		})
		r := redirectOf(t, err)
		want := "/join/identify?quiz=" + itoa(uint64(f.switchQuiz))
		if r.Path != want || r.Flash != FlashIdentitySwitched {
			t.Fatalf("redirect = %+v, want %s with identity-switched", r, want)
		}
		if !out.Cookies.IdentityClear {
			t.Fatalf("identity cookie was not cleared")
		}
		if out.Cookies.TicketValue == "" || out.Cookies.TicketValue == tk {
			t.Fatalf("ticket was not rotated: %q", out.Cookies.TicketValue)
		}
		after, err := parseTicket(f.svc.key, out.Cookies.TicketValue, readTestNow)
		if err != nil {
			t.Fatalf("parse new ticket: %v", err)
		}
		if after.QuizID != before.QuizID || after.Method != before.Method || after.Secret != before.Secret {
			t.Fatalf("rotated ticket changed bindings: before %+v after %+v", before, after)
		}
		if after.Nonce == before.Nonce {
			t.Fatalf("rotated ticket kept the nonce")
		}
		if got := f.count("SELECT COUNT(*) FROM anonymous_participants WHERE id = ? AND deleted_at = ?", id, store.LiveAt); got != 1 {
			t.Fatalf("switch deleted or hid the old identity")
		}

		// The canonical form is reachable with the rotated ticket and a cleared
		// identity cookie, and repeating switch without an identity is a plain
		// GET (AC-17).
		plain, err := f.svc.IdentifyPage(f.ctx, IdentifyPageInput{
			Actor: ActorInput{TicketValue: out.Cookies.TicketValue}, QuizID: f.switchQuiz, Switch: true,
		})
		if err != nil {
			t.Fatalf("IdentifyPage after switch: %v", err)
		}
		if plain.Page.Quiz.ID != f.switchQuiz || plain.Page.FullName != "" {
			t.Fatalf("repeat switch page = %+v", plain.Page)
		}
		if plain.Cookies.IdentityClear {
			t.Fatalf("repeat switch cleared an absent identity")
		}
	})
}

func TestIdentifyIntegration(t *testing.T) {
	f := newJoinFixture(t)

	t.Run("validation failures do not write", func(t *testing.T) {
		ticket := f.ticket(f.publicQuiz, MethodCode, "123456")
		res, err := f.svc.Identify(f.ctx, IdentifyInput{
			QuizID: f.publicQuiz,
			Actor:  ActorInput{TicketValue: ticket},
			Form:   IdentityForm{FullName: "   ", GradeLevel: "9", MajorID: "0"},
		})
		fields, ok := FieldErrorsOf(err)
		if !ok {
			t.Fatalf("error = %v, want field errors", err)
		}
		for _, name := range []string{"full_name", "grade_level", "major_id"} {
			if fields[name] == "" {
				t.Errorf("missing field error %q in %+v", name, fields)
			}
		}
		if res.Page.Quiz.ID != f.publicQuiz {
			t.Errorf("422 page quiz = %+v", res.Page.Quiz)
		}
		if n := f.count("SELECT COUNT(*) FROM anonymous_participants"); n != 0 {
			t.Errorf("validation created %d identities", n)
		}
		if n := f.count("SELECT COUNT(*) FROM registrations WHERE quiz_id = ?", f.publicQuiz); n != 0 {
			t.Errorf("validation created %d registrations", n)
		}
	})

	t.Run("a soft-deleted major is a field error", func(t *testing.T) {
		majorID := seedReadMajor(t, f.db, "Jurusan Hapus", "HPS")
		if _, err := f.db.Exec("UPDATE majors SET deleted_at = UTC_TIMESTAMP() WHERE id = ?", majorID); err != nil {
			t.Fatalf("soft-delete major: %v", err)
		}
		ticket := f.ticket(f.publicQuiz, MethodCode, "123456")
		_, err := f.svc.Identify(f.ctx, IdentifyInput{
			QuizID: f.publicQuiz,
			Actor:  ActorInput{TicketValue: ticket},
			Form:   IdentityForm{FullName: "Budi Santosa", GradeLevel: "10", MajorID: itoa(uint64(majorID))},
		})
		fields, ok := FieldErrorsOf(err)
		if !ok || fields["major_id"] == "" {
			t.Fatalf("error = %v fields = %+v, want major_id", err, fields)
		}
	})

	var (
		createdToken string
		createdID    int64
		createTicket string
	)
	t.Run("valid identify creates the identity and registration", func(t *testing.T) {
		createTicket = f.ticket(f.publicQuiz, MethodCode, "123456")
		out, err := f.svc.Identify(f.ctx, IdentifyInput{
			QuizID: f.publicQuiz,
			Actor:  ActorInput{TicketValue: createTicket},
			Form:   IdentityForm{FullName: "  Budi   Santosa ", GradeLevel: "10", MajorID: itoa(uint64(f.majorID))},
		})
		if err != nil {
			t.Fatalf("Identify: %v", err)
		}
		if out.Out.Result.Path != quizPath(uint32(f.publicQuiz)) || out.Out.Result.Flash != FlashRegistered {
			t.Fatalf("result = %+v, want intro registered", out.Out.Result)
		}
		if out.Out.Cookies.IdentityToken == "" || !out.Out.Cookies.TicketClear {
			t.Fatalf("cookies = %+v, want identity and ticket clear", out.Out.Cookies)
		}
		createdToken = out.Out.Cookies.IdentityToken
		createdID = f.identityIDByName("Budi Santosa")

		var grade int
		var majorID int64
		if err := f.db.QueryRow(
			"SELECT grade_level, major_id FROM anonymous_participants WHERE id = ?", createdID).
			Scan(&grade, &majorID); err != nil {
			t.Fatalf("read identity: %v", err)
		}
		if grade != 10 || majorID != f.majorID {
			t.Fatalf("identity = grade %d major %d", grade, majorID)
		}
		if n := f.count("SELECT COUNT(*) FROM registrations WHERE quiz_id = ? AND anonymous_participant_id = ? AND deleted_at = ?",
			f.publicQuiz, createdID, store.LiveAt); n != 1 {
			t.Fatalf("registrations = %d, want 1", n)
		}
	})

	t.Run("replay with the identity reuses it", func(t *testing.T) {
		out, err := f.svc.Identify(f.ctx, IdentifyInput{
			QuizID: f.publicQuiz,
			Actor:  ActorInput{IdentityToken: createdToken, TicketValue: createTicket},
			Form:   IdentityForm{FullName: "Budi Santosa", GradeLevel: "10", MajorID: itoa(uint64(f.majorID))},
		})
		if err != nil {
			t.Fatalf("Identify replay: %v", err)
		}
		if out.Out.Result.Flash != FlashAlreadyRegistered {
			t.Fatalf("flash = %q, want already_registered", out.Out.Result.Flash)
		}
		if n := f.count("SELECT COUNT(*) FROM anonymous_participants"); n != 1 {
			t.Fatalf("identities = %d, want 1", n)
		}
		if n := f.count("SELECT COUNT(*) FROM registrations WHERE quiz_id = ?", f.publicQuiz); n != 1 {
			t.Fatalf("registrations = %d, want 1", n)
		}
	})

	t.Run("replay without the identity converges on the ticket nonce", func(t *testing.T) {
		out, err := f.svc.Identify(f.ctx, IdentifyInput{
			QuizID: f.publicQuiz,
			Actor:  ActorInput{TicketValue: createTicket},
			Form:   IdentityForm{FullName: "Budi Santosa", GradeLevel: "10", MajorID: itoa(uint64(f.majorID))},
		})
		if err != nil {
			t.Fatalf("Identify replay: %v", err)
		}
		if out.Out.Result.Flash != FlashAlreadyRegistered {
			t.Fatalf("flash = %q, want already_registered", out.Out.Result.Flash)
		}
		if out.Out.Cookies.IdentityToken != createdToken {
			t.Fatalf("replay token differs from the derived identity token")
		}
		if n := f.count("SELECT COUNT(*) FROM anonymous_participants"); n != 1 {
			t.Fatalf("identities = %d, want 1", n)
		}
	})

	t.Run("empty quiz keeps the ticket and creates nothing", func(t *testing.T) {
		ticket := f.ticket(f.emptyQuiz, MethodCode, "222222")
		out, err := f.svc.Identify(f.ctx, IdentifyInput{
			QuizID: f.emptyQuiz,
			Actor:  ActorInput{TicketValue: ticket},
			Form:   IdentityForm{FullName: "Tamu Kosong", GradeLevel: "10", MajorID: itoa(uint64(f.majorID))},
		})
		if err != nil {
			t.Fatalf("Identify: %v", err)
		}
		want := "/join/identify?quiz=" + itoa(uint64(f.emptyQuiz))
		if out.Out.Result.Path != want || out.Out.Result.Flash != FlashQuizEmpty {
			t.Fatalf("result = %+v, want quiz-empty identify redirect", out.Out.Result)
		}
		if out.Out.Cookies.TicketClear || out.Out.Cookies.TicketValue != "" {
			t.Fatalf("empty quiz touched the ticket: %+v", out.Out.Cookies)
		}
		if n := f.count("SELECT COUNT(*) FROM anonymous_participants WHERE display_name = ?", "Tamu Kosong"); n != 0 {
			t.Fatalf("empty quiz created %d identities", n)
		}
	})

	t.Run("closed quiz without a registration clears the ticket", func(t *testing.T) {
		ticket := f.ticket(f.closedQuiz, MethodCode, "111111")
		out, err := f.svc.Identify(f.ctx, IdentifyInput{
			QuizID: f.closedQuiz,
			Actor:  ActorInput{TicketValue: ticket},
			Form:   IdentityForm{FullName: "Tamu Tutup", GradeLevel: "10", MajorID: itoa(uint64(f.majorID))},
		})
		if err != nil {
			t.Fatalf("Identify: %v", err)
		}
		if out.Out.Result.Path != "/join?error=not_open" || !out.Out.Cookies.TicketClear {
			t.Fatalf("result = %+v cookies = %+v, want not_open and ticket clear", out.Out.Result, out.Out.Cookies)
		}
		if n := f.count("SELECT COUNT(*) FROM anonymous_participants WHERE display_name = ?", "Tamu Tutup"); n != 0 {
			t.Fatalf("closed quiz created %d identities", n)
		}
	})

	t.Run("closed quiz with a registration refreshes the profile only", func(t *testing.T) {
		token := f.identityToken(7)
		id := f.seedIdentity(token, "Nama Lama", 10, f.majorID)
		f.seedAnonRegistration(f.closedQuiz, id)
		ticket := f.ticket(f.closedQuiz, MethodCode, "111111")
		out, err := f.svc.Identify(f.ctx, IdentifyInput{
			QuizID: f.closedQuiz,
			Actor:  ActorInput{IdentityToken: token, TicketValue: ticket},
			Form:   IdentityForm{FullName: "Nama Baru", GradeLevel: "12", MajorID: itoa(uint64(f.otherMajorID))},
		})
		if err != nil {
			t.Fatalf("Identify: %v", err)
		}
		if out.Out.Result.Path != quizPath(uint32(f.closedQuiz)) || out.Out.Result.Flash != "" {
			t.Fatalf("result = %+v, want closed intro without flash", out.Out.Result)
		}
		if !out.Out.Cookies.TicketClear || out.Out.Cookies.IdentityToken != token {
			t.Fatalf("cookies = %+v, want ticket clear and identity refresh", out.Out.Cookies)
		}
		var name string
		var grade int
		var majorID int64
		if err := f.db.QueryRow(
			"SELECT display_name, grade_level, major_id FROM anonymous_participants WHERE id = ?", id).
			Scan(&name, &grade, &majorID); err != nil {
			t.Fatalf("read identity: %v", err)
		}
		if name != "Nama Baru" || grade != 12 || majorID != f.otherMajorID {
			t.Fatalf("identity = %s/%d/%d", name, grade, majorID)
		}
		if n := f.count("SELECT COUNT(*) FROM registrations WHERE quiz_id = ? AND anonymous_participant_id = ? AND deleted_at = ?",
			f.closedQuiz, id, store.LiveAt); n != 1 {
			t.Fatalf("registrations = %d, want 1", n)
		}
	})

	t.Run("wrong quiz in the body is 404", func(t *testing.T) {
		ticket := f.ticket(f.publicQuiz, MethodCode, "123456")
		_, err := f.svc.Identify(f.ctx, IdentifyInput{
			QuizID: f.emptyQuiz,
			Actor:  ActorInput{TicketValue: ticket},
			Form:   IdentityForm{FullName: "Budi Santosa", GradeLevel: "10", MajorID: itoa(uint64(f.majorID))},
		})
		if !apperr.IsNotFound(err) {
			t.Fatalf("error = %v, want 404", err)
		}
	})
}

func TestRegisterStartAttemptIntegration(t *testing.T) {
	f := newJoinFixture(t)

	t.Run("register is idempotent", func(t *testing.T) {
		res, err := f.svc.Register(f.ctx, RegisterInput{QuizID: f.publicQuiz, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("Register: %v", err)
		}
		if res.Path != quizPath(uint32(f.publicQuiz)) || res.Flash != FlashRegistered {
			t.Fatalf("result = %+v, want registered", res)
		}
		res, err = f.svc.Register(f.ctx, RegisterInput{QuizID: f.publicQuiz, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("Register repeat: %v", err)
		}
		if res.Flash != FlashAlreadyRegistered {
			t.Fatalf("repeat flash = %q, want already_registered", res.Flash)
		}
		if n := f.count("SELECT COUNT(*) FROM registrations WHERE quiz_id = ? AND student_id = ? AND deleted_at = ?",
			f.publicQuiz, f.studentID, store.LiveAt); n != 1 {
			t.Fatalf("registrations = %d, want 1", n)
		}
	})

	startStub := func(ctx context.Context, tx *sql.Tx, plan StartPlan) (uint32, error) {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO quiz_attempts (registration_id, attempt_no, shuffle_seed)
			VALUES (?, ?, 1)`, plan.RegistrationID, plan.AttemptNo)
		if err != nil {
			return 0, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return 0, err
		}
		return uint32(id), nil
	}
	f.svc.AttemptStart = startStub

	t.Run("start then resume reuses the open attempt", func(t *testing.T) {
		res, err := f.svc.StartAttempt(f.ctx, StartQuizInput{QuizID: f.publicQuiz, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("StartAttempt: %v", err)
		}
		if !strings.HasPrefix(res.Path, "/attempt/") || res.Flash != FlashStarted {
			t.Fatalf("result = %+v, want a started attempt", res)
		}
		firstPath := res.Path

		res, err = f.svc.StartAttempt(f.ctx, StartQuizInput{QuizID: f.publicQuiz, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("StartAttempt resume: %v", err)
		}
		if res.Path != firstPath || res.Flash != "" {
			t.Fatalf("resume = %+v, want %s without flash", res, firstPath)
		}
		if n := f.count("SELECT COUNT(*) FROM quiz_attempts WHERE registration_id = (SELECT id FROM registrations WHERE quiz_id = ? AND student_id = ? AND deleted_at = ?) AND deleted_at = ?",
			f.publicQuiz, f.studentID, store.LiveAt, store.LiveAt); n != 1 {
			t.Fatalf("attempts = %d, want 1", n)
		}
	})

	t.Run("spent quota is a flash, not a new attempt", func(t *testing.T) {
		regID := f.count("SELECT id FROM registrations WHERE quiz_id = ? AND student_id = ? AND deleted_at = ?",
			f.publicQuiz, f.studentID, store.LiveAt)
		if regID == 0 {
			t.Fatal("fixture registration missing")
		}
		if _, err := f.db.Exec(`
			UPDATE quiz_attempts SET submitted_at = UTC_TIMESTAMP(), final_score = 0.5000
			WHERE registration_id = ? AND deleted_at = ?`, regID, store.LiveAt); err != nil {
			t.Fatalf("submit attempt: %v", err)
		}
		res, err := f.svc.StartAttempt(f.ctx, StartQuizInput{QuizID: f.publicQuiz, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("StartAttempt: %v", err)
		}
		if res.Path != quizPath(uint32(f.publicQuiz)) || res.Flash != FlashAttemptsExhausted {
			t.Fatalf("result = %+v, want attempts-exhausted", res)
		}
	})

	t.Run("anonymous start uses the same seam", func(t *testing.T) {
		token := f.identityToken(8)
		id := f.seedIdentity(token, "Tamu Mulai", 10, f.majorID)
		f.seedAnonRegistration(f.oneShotQuiz, id)
		var plan StartPlan
		f.svc.AttemptStart = func(ctx context.Context, tx *sql.Tx, p StartPlan) (uint32, error) {
			plan = p
			return startStub(ctx, tx, p)
		}
		res, err := f.svc.StartAttempt(f.ctx, StartQuizInput{
			QuizID: f.oneShotQuiz, Actor: ActorInput{IdentityToken: token},
		})
		if err != nil {
			t.Fatalf("StartAttempt: %v", err)
		}
		if !strings.HasPrefix(res.Path, "/attempt/") || res.Flash != FlashStarted {
			t.Fatalf("result = %+v, want a started attempt", res)
		}
		if plan.AnonymousParticipantID != uint32(id) || plan.StudentID != 0 || plan.AttemptNo != 1 {
			t.Fatalf("plan = %+v, want anonymous attempt 1", plan)
		}
	})

	t.Run("student without a registration is refused with a flash", func(t *testing.T) {
		res, err := f.svc.StartAttempt(f.ctx, StartQuizInput{QuizID: f.cancelQuiz, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("StartAttempt: %v", err)
		}
		if res.Path != quizPath(uint32(f.cancelQuiz)) || res.Flash != FlashNotRegistered {
			t.Fatalf("result = %+v, want not-registered", res)
		}
	})

	t.Run("cancel removes only a registration with no attempts", func(t *testing.T) {
		if _, err := f.svc.Register(f.ctx, RegisterInput{QuizID: f.cancelQuiz, Actor: f.studentActor()}); err != nil {
			t.Fatalf("Register: %v", err)
		}
		res, err := f.svc.Cancel(f.ctx, CancelInput{QuizID: f.cancelQuiz, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("Cancel: %v", err)
		}
		if res.Path != quizPath(uint32(f.cancelQuiz)) || res.Flash != FlashCancelled {
			t.Fatalf("result = %+v, want cancelled", res)
		}
		if n := f.count("SELECT COUNT(*) FROM registrations WHERE quiz_id = ? AND student_id = ? AND deleted_at = ?",
			f.cancelQuiz, f.studentID, store.LiveAt); n != 0 {
			t.Fatalf("live registrations = %d, want 0", n)
		}
		_, err = f.svc.Cancel(f.ctx, CancelInput{QuizID: f.cancelQuiz, Actor: f.studentActor()})
		if !apperr.IsNotFound(err) {
			t.Fatalf("second cancel error = %v, want 404", err)
		}
	})
}
