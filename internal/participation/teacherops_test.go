package participation

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
	"quiz-v2/internal/store/storetest"
	"quiz-v2/internal/viewmodel"
)

// catalogueFixture is one reset database with an active teacher and one live
// major. Integration tests skip when TEST_DB_DSN is unset (storetest.DB).
type catalogueFixture struct {
	t         *testing.T
	db        *store.DB
	svc       *Service
	ctx       context.Context
	teacherID int64
	majorID   int64
}

func newCatalogueFixture(t *testing.T) *catalogueFixture {
	t.Helper()
	db := storetest.DB(t)
	storetest.Reset(t, db)
	f := &catalogueFixture{
		t:   t,
		db:  db,
		svc: New(db, []byte("participation-test-signing-key")),
		ctx: context.Background(),
	}
	f.teacherID = f.seedTeacher("guru@example.test", "Guru Pengelola")
	f.majorID = f.seedMajor("Rekayasa Perangkat Lunak", "RPL")
	return f
}

func (f *catalogueFixture) actor() ActorInput {
	return ActorInput{AccountID: f.teacherID, Role: viewmodel.RoleTeacher}
}

func (f *catalogueFixture) id(res sql.Result, what string) int64 {
	f.t.Helper()
	id, err := res.LastInsertId()
	if err != nil {
		f.t.Fatalf("seed %s id: %v", what, err)
	}
	return id
}

func (f *catalogueFixture) exec(query string, args ...any) sql.Result {
	f.t.Helper()
	res, err := f.db.ExecContext(f.ctx, query, args...)
	if err != nil {
		f.t.Fatalf("seed query %q: %v", query, err)
	}
	return res
}

func (f *catalogueFixture) seedTeacher(email, name string) int64 {
	f.t.Helper()
	return f.id(f.exec(`INSERT INTO accounts
		(full_name, email, password_hash, role, status, approved_at)
		VALUES (?, ?, 'not-a-real-hash', 'teacher', 'active', UTC_TIMESTAMP())`,
		name, email), "teacher")
}

func (f *catalogueFixture) seedMajor(name, code string) int64 {
	f.t.Helper()
	return f.id(f.exec("INSERT INTO majors (name, code) VALUES (?, ?)", name, code), "major")
}

