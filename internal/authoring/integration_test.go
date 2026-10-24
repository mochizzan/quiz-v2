package authoring

// Integration tests for the authoring domain. They run only when TEST_DB_DSN is
// set (storetest.DB skips otherwise), because they need the real MariaDB
// schema, CHECK constraints, unique keys, and transaction semantics. Run them
// with:
//
//	TEST_DB_DSN='quiz:quiz@tcp(127.0.0.1:3306)/quiz?parseTime=true&loc=UTC' go test ./internal/authoring/...

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
	"quiz-v2/internal/store/storetest"
	"quiz-v2/internal/viewmodel"
)

// ---- Fixture --------------------------------------------------------------

type authoringFixture struct {
	svc     *Service
	db      *store.DB
	uploads string
	owner   int64
	other   int64
	major   int64
}

func newAuthoringFixture(t *testing.T) *authoringFixture {
	t.Helper()
	db := storetest.DB(t)
	storetest.Reset(t, db)
	uploads := t.TempDir()
	return &authoringFixture{
		svc:     New(db, uploads),
		db:      db,
		uploads: uploads,
		owner:   seedTeacher(t, db, "owner@example.test", "Guru Pemilik"),
		other:   seedTeacher(t, db, "other@example.test", "Guru Lain"),
		major:   seedMajor(t, db, "Rekayasa Perangkat Lunak", "RPL"),
	}
}

func seedTeacher(t *testing.T, db *store.DB, email, name string) int64 {
	t.Helper()
	result, err := db.Exec(`INSERT INTO accounts
		(full_name, email, password_hash, role, status, approved_at)
		VALUES (?, ?, 'not-a-real-hash', 'teacher', 'active', UTC_TIMESTAMP())`, name, email)
	if err != nil {
		t.Fatalf("seed teacher %s: %v", email, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("seed teacher id: %v", err)
	}
	return id
}

func seedMajor(t *testing.T, db *store.DB, name, code string) int64 {
	t.Helper()
	result, err := db.Exec("INSERT INTO majors (name, code) VALUES (?, ?)", name, code)
	if err != nil {
		t.Fatalf("seed major %s: %v", name, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("seed major id: %v", err)
	}
	return id
}

func seedAnonymousParticipant(t *testing.T, db *store.DB, majorID int64) int64 {
	t.Helper()
	var hash [32]byte
	hash[0] = 1
	result, err := db.Exec(`INSERT INTO anonymous_participants
		(display_name, grade_level, major_id, cookie_token_hash)
		VALUES ('Tamu', 10, ?, ?)`, majorID, hash[:])
	if err != nil {
		t.Fatalf("seed participant: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("seed participant id: %v", err)
	}
	return id
}

func seedQuestions(t *testing.T, db *store.DB, quizID int64, n int) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("seed questions begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	for i := 1; i <= n; i++ {
		prompt := fmt.Sprintf("Soal seed nomor %03d yang cukup panjang", i)
		if _, err := tx.Exec(
			"INSERT INTO questions (quiz_id, position, type, prompt) VALUES (?, ?, 'essay', ?)",
			quizID, i, prompt); err != nil {
			t.Fatalf("seed question %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("seed questions commit: %v", err)
	}
}

func (f *authoringFixture) createQuiz(t *testing.T, in QuizInput) int64 {
	t.Helper()
	id, flash, err := f.svc.CreateQuiz(context.Background(), f.owner, in)
	if err != nil {
		t.Fatalf("CreateQuiz: %v", err)
	}
	if flash != FlashQuizCreated {
		t.Fatalf("CreateQuiz flash = %q, want %q", flash, FlashQuizCreated)
	}
	if id <= 0 {
		t.Fatalf("CreateQuiz id = %d", id)
	}
	return id
}

func (f *authoringFixture) createQuestion(t *testing.T, quizID int64, in QuestionInput) int64 {
	t.Helper()
	flash, err := f.svc.CreateQuestion(context.Background(), f.owner, quizID, in)
	if err != nil {
		t.Fatalf("CreateQuestion: %v", err)
	}
	if flash != FlashQuestionSaved {
		t.Fatalf("CreateQuestion flash = %q, want %q", flash, FlashQuestionSaved)
	}
	return questionIDByPrompt(t, f.db, quizID, NormalizeText(in.Body))
}

func singleChoice(prompt string, options []string, correct int) QuestionInput {
	return QuestionInput{
		Type:         string(viewmodel.QuestionSingle),
		Body:         prompt,
		Options:      options,
		CorrectIndex: []string{strconv.Itoa(correct)},
	}
}

func multipleChoice(prompt string, options []string, correct ...int) QuestionInput {
	indexes := make([]string, len(correct))
	for i, n := range correct {
		indexes[i] = strconv.Itoa(n)
	}
	return QuestionInput{
		Type:           string(viewmodel.QuestionMultiple),
		Body:           prompt,
		Options:        options,
		CorrectIndexes: indexes,
	}
}

func essay(prompt string) QuestionInput {
	return QuestionInput{Type: string(viewmodel.QuestionEssay), Body: prompt}
}

func pngBytes(t *testing.T, r, g, b uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: r, G: g, B: b, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// ---- Raw row readers ------------------------------------------------------

type rawQuiz struct {
	Title              string
	Description        string
	Visibility         string
	State              string
	Code               sql.NullString
	InviteToken        string
	TargetGrade        sql.NullInt64
	TargetMajorID      sql.NullInt64
	StartsAt           sql.NullTime
	EndsAt             sql.NullTime
	AllowRetake        bool
	MaxAttempts        int
	ShowScore          bool
	ReviewMode         string
	RandomizeQuestions bool
	RandomizeOptions   bool
	PublishedAt        sql.NullTime
	ClosedAt           sql.NullTime
	UpdatedBy          sql.NullInt64
	DeletedAt          time.Time
	DeletedBy          sql.NullInt64
}

func readRawQuiz(t *testing.T, db *store.DB, id int64) rawQuiz {
	t.Helper()
	var q rawQuiz
	err := db.QueryRow(`SELECT title, description, visibility, state, code, invite_token,
		target_grade, target_major_id, starts_at, ends_at, allow_retake, max_attempts,
		show_score, review_mode, randomize_questions, randomize_options,
		published_at, closed_at, updated_by_account_id, deleted_at, deleted_by_account_id
		FROM quizzes WHERE id = ?`, id).Scan(
		&q.Title, &q.Description, &q.Visibility, &q.State, &q.Code, &q.InviteToken,
		&q.TargetGrade, &q.TargetMajorID, &q.StartsAt, &q.EndsAt, &q.AllowRetake,
		&q.MaxAttempts, &q.ShowScore, &q.ReviewMode, &q.RandomizeQuestions,
		&q.RandomizeOptions, &q.PublishedAt, &q.ClosedAt, &q.UpdatedBy, &q.DeletedAt,
		&q.DeletedBy)
	if err != nil {
		t.Fatalf("read quiz %d: %v", id, err)
	}
	return q
}

type rawQuestion struct {
	Position   int
	Kind       string
	Prompt     string
	ImagePath  sql.NullString
	ImageMime  sql.NullString
	ImageBytes sql.NullInt64
	ImageSHA   []byte
	ImageAlt   sql.NullString
	UpdatedBy  sql.NullInt64
	DeletedAt  time.Time
	DeletedBy  sql.NullInt64
}

func readRawQuestion(t *testing.T, db *store.DB, id int64) rawQuestion {
	t.Helper()
	var q rawQuestion
	err := db.QueryRow(`SELECT position, type, prompt, image_path, image_mime,
		image_bytes, image_sha256, image_alt, updated_by_account_id, deleted_at,
		deleted_by_account_id
		FROM questions WHERE id = ?`, id).Scan(
		&q.Position, &q.Kind, &q.Prompt, &q.ImagePath, &q.ImageMime,
		&q.ImageBytes, &q.ImageSHA, &q.ImageAlt, &q.UpdatedBy, &q.DeletedAt,
		&q.DeletedBy)
	if err != nil {
		t.Fatalf("read question %d: %v", id, err)
	}
	return q
}

type rawOption struct {
	ID        int64
	Position  int
	Body      string
	Correct   bool
	DeletedAt time.Time
}

func readRawOptions(t *testing.T, db *store.DB, questionID int64) []rawOption {
	t.Helper()
	rows, err := db.Query(`SELECT id, position, body, is_correct, deleted_at
		FROM answer_options WHERE question_id = ? ORDER BY id`, questionID)
	if err != nil {
		t.Fatalf("read options for %d: %v", questionID, err)
	}
	defer rows.Close()

	var out []rawOption
	for rows.Next() {
		var o rawOption
		if err := rows.Scan(&o.ID, &o.Position, &o.Body, &o.Correct, &o.DeletedAt); err != nil {
			t.Fatalf("scan option: %v", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("options rows: %v", err)
	}
	return out
}

func questionIDByPrompt(t *testing.T, db *store.DB, quizID int64, prompt string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`SELECT id FROM questions
		WHERE quiz_id = ? AND prompt = ? AND deleted_at = ?
		ORDER BY id DESC LIMIT 1`, quizID, prompt, store.LiveAt).Scan(&id)
	if err != nil {
		t.Fatalf("question id for %q: %v", prompt, err)
	}
	return id
}

func liveQuestionIDs(t *testing.T, db *store.DB, quizID int64) []int64 {
	t.Helper()
	rows, err := db.Query(`SELECT id FROM questions
		WHERE quiz_id = ? AND deleted_at = ? ORDER BY position, id`, quizID, store.LiveAt)
	if err != nil {
		t.Fatalf("live questions: %v", err)
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan live question: %v", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("live question rows: %v", err)
	}
	return out
}

func liveOptionIDs(t *testing.T, db *store.DB, questionID int64) []int64 {
	t.Helper()
	rows, err := db.Query(`SELECT id FROM answer_options
		WHERE question_id = ? AND deleted_at = ? ORDER BY position, id`, questionID, store.LiveAt)
	if err != nil {
		t.Fatalf("live options: %v", err)
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan live option: %v", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("live option rows: %v", err)
	}
	return out
}

func liveQuestionCount(t *testing.T, db *store.DB, quizID int64) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM questions
		WHERE quiz_id = ? AND deleted_at = ?`, quizID, store.LiveAt).Scan(&n); err != nil {
		t.Fatalf("count questions: %v", err)
	}
	return n
}

// ---- Assertions -----------------------------------------------------------

func requireFlash(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("flash = %q, want %q", got, want)
	}
}

func requireConflict(t *testing.T, err error, want string) {
	t.Helper()
	key, ok := ConflictKey(err)
	if !ok || key != want {
		t.Fatalf("err = %v, want conflict %q", err, want)
	}
}

func requireNotFound(t *testing.T, err error) {
	t.Helper()
	if !apperr.IsNotFound(err) {
		t.Fatalf("err = %v, want 404", err)
	}
}

func requireInternal(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Kind() != apperr.Internal {
		t.Fatalf("err = %v, want 500", err)
	}
}

func requireValidation(t *testing.T, err error) *apperr.Error {
	t.Helper()
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Kind() != apperr.Validation {
		t.Fatalf("err = %v, want 422", err)
	}
	return appErr
}

func requireLiveOrder(t *testing.T, db *store.DB, quizID int64, want ...int64) {
	t.Helper()
	got := liveQuestionIDs(t, db, quizID)
	if !slices.Equal(got, want) {
		t.Fatalf("live order = %v, want %v", got, want)
	}
}

func requirePositions(t *testing.T, db *store.DB, quizID int64, want map[int64]int) {
	t.Helper()
	rows, err := db.Query(`SELECT id, position FROM questions
		WHERE quiz_id = ? AND deleted_at = ?`, quizID, store.LiveAt)
	if err != nil {
		t.Fatalf("positions: %v", err)
	}
	defer rows.Close()

	got := map[int64]int{}
	for rows.Next() {
		var (
			id  int64
			pos int
		)
		if err := rows.Scan(&id, &pos); err != nil {
			t.Fatalf("scan position: %v", err)
		}
		got[id] = pos
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("position rows: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("live positions = %v, want %v", got, want)
	}
	for id, pos := range want {
		if got[id] != pos {
			t.Fatalf("question %d position = %d, want %d (all: %v)", id, got[id], pos, got)
		}
	}
}

func countUploadFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0
		}
		t.Fatalf("read uploads %s: %v", dir, err)
	}
	n := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			n++
		}
	}
	return n
}

func requireFile(t *testing.T, path string, wantSize int) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Size() != int64(wantSize) {
		t.Fatalf("size of %s = %d, want %d", path, info.Size(), wantSize)
	}
}

func requireNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("file %s still exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat %s: %v", path, err)
	}
}

// ---- Quiz shell -----------------------------------------------------------

func TestIntegrationCreateListDetail(t *testing.T) {
	f := newAuthoringFixture(t)
	ctx := context.Background()

	publicID, flash, err := f.svc.CreateQuiz(ctx, f.owner, QuizInput{
		Title:       "  Kuis Aljabar  ",
		Description: "  Bab 1  ",
		Visibility:  string(viewmodel.VisibilityPublic),
	})
	if err != nil {
		t.Fatalf("CreateQuiz(public): %v", err)
	}
	requireFlash(t, flash, FlashQuizCreated)

	raw := readRawQuiz(t, f.db, publicID)
	if raw.Title != "Kuis Aljabar" || raw.Description != "Bab 1" {
		t.Fatalf("text not normalised: %+v", raw)
	}
	if raw.Visibility != "public" || raw.State != "draft" {
		t.Fatalf("visibility/state = %q/%q", raw.Visibility, raw.State)
	}
	if !raw.Code.Valid || !sixDigits(raw.Code.String) {
		t.Fatalf("public code = %+v", raw.Code)
	}
	if len(raw.InviteToken) != 22 {
		t.Fatalf("invite token length = %d, want 22", len(raw.InviteToken))
	}
	if raw.TargetGrade.Valid || raw.TargetMajorID.Valid {
		t.Fatalf("public quiz has targets: %+v/%+v", raw.TargetGrade, raw.TargetMajorID)
	}
	if raw.StartsAt.Valid || raw.EndsAt.Valid {
		t.Fatalf("new quiz has a window: %+v/%+v", raw.StartsAt, raw.EndsAt)
	}
	if raw.AllowRetake || raw.MaxAttempts != 1 || !raw.ShowScore {
		t.Fatalf("settings defaults = %+v", raw)
	}
	if raw.ReviewMode != "questions_only" || raw.RandomizeQuestions || raw.RandomizeOptions {
		t.Fatalf("review/random defaults = %+v", raw)
	}
	if raw.PublishedAt.Valid || raw.ClosedAt.Valid {
		t.Fatalf("new quiz has lifecycle stamps: %+v/%+v", raw.PublishedAt, raw.ClosedAt)
	}
	if !raw.UpdatedBy.Valid || raw.UpdatedBy.Int64 != f.owner {
		t.Fatalf("updated_by = %+v, want %d", raw.UpdatedBy, f.owner)
	}

	privateID, flash, err := f.svc.CreateQuiz(ctx, f.owner, QuizInput{
		Title:         "Kuis Privat",
		Visibility:    string(viewmodel.VisibilityPrivate),
		TargetGrade:   "11",
		TargetMajorID: strconv.FormatInt(f.major, 10),
	})
	if err != nil {
		t.Fatalf("CreateQuiz(private): %v", err)
	}
	requireFlash(t, flash, FlashQuizCreated)

	privateRaw := readRawQuiz(t, f.db, privateID)
	if privateRaw.Code.Valid {
		t.Fatalf("private quiz has a public code: %+v", privateRaw.Code)
	}
	if !privateRaw.TargetGrade.Valid || privateRaw.TargetGrade.Int64 != 11 {
		t.Fatalf("target grade = %+v", privateRaw.TargetGrade)
	}
	if !privateRaw.TargetMajorID.Valid || privateRaw.TargetMajorID.Int64 != f.major {
		t.Fatalf("target major = %+v", privateRaw.TargetMajorID)
	}

	// A private quiz needs a live target major.
	_, _, err = f.svc.CreateQuiz(ctx, f.owner, QuizInput{
		Title:         "Kuis Privat Tanpa Jurusan",
		Visibility:    string(viewmodel.VisibilityPrivate),
		TargetGrade:   "10",
		TargetMajorID: "999999",
	})
	appErr := requireValidation(t, err)
	if appErr.Fields()["target_major_id"] != MsgMajorUnavailable {
		t.Fatalf("target_major_id message = %q", appErr.Fields()["target_major_id"])
	}

	_, _, err = f.svc.CreateQuiz(ctx, f.owner, QuizInput{
		Title:      "Kuis Privat Tanpa Sasaran",
		Visibility: string(viewmodel.VisibilityPrivate),
	})
	appErr = requireValidation(t, err)
	if appErr.Fields()["target_grade"] != MsgGradeRequired ||
		appErr.Fields()["target_major_id"] != MsgMajorRequired {
		t.Fatalf("missing target messages = %v", appErr.Fields())
	}

	// The list is owner-scoped, counts by state, and searches literally.
	list, err := f.svc.List(ctx, f.owner, ListInput{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.Quizzes.Total != 2 {
		t.Fatalf("list total = %d, want 2", list.Quizzes.Total)
	}
	if len(list.Tabs) != 4 || list.Tabs[0].Count != 2 || list.Tabs[1].Count != 2 ||
		list.Tabs[2].Count != 0 || list.Tabs[3].Count != 0 {
		t.Fatalf("tab counts = %+v", list.Tabs)
	}
	seen := map[int64]bool{}
	for _, card := range list.Quizzes.Items {
		seen[card.ID] = true
	}
	if !seen[publicID] || !seen[privateID] {
		t.Fatalf("list items = %+v", list.Quizzes.Items)
	}

	search, err := f.svc.List(ctx, f.owner, ListInput{Query: "ALJABAR"})
	if err != nil {
		t.Fatalf("List(search): %v", err)
	}
	if search.Quizzes.Total != 1 || search.Quizzes.Items[0].ID != publicID {
		t.Fatalf("search result = %+v", search.Quizzes)
	}
	literal, err := f.svc.List(ctx, f.owner, ListInput{Query: "%"})
	if err != nil {
		t.Fatalf("List(%%): %v", err)
	}
	if literal.Quizzes.Total != 0 {
		t.Fatalf("search %% matched %d quizzes", literal.Quizzes.Total)
	}

	other, err := f.svc.List(ctx, f.other, ListInput{})
	if err != nil {
		t.Fatalf("List(other): %v", err)
	}
	if other.Quizzes.Total != 0 {
		t.Fatalf("other teacher sees %d quizzes", other.Quizzes.Total)
	}

	detail, err := f.svc.Detail(ctx, f.owner, publicID, "Guru Pemilik")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if detail.Quiz.ID != publicID || detail.Quiz.Title != "Kuis Aljabar" {
		t.Fatalf("detail quiz = %+v", detail.Quiz)
	}
	if detail.State != viewmodel.QuizDraft || !detail.Editable {
		t.Fatalf("detail state/editable = %v/%v", detail.State, detail.Editable)
	}
	if detail.CanPublish || detail.PublishBlockedReason != MsgNoQuestions {
		t.Fatalf("draft publish gate = %v/%q", detail.CanPublish, detail.PublishBlockedReason)
	}
	if detail.Code == nil || *detail.Code != raw.Code.String {
		t.Fatalf("detail code = %v, want %q", detail.Code, raw.Code.String)
	}
	if detail.InviteURL != "/invite/"+raw.InviteToken {
		t.Fatalf("invite URL = %q", detail.InviteURL)
	}
	if detail.QuestionCount != 0 || len(detail.Questions) != 0 {
		t.Fatalf("detail questions = %d", detail.QuestionCount)
	}
	if detail.CanClose || detail.CloseAction != nil {
		t.Fatalf("draft has a close action")
	}
	if detail.PublishAction == nil || detail.DeleteAction == nil {
		t.Fatalf("draft is missing its actions: %+v/%+v", detail.PublishAction, detail.DeleteAction)
	}
	if detail.Settings.MaxAttempts != 1 || !detail.Settings.ShowScore {
		t.Fatalf("detail settings = %+v", detail.Settings)
	}
	if detail.TeacherName != "Guru Pemilik" {
		t.Fatalf("teacher name = %q", detail.TeacherName)
	}

	privateDetail, err := f.svc.Detail(ctx, f.owner, privateID, "Guru Pemilik")
	if err != nil {
		t.Fatalf("Detail(private): %v", err)
	}
	if privateDetail.Code != nil || privateDetail.Audience == "" {
		t.Fatalf("private detail code/audience = %v/%q", privateDetail.Code, privateDetail.Audience)
	}

	_, err = f.svc.Detail(ctx, f.other, publicID, "Guru Lain")
	requireNotFound(t, err)

	// The detail gate resolves major liveness in its own snapshot (AUT-025,
	// AUT-027). A private draft with a live target must be publishable; the
	// same draft must report the missing target once the major is deleted.
	gateMajor := seedMajor(t, f.db, "Teknik Komputer dan Jaringan", "TKJ")
	gateID := f.createQuiz(t, QuizInput{
		Title:         "Kuis Privat Siap Dibuka",
		Visibility:    string(viewmodel.VisibilityPrivate),
		TargetGrade:   "10",
		TargetMajorID: strconv.FormatInt(gateMajor, 10),
	})
	seedQuestions(t, f.db, gateID, 1)

	gateDetail, err := f.svc.Detail(ctx, f.owner, gateID, "Guru Pemilik")
	if err != nil {
		t.Fatalf("Detail(gate): %v", err)
	}
	if !gateDetail.CanPublish || gateDetail.PublishBlockedReason != "" {
		t.Fatalf("private draft with live major = %v/%q",
			gateDetail.CanPublish, gateDetail.PublishBlockedReason)
	}
	if _, err := f.db.Exec("UPDATE majors SET deleted_at = UTC_TIMESTAMP() WHERE id = ?", gateMajor); err != nil {
		t.Fatalf("delete major: %v", err)
	}
	blockedDetail, err := f.svc.Detail(ctx, f.owner, gateID, "Guru Pemilik")
	if err != nil {
		t.Fatalf("Detail(blocked): %v", err)
	}
	if blockedDetail.CanPublish || blockedDetail.PublishBlockedReason != MsgTargetMissing {
		t.Fatalf("private draft with deleted major = %v/%q",
			blockedDetail.CanPublish, blockedDetail.PublishBlockedReason)
	}

	form, err := f.svc.NewQuizForm(ctx)
	if err != nil {
		t.Fatalf("NewQuizForm: %v", err)
	}
	if form.Visibility != viewmodel.VisibilityPublic {
		t.Fatalf("new form visibility = %q", form.Visibility)
	}
	if len(form.Majors) != 1 || form.Majors[0].ID != f.major {
		t.Fatalf("new form majors = %+v", form.Majors)
	}
	if form.Action != "/teacher/quizzes" || form.CancelHref != "/teacher/quizzes" {
		t.Fatalf("new form action = %q/%q", form.Action, form.CancelHref)
	}
}

func TestIntegrationSaveQuizShell(t *testing.T) {
	f := newAuthoringFixture(t)
	ctx := context.Background()
	quizID := f.createQuiz(t, QuizInput{Title: "Kuis Awal", Visibility: "public"})

	meta := QuizInput{
		Title:       "  Kuis Aljabar  ",
		Description: "  Bab 1 dan 2  ",
		StartAt:     "2026-10-24T08:00",
		EndAt:       "2026-10-24T10:30:45",
	}
	flash, err := f.svc.SaveMeta(ctx, f.owner, quizID, meta)
	if err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	requireFlash(t, flash, FlashSaved)

	raw := readRawQuiz(t, f.db, quizID)
	if raw.Title != "Kuis Aljabar" || raw.Description != "Bab 1 dan 2" {
		t.Fatalf("metadata = %+v", raw)
	}
	wantStart := time.Date(2026, 10, 24, 1, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 10, 24, 3, 30, 0, 0, time.UTC)
	if !raw.StartsAt.Valid || !raw.StartsAt.Time.Equal(wantStart) {
		t.Fatalf("starts_at = %+v, want %v", raw.StartsAt, wantStart)
	}
	if !raw.EndsAt.Valid || !raw.EndsAt.Time.Equal(wantEnd) {
		t.Fatalf("ends_at = %+v, want %v", raw.EndsAt, wantEnd)
	}
	if !raw.UpdatedBy.Valid || raw.UpdatedBy.Int64 != f.owner {
		t.Fatalf("updated_by = %+v", raw.UpdatedBy)
	}

	// Repeating the same values is a successful no-op.
	flash, err = f.svc.SaveMeta(ctx, f.owner, quizID, meta)
	if err != nil {
		t.Fatalf("SaveMeta(repeat): %v", err)
	}
	requireFlash(t, flash, FlashSaved)

	_, err = f.svc.SaveMeta(ctx, f.owner, quizID, QuizInput{Title: "   "})
	appErr := requireValidation(t, err)
	if appErr.Fields()["title"] != MsgTitleRequired {
		t.Fatalf("title message = %q", appErr.Fields()["title"])
	}

	_, err = f.svc.SaveMeta(ctx, f.owner, quizID, QuizInput{
		Title:   "Kuis",
		StartAt: "2026-10-24T10:00",
		EndAt:   "2026-10-24T09:00",
	})
	appErr = requireValidation(t, err)
	if appErr.Fields()["end_at"] != MsgEndNotAfterStart {
		t.Fatalf("end_at message = %q", appErr.Fields()["end_at"])
	}

	_, err = f.svc.SaveMeta(ctx, f.owner, quizID, QuizInput{
		Title:   "Kuis",
		StartAt: "2026-10-24T08:00Z",
	})
	appErr = requireValidation(t, err)
	if appErr.Fields()["start_at"] != MsgStartInvalid {
		t.Fatalf("start_at message = %q", appErr.Fields()["start_at"])
	}

	// The metadata branch never reads or writes the audience.
	beforeAudience := readRawQuiz(t, f.db, quizID)
	flash, err = f.svc.SaveMeta(ctx, f.owner, quizID, QuizInput{
		Title:         "Kuis Aljabar",
		Description:   "Bab 1 dan 2",
		StartAt:       "2026-10-24T08:00",
		EndAt:         "2026-10-24T10:30",
		Visibility:    "private",
		TargetGrade:   "10",
		TargetMajorID: strconv.FormatInt(f.major, 10),
	})
	if err != nil {
		t.Fatalf("SaveMeta(audience): %v", err)
	}
	requireFlash(t, flash, FlashSaved)
	afterAudience := readRawQuiz(t, f.db, quizID)
	if afterAudience.Visibility != beforeAudience.Visibility ||
		afterAudience.Code.String != beforeAudience.Code.String ||
		afterAudience.TargetGrade.Valid || afterAudience.TargetMajorID.Valid {
		t.Fatalf("metadata changed the audience: %+v", afterAudience)
	}

	// Settings: a full valid branch.
	settings := url.Values{
		"randomize_questions": {"1"},
		"randomize_answers":   {"1"},
		"allow_retake":        {"1"},
		"max_attempts":        {"4"},
		"review_mode":         {"key_only"},
		"show_score":          {"1"},
	}
	flash, err = f.svc.SaveSettings(ctx, f.owner, quizID, settings)
	if err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	requireFlash(t, flash, FlashSaved)

	raw = readRawQuiz(t, f.db, quizID)
	if !raw.RandomizeQuestions || !raw.RandomizeOptions || !raw.AllowRetake ||
		raw.MaxAttempts != 4 || raw.ReviewMode != "key_only" || !raw.ShowScore {
		t.Fatalf("settings row = %+v", raw)
	}

	// allow_retake=false forces max_attempts=1 and ignores the number.
	flash, err = f.svc.SaveSettings(ctx, f.owner, quizID, url.Values{
		"max_attempts": {"9"},
		"review_mode":  {"questions_only"},
	})
	if err != nil {
		t.Fatalf("SaveSettings(retake off): %v", err)
	}
	requireFlash(t, flash, FlashSaved)
	raw = readRawQuiz(t, f.db, quizID)
	if raw.AllowRetake || raw.MaxAttempts != 1 || raw.ShowScore ||
		raw.RandomizeQuestions || raw.RandomizeOptions || raw.ReviewMode != "questions_only" {
		t.Fatalf("settings after retake off = %+v", raw)
	}

	_, err = f.svc.SaveSettings(ctx, f.owner, quizID, url.Values{
		"randomize_questions": {"yes"},
		"review_mode":         {"key_only"},
	})
	appErr = requireValidation(t, err)
	if appErr.Fields()["randomize_questions"] != MsgSettingsInvalid {
		t.Fatalf("checkbox message = %q", appErr.Fields()["randomize_questions"])
	}

	_, err = f.svc.SaveSettings(ctx, f.owner, quizID, url.Values{
		"allow_retake": {"1"},
		"max_attempts": {"11"},
		"review_mode":  {"key_only"},
	})
	appErr = requireValidation(t, err)
	if appErr.Fields()["max_attempts"] != MsgAttemptsRange {
		t.Fatalf("attempts message = %q", appErr.Fields()["max_attempts"])
	}

	_, err = f.svc.SaveSettings(ctx, f.owner, quizID, url.Values{
		"review_mode": {"everything"},
	})
	appErr = requireValidation(t, err)
	if appErr.Fields()["review_mode"] != MsgReviewModeInvalid {
		t.Fatalf("review message = %q", appErr.Fields()["review_mode"])
	}
}

// ---- Lifecycle ------------------------------------------------------------

func TestIntegrationQuizLifecycle(t *testing.T) {
	f := newAuthoringFixture(t)
	ctx := context.Background()

	quizID := f.createQuiz(t, QuizInput{Title: "Kuis Kehidupan", Visibility: "public"})

	// A draft with no questions cannot be opened.
	_, err := f.svc.Publish(ctx, f.owner, quizID)
	requireConflict(t, err, FlashErrNoQuestions)
	if raw := readRawQuiz(t, f.db, quizID); raw.State != "draft" || raw.PublishedAt.Valid {
		t.Fatalf("failed publish mutated the quiz: %+v", raw)
	}

	// Close is open -> closed only.
	_, err = f.svc.Close(ctx, f.owner, quizID)
	requireConflict(t, err, FlashErrNotOpen)

	qid := f.createQuestion(t, quizID,
		singleChoice("Berapakah hasil dari 2 + 2?", []string{"3", "4", "5"}, 1))

	detail, err := f.svc.Detail(ctx, f.owner, quizID, "Guru Pemilik")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if !detail.CanPublish || detail.PublishBlockedReason != "" || detail.PublishAction == nil {
		t.Fatalf("publish gate = %v/%q/%+v", detail.CanPublish, detail.PublishBlockedReason, detail.PublishAction)
	}

	flash, err := f.svc.Publish(ctx, f.owner, quizID)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	requireFlash(t, flash, FlashPublished)

	raw := readRawQuiz(t, f.db, quizID)
	if raw.State != "open" || !raw.PublishedAt.Valid || raw.ClosedAt.Valid {
		t.Fatalf("open row = %+v", raw)
	}

	// Publishing an open quiz is an informational repeat.
	_, err = f.svc.Publish(ctx, f.owner, quizID)
	requireConflict(t, err, FlashAlreadyOpen)

	// Every content write and the two content GETs refuse a frozen quiz.
	_, err = f.svc.SaveMeta(ctx, f.owner, quizID, QuizInput{Title: "Judul Baru"})
	requireConflict(t, err, FlashErrNotDraft)
	_, err = f.svc.SaveSettings(ctx, f.owner, quizID, url.Values{"review_mode": {"key_only"}})
	requireConflict(t, err, FlashErrNotDraft)
	_, err = f.svc.CreateQuestion(ctx, f.owner, quizID,
		essay("Soal yang tidak boleh masuk saat kuis dibuka"))
	requireConflict(t, err, FlashErrNotDraft)
	_, err = f.svc.UpdateQuestion(ctx, f.owner, quizID, qid,
		singleChoice("Soal yang tidak boleh berubah saat kuis dibuka", []string{"A", "B"}, 0))
	requireConflict(t, err, FlashErrNotDraft)
	_, err = f.svc.DeleteQuestion(ctx, f.owner, quizID, qid)
	requireConflict(t, err, FlashErrNotDraft)
	_, err = f.svc.ReorderQuestions(ctx, f.owner, quizID, strconv.FormatInt(qid, 10))
	requireConflict(t, err, FlashErrNotDraft)
	_, err = f.svc.NewQuestionForm(ctx, f.owner, quizID)
	requireConflict(t, err, FlashErrNotDraft)
	_, err = f.svc.EditQuestionForm(ctx, f.owner, quizID, qid)
	requireConflict(t, err, FlashErrNotDraft)
	_, err = f.svc.DeleteQuiz(ctx, f.owner, quizID)
	requireConflict(t, err, FlashErrDeleteNotDraft)

	// Close is terminal.
	flash, err = f.svc.Close(ctx, f.owner, quizID)
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	requireFlash(t, flash, FlashClosed)
	raw = readRawQuiz(t, f.db, quizID)
	if raw.State != "closed" || !raw.ClosedAt.Valid || !raw.PublishedAt.Valid {
		t.Fatalf("closed row = %+v", raw)
	}

	_, err = f.svc.Publish(ctx, f.owner, quizID)
	requireConflict(t, err, FlashErrClosedTerminal)
	_, err = f.svc.Close(ctx, f.owner, quizID)
	requireConflict(t, err, FlashClosed)
	_, err = f.svc.DeleteQuiz(ctx, f.owner, quizID)
	requireConflict(t, err, FlashErrDeleteNotDraft)

	// Draft delete stamps the whole aggregate and unlinks images after commit.
	draftID := f.createQuiz(t, QuizInput{Title: "Draf untuk dihapus", Visibility: "public"})
	image := pngBytes(t, 0, 128, 0)
	withImage := singleChoice("Soal yang akan ikut terhapus", []string{"A", "B"}, 0)
	withImage.Images = []Upload{{Filename: "a.png", Data: image}}
	withImage.ImageAlt = "Gambar soal"
	withImage.AltPresent = true
	questionID := f.createQuestion(t, draftID, withImage)

	question := readRawQuestion(t, f.db, questionID)
	imagePath := filepath.Join(f.uploads, question.ImagePath.String)
	requireFile(t, imagePath, len(image))

	flash, err = f.svc.DeleteQuiz(ctx, f.owner, draftID)
	if err != nil {
		t.Fatalf("DeleteQuiz: %v", err)
	}
	requireFlash(t, flash, FlashQuizDeleted)

	deletedQuiz := readRawQuiz(t, f.db, draftID)
	if deletedQuiz.DeletedAt.Equal(store.LiveAtTime) {
		t.Fatalf("quiz was not stamped: %+v", deletedQuiz)
	}
	if !deletedQuiz.DeletedBy.Valid || deletedQuiz.DeletedBy.Int64 != f.owner {
		t.Fatalf("quiz deleted_by = %+v", deletedQuiz.DeletedBy)
	}
	deletedQuestion := readRawQuestion(t, f.db, questionID)
	if !deletedQuestion.DeletedAt.Equal(deletedQuiz.DeletedAt) {
		t.Fatalf("question stamp %v != quiz stamp %v", deletedQuestion.DeletedAt, deletedQuiz.DeletedAt)
	}
	if deletedQuestion.DeletedBy.Valid {
		t.Fatalf("question deleted_by = %+v, want NULL", deletedQuestion.DeletedBy)
	}
	for _, opt := range readRawOptions(t, f.db, questionID) {
		if !opt.DeletedAt.Equal(deletedQuiz.DeletedAt) {
			t.Fatalf("option stamp %v != quiz stamp %v", opt.DeletedAt, deletedQuiz.DeletedAt)
		}
	}
	requireNoFile(t, imagePath)

	_, err = f.svc.Detail(ctx, f.owner, draftID, "Guru Pemilik")
	requireNotFound(t, err)
	_, err = f.svc.DeleteQuiz(ctx, f.owner, draftID)
	requireNotFound(t, err)

	list, err := f.svc.List(ctx, f.owner, ListInput{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.Quizzes.Total != 1 {
		t.Fatalf("live quiz total = %d, want 1", list.Quizzes.Total)
	}
}

// ---- Questions ------------------------------------------------------------

func TestIntegrationQuestionCRUD(t *testing.T) {
	f := newAuthoringFixture(t)
	ctx := context.Background()
	quizID := f.createQuiz(t, QuizInput{Title: "Kuis Soal", Visibility: "public"})

	form, err := f.svc.NewQuestionForm(ctx, f.owner, quizID)
	if err != nil {
		t.Fatalf("NewQuestionForm: %v", err)
	}
	if form.Position != 1 || form.Type != viewmodel.QuestionSingle || !form.IsNew {
		t.Fatalf("new form = %+v", form)
	}
	if len(form.Options) != 2 || form.Options[0].Letter != "A" || form.Options[1].Letter != "B" ||
		form.Options[0].Correct || form.Options[1].Correct {
		t.Fatalf("new form options = %+v", form.Options)
	}
	if form.Action != fmt.Sprintf("/teacher/quizzes/%d/questions", quizID) {
		t.Fatalf("new form action = %q", form.Action)
	}
	if form.EssayNote != QuestionEssayNote {
		t.Fatalf("essay note = %q", form.EssayNote)
	}

	firstPNG := pngBytes(t, 0, 100, 200)
	create := QuestionInput{
		Type:         string(viewmodel.QuestionSingle),
		Body:         "  Berapakah hasil dari 2 + 2?  ",
		ImageAlt:     "  Diagram dua tambah dua  ",
		AltPresent:   true,
		Options:      []string{" 3 ", " 4 ", " 5 ", "   "},
		CorrectIndex: []string{"1"},
		Images:       []Upload{{Filename: "../../evil.php", ContentType: "text/html", Data: firstPNG}},
	}
	flash, err := f.svc.CreateQuestion(ctx, f.owner, quizID, create)
	if err != nil {
		t.Fatalf("CreateQuestion: %v", err)
	}
	requireFlash(t, flash, FlashQuestionSaved)

	qid := questionIDByPrompt(t, f.db, quizID, "Berapakah hasil dari 2 + 2?")
	question := readRawQuestion(t, f.db, qid)
	if question.Position != 1 || question.Kind != "single" ||
		question.Prompt != "Berapakah hasil dari 2 + 2?" {
		t.Fatalf("question row = %+v", question)
	}
	if !safeImageName.MatchString(question.ImagePath.String) {
		t.Fatalf("image path = %q", question.ImagePath.String)
	}
	if question.ImageMime.String != "image/png" ||
		question.ImageBytes.Int64 != int64(len(firstPNG)) {
		t.Fatalf("image metadata = %+v", question)
	}
	wantSHA := sha256.Sum256(firstPNG)
	if !bytes.Equal(question.ImageSHA, wantSHA[:]) {
		t.Fatalf("image sha mismatch")
	}
	if question.ImageAlt.String != "Diagram dua tambah dua" {
		t.Fatalf("image alt = %q", question.ImageAlt.String)
	}
	firstPath := filepath.Join(f.uploads, question.ImagePath.String)
	requireFile(t, firstPath, len(firstPNG))

	options := readRawOptions(t, f.db, qid)
	if len(options) != 3 {
		t.Fatalf("options = %+v", options)
	}
	for i, opt := range options {
		if opt.Position != i+1 || !opt.DeletedAt.Equal(store.LiveAtTime) {
			t.Fatalf("option %d = %+v", i, opt)
		}
		if opt.Body != []string{"3", "4", "5"}[i] {
			t.Fatalf("option %d body = %q", i, opt.Body)
		}
		if opt.Correct != (i == 1) {
			t.Fatalf("option %d correct = %v", i, opt.Correct)
		}
	}

	edit, err := f.svc.EditQuestionForm(ctx, f.owner, quizID, qid)
	if err != nil {
		t.Fatalf("EditQuestionForm: %v", err)
	}
	if edit.ID != qid || edit.Position != 1 || edit.Type != viewmodel.QuestionSingle ||
		edit.Body != "Berapakah hasil dari 2 + 2?" {
		t.Fatalf("edit form = %+v", edit)
	}
	if edit.Image == nil || edit.Image.URL != "/uploads/"+question.ImagePath.String ||
		edit.Image.Alt != "Diagram dua tambah dua" || edit.Image.SizeLabel == "" {
		t.Fatalf("edit image = %+v", edit.Image)
	}
	if len(edit.Options) != 3 || edit.Options[0].Letter != "A" ||
		edit.Options[1].Letter != "B" || edit.Options[2].Letter != "C" {
		t.Fatalf("edit options = %+v", edit.Options)
	}
	if edit.Options[0].Correct || !edit.Options[1].Correct || edit.Options[2].Correct {
		t.Fatalf("edit keys = %+v", edit.Options)
	}
	if edit.Action != fmt.Sprintf("/teacher/quizzes/%d/questions/%d", quizID, qid) ||
		edit.DeleteHref != edit.Action+"/delete" {
		t.Fatalf("edit links = %q/%q", edit.Action, edit.DeleteHref)
	}

	// A byte-identical re-upload keeps the live path and discards the duplicate.
	liveOptionsBefore := liveOptionIDs(t, f.db, qid)
	flash, err = f.svc.UpdateQuestion(ctx, f.owner, quizID, qid, QuestionInput{
		Type:         string(viewmodel.QuestionSingle),
		Body:         create.Body,
		ImageAlt:     create.ImageAlt,
		AltPresent:   true,
		Options:      create.Options,
		CorrectIndex: create.CorrectIndex,
		Images:       []Upload{{Filename: "same.png", Data: firstPNG}},
	})
	if err != nil {
		t.Fatalf("UpdateQuestion(no-op): %v", err)
	}
	requireFlash(t, flash, FlashQuestionSaved)
	unchanged := readRawQuestion(t, f.db, qid)
	if unchanged.ImagePath.String != question.ImagePath.String {
		t.Fatalf("no-op changed the image path")
	}
	if got := countUploadFiles(t, f.uploads); got != 1 {
		t.Fatalf("upload count after no-op = %d, want 1", got)
	}
	if got := liveOptionIDs(t, f.db, qid); !slices.Equal(got, liveOptionsBefore) {
		t.Fatalf("no-op replaced options: %v -> %v", liveOptionsBefore, got)
	}

	// Replace the body, the option set, and the image.
	secondPNG := pngBytes(t, 250, 0, 0)
	flash, err = f.svc.UpdateQuestion(ctx, f.owner, quizID, qid, QuestionInput{
		Type:         string(viewmodel.QuestionSingle),
		Body:         "Berapakah hasil dari 3 + 4?",
		ImageAlt:     "Gambar baru",
		AltPresent:   true,
		Options:      []string{"6", "7", "8"},
		CorrectIndex: []string{"1"},
		Images:       []Upload{{Filename: "new.png", ContentType: "image/jpeg", Data: secondPNG}},
	})
	if err != nil {
		t.Fatalf("UpdateQuestion(replace): %v", err)
	}
	requireFlash(t, flash, FlashQuestionSaved)

	replaced := readRawQuestion(t, f.db, qid)
	if replaced.Prompt != "Berapakah hasil dari 3 + 4?" {
		t.Fatalf("prompt = %q", replaced.Prompt)
	}
	if replaced.ImagePath.String == question.ImagePath.String {
		t.Fatalf("image path did not change")
	}
	if replaced.ImageMime.String != "image/png" || replaced.ImageAlt.String != "Gambar baru" {
		t.Fatalf("replacement image = %+v", replaced)
	}
	requireNoFile(t, firstPath)
	secondPath := filepath.Join(f.uploads, replaced.ImagePath.String)
	requireFile(t, secondPath, len(secondPNG))

	options = readRawOptions(t, f.db, qid)
	var live, firstDeleted []rawOption
	for _, opt := range options {
		if opt.DeletedAt.Equal(store.LiveAtTime) {
			live = append(live, opt)
		} else {
			firstDeleted = append(firstDeleted, opt)
		}
	}
	if len(live) != 3 || len(firstDeleted) != 3 {
		t.Fatalf("option groups = live %d, deleted %d", len(live), len(firstDeleted))
	}
	for i, opt := range live {
		if opt.Position != i+1 || opt.Body != []string{"6", "7", "8"}[i] || opt.Correct != (i == 1) {
			t.Fatalf("live option %d = %+v", i, opt)
		}
	}
	firstStamp := firstDeleted[0].DeletedAt
	for _, opt := range firstDeleted {
		if !opt.DeletedAt.Equal(firstStamp) {
			t.Fatalf("first replacement stamps differ: %v vs %v", opt.DeletedAt, firstStamp)
		}
	}

	// Replacing again must move T_opt past the previous soft-delete stamp.
	flash, err = f.svc.UpdateQuestion(ctx, f.owner, quizID, qid, QuestionInput{
		Type:         string(viewmodel.QuestionSingle),
		Body:         "Berapakah hasil dari 9 + 1?",
		Options:      []string{"9", "10"},
		CorrectIndex: []string{"0"},
	})
	if err != nil {
		t.Fatalf("UpdateQuestion(second replace): %v", err)
	}
	requireFlash(t, flash, FlashQuestionSaved)
	if kept := readRawQuestion(t, f.db, qid); kept.ImagePath.String != replaced.ImagePath.String {
		t.Fatalf("an update without an upload changed the image")
	}

	options = readRawOptions(t, f.db, qid)
	var secondStamp time.Time
	live = nil
	for _, opt := range options {
		switch {
		case opt.DeletedAt.Equal(store.LiveAtTime):
			live = append(live, opt)
		case opt.DeletedAt.Equal(firstStamp):
		default:
			if secondStamp.IsZero() {
				secondStamp = opt.DeletedAt
			} else if !opt.DeletedAt.Equal(secondStamp) {
				t.Fatalf("second replacement stamps differ: %v vs %v", opt.DeletedAt, secondStamp)
			}
		}
	}
	if len(live) != 2 || live[0].Body != "9" || live[1].Body != "10" || !live[0].Correct {
		t.Fatalf("second live options = %+v", live)
	}
	if secondStamp.IsZero() || !secondStamp.After(firstStamp) {
		t.Fatalf("T_opt did not advance: %v <= %v", secondStamp, firstStamp)
	}

	// Remove the image and clear its alt text.
	flash, err = f.svc.UpdateQuestion(ctx, f.owner, quizID, qid, QuestionInput{
		Type:         string(viewmodel.QuestionSingle),
		Body:         "Berapakah hasil dari 9 + 1?",
		Options:      []string{"9", "10"},
		CorrectIndex: []string{"0"},
		RemoveImage:  true,
		AltPresent:   true,
		ImageAlt:     "diabaikan",
	})
	if err != nil {
		t.Fatalf("UpdateQuestion(remove image): %v", err)
	}
	requireFlash(t, flash, FlashQuestionSaved)
	removed := readRawQuestion(t, f.db, qid)
	if removed.ImagePath.Valid || removed.ImageMime.Valid || removed.ImageBytes.Valid ||
		len(removed.ImageSHA) != 0 || removed.ImageAlt.Valid {
		t.Fatalf("image row not cleared: %+v", removed)
	}
	requireNoFile(t, secondPath)
	if got := countUploadFiles(t, f.uploads); got != 0 {
		t.Fatalf("upload count after remove = %d, want 0", got)
	}
	withoutImage, err := f.svc.EditQuestionForm(ctx, f.owner, quizID, qid)
	if err != nil {
		t.Fatalf("EditQuestionForm(no image): %v", err)
	}
	if withoutImage.Image != nil {
		t.Fatalf("edit form still has an image: %+v", withoutImage.Image)
	}

	// An essay ignores submitted options entirely.
	essayID := f.createQuestion(t, quizID, QuestionInput{
		Type:         string(viewmodel.QuestionEssay),
		Body:         "Jelaskan proses fotosintesis pada tumbuhan.",
		Options:      []string{"A", "B"},
		CorrectIndex: []string{"0"},
	})
	essayRow := readRawQuestion(t, f.db, essayID)
	if essayRow.Kind != "essay" || essayRow.Position != 2 {
		t.Fatalf("essay row = %+v", essayRow)
	}
	if opts := readRawOptions(t, f.db, essayID); len(opts) != 0 {
		t.Fatalf("essay options = %+v", opts)
	}
	essayForm, err := f.svc.EditQuestionForm(ctx, f.owner, quizID, essayID)
	if err != nil {
		t.Fatalf("EditQuestionForm(essay): %v", err)
	}
	if essayForm.Type != viewmodel.QuestionEssay || len(essayForm.Options) != 0 {
		t.Fatalf("essay form = %+v", essayForm)
	}
	flash, err = f.svc.UpdateQuestion(ctx, f.owner, quizID, essayID, essay(
		"Jelaskan proses fotosintesis dan respirasi pada tumbuhan."))
	if err != nil {
		t.Fatalf("UpdateQuestion(essay): %v", err)
	}
	requireFlash(t, flash, FlashQuestionSaved)

	// Every field is validated before anything is staged or written.
	tooMany := make([]string, MaxOptions+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("Pilihan %d", i+1)
	}
	validSingle := func() QuestionInput {
		return singleChoice("Soal yang cukup panjang untuk validasi", []string{"A", "B"}, 0)
	}
	cases := []struct {
		name    string
		in      QuestionInput
		field   string
		message string
	}{
		{"empty prompt", QuestionInput{
			Type: string(viewmodel.QuestionSingle), Body: "   ",
			Options: []string{"A", "B"}, CorrectIndex: []string{"0"},
		}, "body", MsgPromptRequired},
		{"short prompt", QuestionInput{
			Type: string(viewmodel.QuestionSingle), Body: "pendek",
			Options: []string{"A", "B"}, CorrectIndex: []string{"0"},
		}, "body", MsgPromptTooShort},
		{"long prompt", QuestionInput{
			Type: string(viewmodel.QuestionSingle), Body: strings.Repeat("a", 2001),
			Options: []string{"A", "B"}, CorrectIndex: []string{"0"},
		}, "body", MsgPromptTooLong},
		{"single missing key", QuestionInput{
			Type: string(viewmodel.QuestionSingle), Body: "Soal yang cukup panjang",
			Options: []string{"A", "B"},
		}, "correct_index", MsgSingleKey},
		{"single out of range", QuestionInput{
			Type: string(viewmodel.QuestionSingle), Body: "Soal yang cukup panjang",
			Options: []string{"A", "B"}, CorrectIndex: []string{"2"},
		}, "correct_index", MsgSingleKey},
		{"multiple one key", multipleChoice("Soal yang cukup panjang", []string{"A", "B"}, 0),
			"correct_indexes", MsgMultipleKey},
		{"multiple out of range", QuestionInput{
			Type: string(viewmodel.QuestionMultiple), Body: "Soal yang cukup panjang",
			Options: []string{"A", "B"}, CorrectIndexes: []string{"0", "9"},
		}, "correct_indexes", MsgOptionKeyInvalid},
		{"one option", QuestionInput{
			Type: string(viewmodel.QuestionSingle), Body: "Soal yang cukup panjang",
			Options: []string{"A"}, CorrectIndex: []string{"0"},
		}, "option_body", MsgOptionsMin},
		{"too many options", QuestionInput{
			Type: string(viewmodel.QuestionSingle), Body: "Soal yang cukup panjang",
			Options: tooMany, CorrectIndex: []string{"0"},
		}, "option_body", MsgOptionsMax},
		{"interior blank option", QuestionInput{
			Type: string(viewmodel.QuestionSingle), Body: "Soal yang cukup panjang",
			Options: []string{"A", "", "C"}, CorrectIndex: []string{"0"},
		}, "option_body[1]", "Pilihan B wajib diisi."},
		{"long option", QuestionInput{
			Type: string(viewmodel.QuestionSingle), Body: "Soal yang cukup panjang",
			Options: []string{"A", strings.Repeat("b", 501)}, CorrectIndex: []string{"0"},
		}, "option_body[1]", "Pilihan B maksimal 500 karakter."},
		{"bad type", QuestionInput{
			Type: "truth", Body: "Soal yang cukup panjang",
		}, "type", MsgQuestionTypeInvalid},
		{"non-image upload", func() QuestionInput {
			in := validSingle()
			in.Images = []Upload{{Filename: "x.png", Data: []byte("not an image")}}
			return in
		}(), "image", MsgImageType},
		{"empty image", func() QuestionInput {
			in := validSingle()
			in.Images = []Upload{{Filename: "empty.png"}}
			return in
		}(), "image", MsgImageEmpty},
		{"two images", func() QuestionInput {
			in := validSingle()
			in.Images = []Upload{
				{Filename: "a.png", Data: pngBytes(t, 1, 1, 1)},
				{Filename: "b.png", Data: pngBytes(t, 2, 2, 2)},
			}
			return in
		}(), "image", MsgOneImage},
		{"long alt", func() QuestionInput {
			in := essay("Soal esai dengan teks alternatif terlalu panjang")
			in.ImageAlt = strings.Repeat("a", 256)
			in.AltPresent = true
			in.Images = []Upload{{Filename: "a.png", Data: pngBytes(t, 3, 3, 3)}}
			return in
		}(), "image_alt", MsgAltTooLong},
	}

	for _, tc := range cases {
		t.Run("validation/"+tc.name, func(t *testing.T) {
			before := countUploadFiles(t, f.uploads)
			_, err := f.svc.CreateQuestion(ctx, f.owner, quizID, tc.in)
			appErr := requireValidation(t, err)
			if got := appErr.Fields()[tc.field]; got != tc.message {
				t.Fatalf("field %q = %q, want %q (all: %v)", tc.field, got, tc.message, appErr.Fields())
			}
			if after := countUploadFiles(t, f.uploads); after != before {
				t.Fatalf("a rejected question staged a file: %d -> %d", before, after)
			}
		})
	}
	if got := liveQuestionCount(t, f.db, quizID); got != 2 {
		t.Fatalf("live question count after validation = %d, want 2", got)
	}

	// Non-owner and missing rows are the same 404.
	_, err = f.svc.EditQuestionForm(ctx, f.other, quizID, qid)
	requireNotFound(t, err)
	_, err = f.svc.CreateQuestion(ctx, f.other, quizID,
		essay("Soal yang tidak boleh dibuat guru lain"))
	requireNotFound(t, err)
	_, err = f.svc.UpdateQuestion(ctx, f.other, quizID, qid,
		essay("Soal yang tidak boleh diubah guru lain"))
	requireNotFound(t, err)
	_, err = f.svc.DeleteQuestion(ctx, f.other, quizID, qid)
	requireNotFound(t, err)
	_, err = f.svc.ReorderQuestions(ctx, f.other, quizID,
		fmt.Sprintf("%d,%d", qid, essayID))
	requireNotFound(t, err)

	_, err = f.svc.EditQuestionForm(ctx, f.owner, quizID, 999999)
	requireNotFound(t, err)
	_, err = f.svc.UpdateQuestion(ctx, f.owner, quizID, 999999,
		essay("Soal yang tidak ada di kuis ini"))
	requireNotFound(t, err)
	_, err = f.svc.DeleteQuestion(ctx, f.owner, quizID, 999999)
	requireNotFound(t, err)

	otherQuiz := f.createQuiz(t, QuizInput{Title: "Kuis Lain", Visibility: "public"})
	_, err = f.svc.EditQuestionForm(ctx, f.owner, otherQuiz, qid)
	requireNotFound(t, err)
	_, err = f.svc.UpdateQuestion(ctx, f.owner, otherQuiz, qid,
		essay("Soal dari kuis lain tidak boleh diubah"))
	requireNotFound(t, err)
	_, err = f.svc.DeleteQuestion(ctx, f.owner, otherQuiz, qid)
	requireNotFound(t, err)
}

// TestIntegrationLookupImage proves the serving lookup of AUT-054 against the
// real schema: the join is by image_path over two soft-delete filters, a
// malformed or orphaned name is the same 404, and a shared path is an
// integrity fault rather than a served image.
func TestIntegrationLookupImage(t *testing.T) {
	f := newAuthoringFixture(t)
	ctx := context.Background()
	quizID := f.createQuiz(t, QuizInput{Title: "Kuis Bergambar", Visibility: "public"})

	first := pngBytes(t, 10, 20, 30)
	qid := f.createQuestion(t, quizID, QuestionInput{
		Type:   string(viewmodel.QuestionEssay),
		Body:   "Jelaskan gambar berikut dengan lengkap",
		Images: []Upload{{Filename: "a.png", Data: first}},
	})
	name := readRawQuestion(t, f.db, qid).ImagePath.String

	record, err := f.svc.LookupImage(ctx, name)
	if err != nil {
		t.Fatalf("LookupImage: %v", err)
	}
	wantSHA := sha256.Sum256(first)
	if record.Name != name || record.QuestionID != qid || record.QuizID != quizID ||
		record.MIME != "image/png" || record.Bytes != int64(len(first)) ||
		!bytes.Equal(record.SHA256, wantSHA[:]) {
		t.Fatalf("record = %+v", record)
	}

	// A path shared by two live questions violates AUT-INV-09.
	if _, err := f.db.Exec(`INSERT INTO questions (quiz_id, position, type, prompt, image_path, image_mime, image_bytes, image_sha256)
		VALUES (?, 2, 'essay', 'Soal kedua yang berbagi gambar', ?, 'image/png', ?, ?)`,
		quizID, name, len(first), wantSHA[:]); err != nil {
		t.Fatalf("seed shared path: %v", err)
	}
	if _, err := f.svc.LookupImage(ctx, name); err == nil || apperr.IsNotFound(err) {
		t.Fatalf("shared path = %v, want integrity fault", err)
	}
	if _, err := f.db.Exec("DELETE FROM questions WHERE quiz_id = ? AND position = 2", quizID); err != nil {
		t.Fatalf("delete shared row: %v", err)
	}

	for _, bad := range []string{"../" + name, strings.Repeat("A", 32) + ".png", strings.Repeat("a", 32) + ".jpeg"} {
		if _, err := f.svc.LookupImage(ctx, bad); !apperr.IsNotFound(err) {
			t.Fatalf("LookupImage(%q) = %v, want 404", bad, err)
		}
	}
	if _, err := f.svc.LookupImage(ctx, strings.Repeat("c", 32)+".png"); !apperr.IsNotFound(err) {
		t.Fatalf("orphan lookup = %v, want 404", err)
	}

	// A deleted question tombstones its capability URL even if a file remains.
	if _, err := f.svc.DeleteQuestion(ctx, f.owner, quizID, qid); err != nil {
		t.Fatalf("DeleteQuestion: %v", err)
	}
	if _, err := f.svc.LookupImage(ctx, name); !apperr.IsNotFound(err) {
		t.Fatalf("deleted question lookup = %v, want 404", err)
	}

	second := pngBytes(t, 40, 50, 60)
	qid2 := f.createQuestion(t, quizID, QuestionInput{
		Type:   string(viewmodel.QuestionEssay),
		Body:   "Soal kedua dengan gambar yang berbeda",
		Images: []Upload{{Filename: "b.png", Data: second}},
	})
	name2 := readRawQuestion(t, f.db, qid2).ImagePath.String
	if _, err := f.svc.LookupImage(ctx, name2); err != nil {
		t.Fatalf("LookupImage(second): %v", err)
	}
	if _, err := f.svc.DeleteQuiz(ctx, f.owner, quizID); err != nil {
		t.Fatalf("DeleteQuiz: %v", err)
	}
	if _, err := f.svc.LookupImage(ctx, name2); !apperr.IsNotFound(err) {
		t.Fatalf("deleted quiz lookup = %v, want 404", err)
	}
}