// seedAnonAt inserts one live anonymous identity with explicit activity stamps
// so sorting is deterministic regardless of the wall clock.
func (f *catalogueFixture) seedAnonAt(name string, grade int, majorID int64, b byte, lastSeen, createdAt string) int64 {
	f.t.Helper()
	hash := make([]byte, 32)
	hash[0] = b
	return f.id(f.exec(`INSERT INTO anonymous_participants
		(display_name, grade_level, major_id, cookie_token_hash, last_seen_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		name, grade, majorID, hash, lastSeen, createdAt), "anonymous participant")
}

func (f *catalogueFixture) seedAnon(name string, grade int, majorID int64, b byte, at string) int64 {
	return f.seedAnonAt(name, grade, majorID, b, at, at)
}

// seedQuiz inserts a private draft targeting the given major.
func (f *catalogueFixture) seedQuiz(majorID int64, invite string) int64 {
	f.t.Helper()
	return f.id(f.exec(`INSERT INTO quizzes
		(owner_account_id, title, visibility, state, invite_token, target_grade, target_major_id)
		VALUES (?, ?, 'private', 'draft', ?, 10, ?)`,
		f.teacherID, "Kuis "+invite, invite, majorID), "quiz")
}

func (f *catalogueFixture) register(quizID, anonID int64, deleted string) int64 {
	f.t.Helper()
	if deleted == "" {
		return f.id(f.exec(`INSERT INTO registrations
			(quiz_id, anonymous_participant_id) VALUES (?, ?)`, quizID, anonID), "registration")
	}
	return f.id(f.exec(`INSERT INTO registrations
		(quiz_id, anonymous_participant_id, deleted_at) VALUES (?, ?, ?)`,
		quizID, anonID, deleted), "deleted registration")
}

func TestMajorCodeBase(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Teknik Komputer dan Jaringan", "TEKNIKKOMP"},
		{"ipa", "IPA"},
		{"  ", "JUR"},
		{"!!!", "JUR"},
		{"TKJ 1", "TKJ1"},
		{"Rekayasa Perangkat Lunak 2", "REKAYASAPE"},
		{"abcdefghijklm", "ABCDEFGHIJ"},
	}
	for _, tc := range cases {
		if got := majorCodeBase(tc.name); got != tc.want {
			t.Errorf("majorCodeBase(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := fitMajorCode("ABCDEFGHIJ", "2"); got != "ABCDEFGHI2" {
		t.Errorf("fitMajorCode numeric suffix = %q", got)
	}
	if got := fitMajorCode("IPA", "1234"); got != "IPA1234" {
		t.Errorf("fitMajorCode short base = %q", got)
	}
}

func TestRandomMajorSuffixShape(t *testing.T) {
	suffix, err := randomMajorSuffix(4)
	if err != nil {
		t.Fatalf("randomMajorSuffix: %v", err)
	}
	if len(suffix) != 4 {
		t.Fatalf("suffix length = %d, want 4", len(suffix))
	}
	for _, r := range suffix {
		if !strings.ContainsRune(majorCodeAlphabet, r) {
			t.Fatalf("suffix %q contains %q outside the ASCII code alphabet", suffix, r)
		}
	}
}

func TestAnonymousListPaginationSearchAndEmptyCopy(t *testing.T) {
	f := newCatalogueFixture(t)
	for i := 1; i <= 12; i++ {
		at := fmt.Sprintf("2026-01-01 00:%02d:00", i)
		f.seedAnon(fmt.Sprintf("Tamu %02d", i), 10, f.majorID, byte(i), at)
	}

	first, err := f.svc.AnonymousList(f.ctx, AnonymousListInput{
		Actor: f.actor(), Page: "1",
	})
	if err != nil {
		t.Fatalf("AnonymousList: %v", err)
	}
	if first.Rows.Total != 12 || len(first.Rows.Items) != 10 || first.Rows.Page != 1 {
		t.Fatalf("page 1 = total %d, rows %d, page %d", first.Rows.Total, len(first.Rows.Items), first.Rows.Page)
	}
	if got := first.Rows.Items[0].FullName; got != "Tamu 12" {
		t.Errorf("first row = %q, want Tamu 12 (last activity desc)", got)
	}
	if first.Empty.Action != nil {
		t.Errorf("truly empty copy must not carry a reset action")
	}

	second, err := f.svc.AnonymousList(f.ctx, AnonymousListInput{
		Actor: f.actor(), Page: "99",
	})
	if err != nil {
		t.Fatalf("AnonymousList page 99: %v", err)
	}
	if second.Rows.Page != 2 || len(second.Rows.Items) != 2 {
		t.Errorf("bad page must clamp to 2: page %d, rows %d", second.Rows.Page, len(second.Rows.Items))
	}

	search, err := f.svc.AnonymousList(f.ctx, AnonymousListInput{
		Actor: f.actor(), Query: "  TAMU 1  ",
	})
	if err != nil {
		t.Fatalf("AnonymousList search: %v", err)
	}
	if search.Rows.Total != 3 || search.Query.Q != "TAMU 1" {
		t.Errorf("case-insensitive search total = %d, q = %q", search.Rows.Total, search.Query.Q)
	}

	long := strings.Repeat("x", 101)
	miss, err := f.svc.AnonymousList(f.ctx, AnonymousListInput{
		Actor: f.actor(), Query: long,
	})
	if err != nil {
		t.Fatalf("AnonymousList overlong q: %v", err)
	}
	if miss.Rows.Total != 0 || miss.Query.Q != strings.Repeat("x", 100) {
		t.Errorf("overlong q must truncate to 100 runes: total %d, len %d", miss.Rows.Total, len(miss.Query.Q))
	}
	if miss.Empty.Title != "Tidak ada peserta anonim yang cocok." || miss.Empty.Action == nil {
		t.Errorf("filtered empty copy = %q action %v", miss.Empty.Title, miss.Empty.Action)
	}
}

func TestAnonymousListEmptyAndCounts(t *testing.T) {
	f := newCatalogueFixture(t)
	empty, err := f.svc.AnonymousList(f.ctx, AnonymousListInput{Actor: f.actor()})
	if err != nil {
		t.Fatalf("AnonymousList empty: %v", err)
	}
	if empty.Empty.Title != "Belum ada peserta anonim." || empty.Empty.Action != nil {
		t.Errorf("empty copy = %q action %v", empty.Empty.Title, empty.Empty.Action)
	}

	anonID := f.seedAnonAt("Tamu Aktif", 11, f.majorID, 1,
		"2025-06-01 00:00:00", "2025-01-01 00:00:00")
	q1 := f.seedQuiz(f.majorID, "INVITE-1")
	q2 := f.seedQuiz(f.majorID, "INVITE-2")
	q3 := f.seedQuiz(f.majorID, "INVITE-3")
	f.register(q1, anonID, "")
	reg2 := f.register(q2, anonID, "")
	f.register(q3, anonID, "2025-02-02 00:00:00")
	f.exec(`UPDATE registrations SET created_at = '2025-03-03 00:00:00'
		WHERE anonymous_participant_id = ? AND deleted_at = ?`, anonID, store.LiveAt)
	f.exec(`INSERT INTO quiz_attempts
		(registration_id, attempt_no, shuffle_seed, updated_at)
		VALUES (?, 1, 42, '2026-09-09 09:09:09')`, reg2)

	list, err := f.svc.AnonymousList(f.ctx, AnonymousListInput{Actor: f.actor(), Query: "aktif"})
	if err != nil {
		t.Fatalf("AnonymousList counts: %v", err)
	}
	if len(list.Rows.Items) != 1 {
		t.Fatalf("rows = %d, want 1", len(list.Rows.Items))
	}
	row := list.Rows.Items[0]
	if row.QuizzesJoined != 2 {
		t.Errorf("QuizzesJoined = %d, want 2 live registrations", row.QuizzesJoined)
	}
	want := time.Date(2026, 9, 9, 9, 9, 9, 0, time.UTC)
	if row.LastActivity == nil || !row.LastActivity.Time().Equal(want) {
		t.Errorf("LastActivity = %v, want %v", row.LastActivity, want)
	}
	if row.ClassLabel != "XI Rekayasa Perangkat Lunak" {
		t.Errorf("ClassLabel = %q", row.ClassLabel)
	}
}

func TestAnonymousFormAndEdit(t *testing.T) {
	f := newCatalogueFixture(t)
	anonID := f.seedAnon("Nama Lama", 10, f.majorID, 7, "2026-05-05 05:05:05")

	form, err := f.svc.AnonymousFormPage(f.ctx, AnonymousFormInput{Actor: f.actor(), ID: anonID})
	if err != nil {
		t.Fatalf("AnonymousFormPage: %v", err)
	}
	if form.ID != anonID || form.FullName != "Nama Lama" || form.GradeLevel != 10 ||
		form.MajorID != f.majorID || len(form.Majors) == 0 ||
		form.Action != fmt.Sprintf("/teacher/anonymous/%d", anonID) {
		t.Fatalf("unexpected form: %+v", form)
	}
	if form.CancelHref != "/teacher/anonymous" {
		t.Errorf("CancelHref = %q", form.CancelHref)
	}

	missing, err := f.svc.AnonymousFormPage(f.ctx, AnonymousFormInput{Actor: f.actor(), ID: anonID + 999})
	if !apperr.IsNotFound(err) {
		t.Fatalf("missing form err = %v, want 404 (form %+v)", err, missing)
	}

	res, err := f.svc.EditAnonymous(f.ctx, AnonymousEditInput{
		Actor:      f.actor(),
		ID:         anonID,
		FullName:   "  Budi   Santoso ",
		GradeLevel: "11",
		MajorID:    fmt.Sprintf("%d", f.majorID),
	})
	if err != nil {
		t.Fatalf("EditAnonymous: %v", err)
	}
	if res.Result.Path != "/teacher/anonymous" || res.Result.Flash != "saved" {
		t.Errorf("edit result = %+v", res.Result)
	}
	var name string
	var grade int
	var majorID int64
	var lastSeen time.Time
	if err := f.db.QueryRowContext(f.ctx, `
		SELECT display_name, grade_level, major_id, last_seen_at
		FROM anonymous_participants WHERE id = ?`, anonID).Scan(&name, &grade, &majorID, &lastSeen); err != nil {
		t.Fatalf("read edited row: %v", err)
	}
	if name != "Budi Santoso" || grade != 11 || majorID != f.majorID {
		t.Errorf("edited row = %q/%d/%d", name, grade, majorID)
	}
	if !lastSeen.Equal(time.Date(2026, 5, 5, 5, 5, 5, 0, time.UTC)) {
		t.Errorf("last_seen_at moved to %v", lastSeen)
	}

	bad, err := f.svc.EditAnonymous(f.ctx, AnonymousEditInput{
		Actor:      f.actor(),
		ID:         anonID,
		FullName:   "A",
		GradeLevel: "11",
		MajorID:    fmt.Sprintf("%d", f.majorID),
	})
	if err == nil {
		t.Fatal("short name must fail")
	}
	if msg := fieldError(t, err, "full_name"); msg != ErrNameShort {
		t.Errorf("full_name error = %q", msg)
	}
	if bad.Form.GradeLevel != 11 || bad.Form.FullName != "A" {
		t.Errorf("422 form did not echo submitted values: %+v", bad.Form)
	}
	var still string
	if err := f.db.QueryRowContext(f.ctx,
		"SELECT display_name FROM anonymous_participants WHERE id = ?", anonID).Scan(&still); err != nil {
		t.Fatalf("read row after 422: %v", err)
	}
	if still != "Budi Santoso" {
		t.Errorf("422 wrote %q", still)
	}

	f.exec(`UPDATE anonymous_participants SET deleted_at = UTC_TIMESTAMP() WHERE id = ?`, anonID)
	if _, err := f.svc.EditAnonymous(f.ctx, AnonymousEditInput{
		Actor: f.actor(), ID: anonID, FullName: "Nama Baru", GradeLevel: "10",
		MajorID: fmt.Sprintf("%d", f.majorID),
	}); !apperr.IsNotFound(err) {
		t.Errorf("deleted identity edit err = %v, want 404", err)
	}
}

func TestDeleteAnonymousCascade(t *testing.T) {
	f := newCatalogueFixture(t)
	anonID := f.seedAnon("Tamu Hapus", 10, f.majorID, 9, "2026-01-01 00:00:00")
	quizID := f.seedQuiz(f.majorID, "INVITE-DEL")
	regID := f.register(quizID, anonID, "")
	questionID := f.id(f.exec(`INSERT INTO questions (quiz_id, position, type, prompt)
		VALUES (?, 1, 'single', 'Soal satu')`, quizID), "question")
	optionID := f.id(f.exec(`INSERT INTO answer_options (question_id, position, body, is_correct)
		VALUES (?, 1, 'Pilihan', 1)`, questionID), "option")
	attemptID := f.id(f.exec(`INSERT INTO quiz_attempts (registration_id, attempt_no, shuffle_seed)
		VALUES (?, 1, 7)`, regID), "attempt")
	answerID := f.id(f.exec(`INSERT INTO attempt_answers (attempt_id, question_id)
		VALUES (?, ?)`, attemptID, questionID), "answer")
	selectionID := f.id(f.exec(`INSERT INTO attempt_answer_selections (attempt_answer_id, option_id)
		VALUES (?, ?)`, answerID, optionID), "selection")
	focusID := f.id(f.exec(`INSERT INTO focus_loss_events (attempt_id, kind)
		VALUES (?, 'blur')`, attemptID), "focus event")

	res, err := f.svc.DeleteAnonymous(f.ctx, AnonymousDeleteInput{Actor: f.actor(), ID: anonID})
	if err != nil {
		t.Fatalf("DeleteAnonymous: %v", err)
	}
	if res.Path != "/teacher/anonymous" || res.Flash != "deleted" {
		t.Errorf("delete result = %+v", res)
	}

	var identityDeleted time.Time
	var identityBy sql.NullInt64
	if err := f.db.QueryRowContext(f.ctx, `
		SELECT deleted_at, deleted_by_account_id FROM anonymous_participants WHERE id = ?`,
		anonID).Scan(&identityDeleted, &identityBy); err != nil {
		t.Fatalf("identity after delete: %v", err)
	}
	if identityDeleted.Equal(store.LiveAtTime) || !identityBy.Valid || identityBy.Int64 != f.teacherID {
		t.Errorf("identity root = %v / %v", identityDeleted, identityBy)
	}

	checks := []struct {
		name  string
		query string
		id    int64
	}{
		{"registration", "SELECT deleted_at FROM registrations WHERE id = ?", regID},
		{"attempt", "SELECT deleted_at FROM quiz_attempts WHERE id = ?", attemptID},
		{"answer", "SELECT deleted_at FROM attempt_answers WHERE id = ?", answerID},
		{"selection", "SELECT deleted_at FROM attempt_answer_selections WHERE id = ?", selectionID},
		{"focus", "SELECT deleted_at FROM focus_loss_events WHERE id = ?", focusID},
	}
	for _, c := range checks {
		var deleted time.Time
		if err := f.db.QueryRowContext(f.ctx, c.query, c.id).Scan(&deleted); err != nil {
			t.Fatalf("%s after delete: %v", c.name, err)
		}
		if !deleted.Equal(identityDeleted) {
			t.Errorf("%s deleted_at = %v, want the one root stamp %v", c.name, deleted, identityDeleted)
		}
	}

	if _, err := f.svc.DeleteAnonymous(f.ctx, AnonymousDeleteInput{Actor: f.actor(), ID: anonID}); !apperr.IsNotFound(err) {
		t.Errorf("second delete err = %v, want 404", err)
	}
}

func TestCreateAndRenameMajor(t *testing.T) {
	f := newCatalogueFixture(t)
	created, err := f.svc.CreateMajor(f.ctx, MajorCreateInput{
		Actor: f.actor(),
		Name:  "  Teknik   Komputer dan Jaringan ",
	})
	if err != nil {
		t.Fatalf("CreateMajor: %v", err)
	}
	if created.Result.Flash != "saved" || created.Result.Path != "/teacher/majors" {
		t.Errorf("create result = %+v", created.Result)
	}
	var name, code string
	if err := f.db.QueryRowContext(f.ctx,
		"SELECT name, code FROM majors WHERE name = 'Teknik Komputer dan Jaringan'").
		Scan(&name, &code); err != nil {
		t.Fatalf("read created major: %v", err)
	}
	if code != "TEKNIKKOMP" || len(code) > 10 || code != strings.ToUpper(code) {
		t.Errorf("generated code = %q", code)
	}

	dup, err := f.svc.CreateMajor(f.ctx, MajorCreateInput{
		Actor: f.actor(), Name: "teknik komputer dan jaringan",
	})
	if msg := fieldError(t, err, "name"); msg != ErrMajorDup {
		t.Errorf("duplicate name error = %q", msg)
	}
	if dup.Page.Majors == nil {
		t.Errorf("duplicate 422 did not rebuild the majors page")
	}

	invalid, err := f.svc.CreateMajor(f.ctx, MajorCreateInput{Actor: f.actor(), Name: "A"})
	if msg := fieldError(t, err, "name"); msg != ErrMajorShort {
		t.Errorf("short name error = %q", msg)
	}
	if invalid.Page.Majors == nil {
		t.Errorf("invalid 422 did not rebuild the majors page")
	}

	f.seedMajor("Sains", "SAINS")
	collide, err := f.svc.CreateMajor(f.ctx, MajorCreateInput{Actor: f.actor(), Name: "Sains!"})
	if err != nil {
		t.Fatalf("CreateMajor code collision: %v", err)
	}
	if collide.Result.Flash != "saved" {
		t.Errorf("collision create result = %+v", collide.Result)
	}
	var collided string
	if err := f.db.QueryRowContext(f.ctx,
		"SELECT code FROM majors WHERE name = 'Sains!'").Scan(&collided); err != nil {
		t.Fatalf("read collision code: %v", err)
	}
	if collided != "SAINS2" {
		t.Errorf("collision code = %q, want SAINS2", collided)
	}

	var newID int64
	if err := f.db.QueryRowContext(f.ctx,
		"SELECT id FROM majors WHERE name = 'Teknik Komputer dan Jaringan'").Scan(&newID); err != nil {
		t.Fatalf("created major id: %v", err)
	}
	renamed, err := f.svc.RenameMajor(f.ctx, MajorRenameInput{
		Actor: f.actor(), ID: newID, Name: "TKJ Baru",
	})
	if err != nil || renamed.Result.Flash != "saved" {
		t.Fatalf("RenameMajor = %+v, %v", renamed.Result, err)
	}
	var afterCode string
	if err := f.db.QueryRowContext(f.ctx,
		"SELECT name, code FROM majors WHERE id = ?", newID).Scan(&name, &afterCode); err != nil {
		t.Fatalf("read renamed major: %v", err)
	}
	if name != "TKJ Baru" || afterCode != code {
		t.Errorf("rename = %q / code %q (was %q)", name, afterCode, code)
	}

	if _, err := f.svc.RenameMajor(f.ctx, MajorRenameInput{
		Actor: f.actor(), ID: newID, Name: "Rekayasa Perangkat Lunak",
	}); fieldError(t, err, "name") != ErrMajorDup {
		t.Errorf("rename duplicate did not answer the name field")
	}

	if _, err := f.svc.RenameMajor(f.ctx, MajorRenameInput{
		Actor: f.actor(), ID: newID, Name: "TKJ Baru",
	}); err != nil {
		t.Errorf("same-name rename must be a success no-op: %v", err)
	}

	if _, err := f.svc.RenameMajor(f.ctx, MajorRenameInput{
		Actor: f.actor(), ID: newID + 999, Name: "Hilang",
	}); !apperr.IsNotFound(err) {
		t.Errorf("missing rename err = %v, want 404", err)
	}
}

func TestDeleteMajorGuardAndSuccess(t *testing.T) {
	f := newCatalogueFixture(t)
	f.seedAnon("Tamu Terikat", 10, f.majorID, 3, "2026-01-01 00:00:00")

	res, err := f.svc.DeleteMajor(f.ctx, MajorDeleteInput{Actor: f.actor(), ID: f.majorID})
	if err != nil {
		t.Fatalf("DeleteMajor referenced: %v", err)
	}
	if res.Flash != "major-in-use" || res.Path != "/teacher/majors" {
		t.Errorf("referenced delete result = %+v", res)
	}
	var deleted time.Time
	if err := f.db.QueryRowContext(f.ctx,
		"SELECT deleted_at FROM majors WHERE id = ?", f.majorID).Scan(&deleted); err != nil {
		t.Fatalf("read referenced major: %v", err)
	}
	if !deleted.Equal(store.LiveAtTime) {
		t.Errorf("referenced major was deleted at %v", deleted)
	}

	created, err := f.svc.CreateMajor(f.ctx, MajorCreateInput{Actor: f.actor(), Name: "Jurusan Longgar"})
	if err != nil {
		t.Fatalf("CreateMajor: %v", err)
	}
	if created.Result.Flash != "saved" {
		t.Fatalf("CreateMajor result = %+v", created.Result)
	}
	var unusedID int64
	if err := f.db.QueryRowContext(f.ctx,
		"SELECT id FROM majors WHERE name = 'Jurusan Longgar'").Scan(&unusedID); err != nil {
		t.Fatalf("unused major id: %v", err)
	}
	ok, err := f.svc.DeleteMajor(f.ctx, MajorDeleteInput{Actor: f.actor(), ID: unusedID})
	if err != nil || ok.Flash != "deleted" {
		t.Fatalf("DeleteMajor unused = %+v, %v", ok, err)
	}
	var by sql.NullInt64
	if err := f.db.QueryRowContext(f.ctx,
		"SELECT deleted_at, deleted_by_account_id FROM majors WHERE id = ?", unusedID).
		Scan(&deleted, &by); err != nil {
		t.Fatalf("read deleted major: %v", err)
	}
	if deleted.Equal(store.LiveAtTime) || !by.Valid || by.Int64 != f.teacherID {
		t.Errorf("deleted major = %v / %v", deleted, by)
	}
	if _, err := f.svc.DeleteMajor(f.ctx, MajorDeleteInput{Actor: f.actor(), ID: unusedID}); !apperr.IsNotFound(err) {
		t.Errorf("second major delete err = %v, want 404", err)
	}
}

// fieldError asserts a 422 field map and returns one message.
func fieldError(t *testing.T, err error, field string) string {
	t.Helper()
	if err == nil {
		t.Fatalf("expected a validation error for %q", field)
	}
	fields, ok := FieldErrorsOf(err)
	if !ok {
		t.Fatalf("error %v is not field-scoped", err)
	}
	msg, ok := fields[field]
	if !ok {
		t.Fatalf("field %q missing from %v", field, fields)
	}
	return msg
}
