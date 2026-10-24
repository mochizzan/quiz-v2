package authoring

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/viewmodel"
)

// ---- Lifecycle decisions --------------------------------------------------

func TestContentDecision(t *testing.T) {
	tests := []struct {
		state    string
		allowed  bool
		conflict string
		wantErr  bool
	}{
		{"draft", true, "", false},
		{"open", false, FlashErrNotDraft, false},
		{"closed", false, FlashErrNotDraft, false},
		{"bogus", false, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.state, func(t *testing.T) {
			d, err := contentDecision(tc.state)
			assertDecision(t, d, err, tc.allowed, tc.conflict, tc.wantErr)
		})
	}
}

func TestPublishDecision(t *testing.T) {
	tests := []struct {
		state    string
		allowed  bool
		conflict string
		wantErr  bool
	}{
		{"draft", true, "", false},
		{"open", false, FlashAlreadyOpen, false},
		{"closed", false, FlashErrClosedTerminal, false},
		{"bogus", false, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.state, func(t *testing.T) {
			d, err := publishDecision(tc.state)
			assertDecision(t, d, err, tc.allowed, tc.conflict, tc.wantErr)
		})
	}
}

func TestCloseDecision(t *testing.T) {
	tests := []struct {
		state    string
		allowed  bool
		conflict string
		wantErr  bool
	}{
		{"open", true, "", false},
		{"closed", false, FlashClosed, false},
		{"draft", false, FlashErrNotOpen, false},
		{"bogus", false, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.state, func(t *testing.T) {
			d, err := closeDecision(tc.state)
			assertDecision(t, d, err, tc.allowed, tc.conflict, tc.wantErr)
		})
	}
}

func TestDeleteDecision(t *testing.T) {
	tests := []struct {
		state    string
		allowed  bool
		conflict string
		wantErr  bool
	}{
		{"draft", true, "", false},
		{"open", false, FlashErrDeleteNotDraft, false},
		{"closed", false, FlashErrDeleteNotDraft, false},
		{"bogus", false, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.state, func(t *testing.T) {
			d, err := deleteDecision(tc.state)
			assertDecision(t, d, err, tc.allowed, tc.conflict, tc.wantErr)
		})
	}
}

func assertDecision(t *testing.T, d decision, err error, allowed bool, conflictKey string, wantErr bool) {
	t.Helper()
	if wantErr {
		if err == nil {
			t.Fatalf("want an integrity error, got decision %+v", d)
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Allowed != allowed {
		t.Fatalf("allowed = %v, want %v", d.Allowed, allowed)
	}
	if d.Conflict != conflictKey {
		t.Fatalf("conflict = %q, want %q", d.Conflict, conflictKey)
	}
}

func TestGuard(t *testing.T) {
	err := guard(decision{Conflict: FlashErrNotDraft}, nil)
	if key, ok := ConflictKey(err); !ok || key != FlashErrNotDraft {
		t.Fatalf("guard conflict = %q, %v", key, ok)
	}
	err = guard(decision{}, errUnknownState)
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Kind() != apperr.Internal {
		t.Fatalf("unknown state must be internal, got %v", err)
	}
	if err := guard(decision{Allowed: true}, nil); err != nil {
		t.Fatalf("allowed decision returned %v", err)
	}
}

// ---- Quiz and settings validation -----------------------------------------

func TestValidateQuizCreate(t *testing.T) {
	base := QuizInput{Title: "Kuis IPA", Visibility: "public"}
	tests := []struct {
		name  string
		in    QuizInput
		field string
		msg   string
	}{
		{"title empty", QuizInput{Visibility: "public"}, "title", MsgTitleRequired},
		{"title too long", QuizInput{Title: strings.Repeat("a", 151), Visibility: "public"}, "title", MsgTitleTooLong},
		{"description too long", QuizInput{Title: "Kuis", Visibility: "public", Description: strings.Repeat("a", 1001)}, "description", MsgDescriptionTooLong},
		{"start invalid", QuizInput{Title: "Kuis", Visibility: "public", StartAt: "2026-10-23T09:00Z"}, "start_at", MsgStartInvalid},
		{"end invalid", QuizInput{Title: "Kuis", Visibility: "public", EndAt: "23-10-2026 09:00"}, "end_at", MsgEndInvalid},
		{"end equal start", QuizInput{Title: "Kuis", Visibility: "public", StartAt: "2026-10-23T09:00", EndAt: "2026-10-23T09:00"}, "end_at", MsgEndNotAfterStart},
		{"visibility invalid", QuizInput{Title: "Kuis"}, "visibility", MsgVisibilityInvalid},
		{"grade required", QuizInput{Title: "Kuis", Visibility: "private"}, "target_grade", MsgGradeRequired},
		{"grade invalid", QuizInput{Title: "Kuis", Visibility: "private", TargetGrade: "9", TargetMajorID: "1"}, "target_grade", MsgGradeInvalid},
		{"major required", QuizInput{Title: "Kuis", Visibility: "private", TargetGrade: "10"}, "target_major_id", MsgMajorRequired},
		{"major invalid", QuizInput{Title: "Kuis", Visibility: "private", TargetGrade: "10", TargetMajorID: "0"}, "target_major_id", MsgMajorUnavailable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateQuizCreate(tc.in)
			assertField(t, err, tc.field, tc.msg)
		})
	}

	out, err := ValidateQuizCreate(QuizInput{
		Title: "  Kuis\nIPA  ", Description: "  Deskripsi  ",
		Visibility: "public", StartAt: "2026-10-23T09:15", EndAt: "2026-10-23T10:00:59",
	})
	if err != nil {
		t.Fatalf("valid public create: %v", err)
	}
	if out.Title != "Kuis\nIPA" || out.Description != "Deskripsi" {
		t.Fatalf("text not normalised: %+v", out)
	}
	if out.StartsAt == nil || out.StartsAt.UTC().Format(time.RFC3339) != "2026-10-23T02:15:00Z" {
		t.Fatalf("start not parsed as WIB: %v", out.StartsAt)
	}
	if out.EndsAt == nil || out.EndsAt.UTC().Format(time.RFC3339) != "2026-10-23T03:00:00Z" {
		t.Fatalf("end seconds not truncated: %v", out.EndsAt)
	}

	out, err = ValidateQuizCreate(QuizInput{
		Title: "Kuis privat", Visibility: "private", TargetGrade: "11", TargetMajorID: "4",
	})
	if err != nil {
		t.Fatalf("valid private create: %v", err)
	}
	if out.Visibility != viewmodel.VisibilityPrivate || out.TargetGrade != 11 || out.TargetMajorID != 4 {
		t.Fatalf("audience not captured: %+v", out)
	}
	_ = base
}

func TestValidateQuizMetaIgnoresAudience(t *testing.T) {
	out, err := ValidateQuizMeta(QuizInput{Title: "Judul baru", Visibility: "bogus", TargetGrade: "x"})
	if err != nil {
		t.Fatalf("metadata must ignore audience: %v", err)
	}
	if out.Title != "Judul baru" {
		t.Fatalf("title = %q", out.Title)
	}
}

func TestValidateSettings(t *testing.T) {
	base := func() url.Values {
		return url.Values{
			"review_mode":  {"questions_only"},
			"max_attempts": {"1"},
			"show_score":   {"1"},
		}
	}
	tests := []struct {
		name   string
		mutate func(url.Values)
		field  string
		msg    string
	}{
		{"attempts zero", func(v url.Values) { v.Set("allow_retake", "1"); v.Set("max_attempts", "0") }, "max_attempts", MsgAttemptsRange},
		{"attempts eleven", func(v url.Values) { v.Set("allow_retake", "1"); v.Set("max_attempts", "11") }, "max_attempts", MsgAttemptsRange},
		{"attempts not a number", func(v url.Values) { v.Set("allow_retake", "1"); v.Set("max_attempts", "dua") }, "max_attempts", MsgAttemptsRange},
		{"checkbox contract", func(v url.Values) { v.Set("allow_retake", "yes") }, "allow_retake", MsgSettingsInvalid},
		{"review mode", func(v url.Values) { v.Set("review_mode", "all") }, "review_mode", MsgReviewModeInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			form := base()
			tc.mutate(form)
			_, err := ValidateSettings(form)
			assertField(t, err, tc.field, tc.msg)
		})
	}

	out, err := ValidateSettings(base())
	if err != nil {
		t.Fatalf("defaults: %v", err)
	}
	if out.MaxAttempts != 1 || !out.ShowScore || out.AllowRetake || out.RandomizeQuestions {
		t.Fatalf("defaults = %+v", out)
	}

	form := base()
	form.Set("allow_retake", "1")
	form.Set("max_attempts", "5")
	form.Set("randomize_questions", "1")
	form.Set("randomize_answers", "1")
	form.Set("review_mode", "answers_only")
	out, err = ValidateSettings(form)
	if err != nil {
		t.Fatalf("valid retake: %v", err)
	}
	if out.MaxAttempts != 5 || !out.AllowRetake || !out.RandomizeQuestions || !out.RandomizeAnswers || out.ReviewMode != viewmodel.ReviewAnswersOnly {
		t.Fatalf("valid retake = %+v", out)
	}

	// allow_retake=false forces max_attempts=1 even when the number is junk.
	form = base()
	form.Set("max_attempts", "9")
	out, err = ValidateSettings(form)
	if err != nil || out.MaxAttempts != 1 {
		t.Fatalf("forced attempts = %+v, %v", out, err)
	}
}

// ---- Question validation --------------------------------------------------

func TestValidateQuestion(t *testing.T) {
	base := func() QuestionInput {
		return QuestionInput{
			Type:         "single",
			Body:         "Pertanyaan yang cukup panjang",
			Options:      []string{"Pilihan A", "Pilihan B"},
			CorrectIndex: []string{"0"},
		}
	}
	tests := []struct {
		name   string
		mutate func(*QuestionInput)
		field  string
		msg    string
	}{
		{"type", func(in *QuestionInput) { in.Type = "multi" }, "type", MsgQuestionTypeInvalid},
		{"body empty", func(in *QuestionInput) { in.Body = "   " }, "body", MsgPromptRequired},
		{"body short", func(in *QuestionInput) { in.Body = "1234567" }, "body", MsgPromptTooShort},
		{"body long", func(in *QuestionInput) { in.Body = strings.Repeat("a", 2001) }, "body", MsgPromptTooLong},
		{"options one", func(in *QuestionInput) { in.Options = []string{"A"} }, "option_body", MsgOptionsMin},
		{"options too many", func(in *QuestionInput) {
			in.Options = make([]string, 27)
			for i := range in.Options {
				in.Options[i] = "Pilihan"
			}
		}, "option_body", MsgOptionsMax},
		{"interior blank", func(in *QuestionInput) { in.Options = []string{"A", "", "C"} }, "option_body[1]", "Pilihan B wajib diisi."},
		{"leading blank", func(in *QuestionInput) { in.Options = []string{"", "B"} }, "option_body[0]", "Pilihan A wajib diisi."},
		{"option too long", func(in *QuestionInput) { in.Options = []string{strings.Repeat("a", 501), "B"} }, "option_body[0]", "Pilihan A maksimal 500 karakter."},
		{"single missing key", func(in *QuestionInput) { in.CorrectIndex = nil }, "correct_index", MsgSingleKey},
		{"single two keys", func(in *QuestionInput) { in.CorrectIndex = []string{"0", "1"} }, "correct_index", MsgSingleKey},
		{"single out of range", func(in *QuestionInput) { in.CorrectIndex = []string{"2"} }, "correct_index", MsgSingleKey},
		{"multiple one key", func(in *QuestionInput) {
			in.Type = "multiple"
			in.CorrectIndexes = []string{"1"}
		}, "correct_indexes", MsgMultipleKey},
		{"multiple duplicate keys", func(in *QuestionInput) {
			in.Type = "multiple"
			in.CorrectIndexes = []string{"1", "1"}
		}, "correct_indexes", MsgMultipleKey},
		{"multiple out of range", func(in *QuestionInput) {
			in.Type = "multiple"
			in.CorrectIndexes = []string{"0", "9"}
		}, "correct_indexes", MsgOptionKeyInvalid},
		{"alt too long", func(in *QuestionInput) { in.ImageAlt = strings.Repeat("a", 256) }, "image_alt", MsgAltTooLong},
		{"two images", func(in *QuestionInput) {
			in.Images = []Upload{{Data: pngStub()}, {Data: pngStub()}}
		}, "image", MsgOneImage},
		{"empty image", func(in *QuestionInput) { in.Images = []Upload{{Filename: "x.png"}} }, "image", MsgImageEmpty},
		{"gif image", func(in *QuestionInput) { in.Images = []Upload{{Data: gifBytes()}} }, "image", MsgImageType},
		{"oversized image", func(in *QuestionInput) {
			in.Images = []Upload{{Data: make([]byte, MaxImageBytes+1)}}
		}, "image", MsgImageTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := base()
			tc.mutate(&in)
			_, err := ValidateQuestion(in)
			assertField(t, err, tc.field, tc.msg)
		})
	}

	// Trailing blank rows are dropped, and duplicate correct indexes collapse.
	in := base()
	in.Options = []string{"A", "B", "  ", ""}
	in.CorrectIndexes = []string{"0", "0"}
	in.Type = "multiple"
	in.CorrectIndexes = []string{"0", "1", "1"}
	out, err := ValidateQuestion(in)
	if err != nil {
		t.Fatalf("trailing blanks: %v", err)
	}
	if len(out.Options) != 2 || !out.Options[0].Correct || !out.Options[1].Correct {
		t.Fatalf("multiple keys = %+v", out.Options)
	}

	// Essay ignores option bodies and keys entirely.
	in = base()
	in.Type = "essay"
	in.Options = []string{""}
	in.CorrectIndex = nil
	out, err = ValidateQuestion(in)
	if err != nil {
		t.Fatalf("essay: %v", err)
	}
	if len(out.Options) != 0 {
		t.Fatalf("essay options = %+v", out.Options)
	}

	// A valid image carries MIME, extension, size, and alt only when non-empty.
	in = base()
	in.ImageAlt = "  Diagram sel  "
	in.Images = []Upload{{Data: pngStub()}}
	out, err = ValidateQuestion(in)
	if err != nil {
		t.Fatalf("valid image: %v", err)
	}
	if out.Image == nil || out.Image.MIME != "image/png" || out.Image.Ext != ".png" {
		t.Fatalf("image = %+v", out.Image)
	}
	if out.Alt == nil || *out.Alt != "Diagram sel" || out.AltText != "Diagram sel" {
		t.Fatalf("alt = %+v / %q", out.Alt, out.AltText)
	}

	// Alt without an image is ignored for storage but still normalised.
	in = base()
	in.ImageAlt = "  teks  "
	out, err = ValidateQuestion(in)
	if err != nil {
		t.Fatalf("alt without image: %v", err)
	}
	if out.Alt != nil || out.AltText != "teks" {
		t.Fatalf("alt without image = %+v / %q", out.Alt, out.AltText)
	}
}

func TestParseLocalDateTime(t *testing.T) {
	tests := []struct {
		raw     string
		present bool
		ok      bool
		want    string
	}{
		{"", false, true, ""},
		{"2026-10-23T08:30", true, true, "2026-10-23T01:30:00Z"},
		{"2026-10-23T08:30:59", true, true, "2026-10-23T01:30:00Z"},
		{"2026-10-23T08:30Z", true, false, ""},
		{"2026-10-23T08:30+07:00", true, false, ""},
		{"2026-02-30T08:30", true, false, ""},
		{"23-10-2026 08:30", true, false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			got, present, ok := ParseLocalDateTime(tc.raw)
			if present != tc.present || ok != tc.ok {
				t.Fatalf("present=%v ok=%v, want %v/%v", present, ok, tc.present, tc.ok)
			}
			if ok && present && got.UTC().Format(time.RFC3339) != tc.want {
				t.Fatalf("got %s, want %s", got.UTC().Format(time.RFC3339), tc.want)
			}
		})
	}
}

func TestNormalizeText(t *testing.T) {
	got := NormalizeText(" \r\nHalo\rDunia\x00\t  ")
	if got != "Halo\nDunia" {
		t.Fatalf("NormalizeText = %q", got)
	}
	if got := NormalizeText(" Halo\tDunia "); got != "Halo\tDunia" {
		t.Fatalf("NormalizeText dropped an internal tab: %q", got)
	}
	if RuneLen("é🙂") != 2 {
		t.Fatalf("RuneLen counted bytes")
	}
	if TruncateRunes("abcdef", 3) != "abc" || TruncateRunes("abc", 5) != "abc" {
		t.Fatalf("TruncateRunes failed")
	}
}

// ---- Publish gate ---------------------------------------------------------

func TestEvaluatePublish(t *testing.T) {
	uploads := t.TempDir()

	tests := []struct {
		name    string
		mutate  func(*PublishInput)
		reason  string
		wantErr bool
	}{
		{"no questions", func(in *PublishInput) {
			in.Questions = nil
			in.Quiz.MaxAttempts = 0
		}, FlashErrNoQuestions, false},
		{"private targets missing", func(in *PublishInput) {
			in.Quiz.Visibility = "private"
			in.Quiz.Code = sql.NullString{}
			in.Questions[0].Position = 9
		}, FlashErrTargets, false},
		{"private major missing", func(in *PublishInput) {
			in.Quiz.Visibility = "private"
			in.Quiz.Code = sql.NullString{}
			in.Quiz.TargetGrade = sql.NullInt64{Int64: 10, Valid: true}
			in.Quiz.TargetMajorID = sql.NullInt64{Int64: 3, Valid: true}
			in.Quiz.MajorLive = false
		}, FlashErrTargetMissing, false},
		{"position gap", func(in *PublishInput) { in.Questions[0].Position = 2 }, FlashErrInvalidQuestion, false},
		{"short prompt", func(in *PublishInput) { in.Questions[0].Prompt = "pendek" }, FlashErrInvalidQuestion, false},
		{"single no key", func(in *PublishInput) { in.Questions[0].Options[0].Correct = false }, FlashErrInvalidQuestion, false},
		{"single two keys", func(in *PublishInput) { in.Questions[0].Options[1].Correct = true }, FlashErrInvalidQuestion, false},
		{"option gap", func(in *PublishInput) { in.Questions[0].Options[1].Position = 3 }, FlashErrInvalidQuestion, false},
		{"empty option", func(in *PublishInput) { in.Questions[0].Options[0].Body = " " }, FlashErrInvalidQuestion, false},
		{"essay with options", func(in *PublishInput) { in.Questions[0].Type = "essay" }, FlashErrInvalidQuestion, false},
		{"image missing", func(in *PublishInput) {
			in.Questions[0].ImagePath = sql.NullString{String: strings.Repeat("a", 32) + ".png", Valid: true}
			in.Questions[0].ImageMime = sql.NullString{String: "image/png", Valid: true}
			in.Questions[0].ImageBytes = sql.NullInt64{Int64: 10, Valid: true}
			in.Questions[0].ImageSHA = make([]byte, 32)
		}, FlashErrImageMissing, false},
		{"partial image row", func(in *PublishInput) {
			in.Questions[0].ImagePath = sql.NullString{String: strings.Repeat("a", 32) + ".png", Valid: true}
		}, "", true},
		{"unsupported image mime", func(in *PublishInput) {
			in.Questions[0].ImagePath = sql.NullString{String: strings.Repeat("a", 32) + ".gif", Valid: true}
			in.Questions[0].ImageMime = sql.NullString{String: "image/gif", Valid: true}
			in.Questions[0].ImageBytes = sql.NullInt64{Int64: 10, Valid: true}
			in.Questions[0].ImageSHA = make([]byte, 32)
		}, "", true},
		{"public with targets", func(in *PublishInput) {
			in.Quiz.TargetGrade = sql.NullInt64{Int64: 10, Valid: true}
		}, FlashErrInvalidSettings, false},
		{"attempts out of range", func(in *PublishInput) { in.Quiz.MaxAttempts = 0 }, FlashErrInvalidSettings, false},
		{"retake mismatch", func(in *PublishInput) { in.Quiz.MaxAttempts = 3 }, FlashErrInvalidSettings, false},
		{"review mode", func(in *PublishInput) { in.Quiz.ReviewMode = "all" }, FlashErrInvalidSettings, false},
		{"public code missing", func(in *PublishInput) { in.Quiz.Code = sql.NullString{} }, "", true},
		{"public code not six digits", func(in *PublishInput) {
			in.Quiz.Code = sql.NullString{String: "12345", Valid: true}
		}, "", true},
		{"invite token missing", func(in *PublishInput) { in.Quiz.InviteToken = " " }, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := validPublishInput(uploads)
			tc.mutate(&in)
			result := EvaluatePublish(in)
			if tc.wantErr {
				if result.Err == nil {
					t.Fatalf("want integrity error, got %+v", result)
				}
				return
			}
			if result.Err != nil {
				t.Fatalf("unexpected integrity error: %v", result.Err)
			}
			if result.OK {
				t.Fatalf("unexpected OK, reason %q", result.Reason)
			}
			if result.Reason != tc.reason {
				t.Fatalf("reason = %q, want %q", result.Reason, tc.reason)
			}
		})
	}
}

func TestEvaluatePublishPrivateLiveMajor(t *testing.T) {
	// The detail page previously left MajorLive at its zero value, so every
	// private draft reported a missing target (AUT-025/AUT-027). This pins the
	// passing branch the detail snapshot must produce for a live major.
	in := validPublishInput(t.TempDir())
	in.Quiz.Visibility = string(viewmodel.VisibilityPrivate)
	in.Quiz.Code = sql.NullString{}
	in.Quiz.TargetGrade = sql.NullInt64{Int64: 10, Valid: true}
	in.Quiz.TargetMajorID = sql.NullInt64{Int64: 3, Valid: true}
	in.Quiz.MajorLive = true
	if result := EvaluatePublish(in); !result.OK || result.Err != nil || result.Reason != "" {
		t.Fatalf("private draft with live major = %+v", result)
	}
}

func TestEvaluatePublishOKWithImage(t *testing.T) {
	uploads := t.TempDir()
	in := validPublishInput(uploads)
	name := strings.Repeat("b", 32) + ".png"
	if err := os.WriteFile(filepath.Join(uploads, name), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	in.Questions[0].ImagePath = sql.NullString{String: name, Valid: true}
	in.Questions[0].ImageMime = sql.NullString{String: "image/png", Valid: true}
	in.Questions[0].ImageBytes = sql.NullInt64{Int64: 10, Valid: true}
	in.Questions[0].ImageSHA = make([]byte, 32)
	if result := EvaluatePublish(in); !result.OK || result.Err != nil {
		t.Fatalf("valid image gate = %+v", result)
	}

	// Size drift is a user-fixable missing image, not an integrity fault.
	if err := os.WriteFile(filepath.Join(uploads, name), []byte("0123"), 0o644); err != nil {
		t.Fatal(err)
	}
	if result := EvaluatePublish(in); result.Reason != FlashErrImageMissing || result.Err != nil {
		t.Fatalf("size drift = %+v", result)
	}

	// Extension family must match the stored MIME.
	in.Questions[0].ImagePath = sql.NullString{String: strings.Repeat("b", 32) + ".jpg", Valid: true}
	if result := EvaluatePublish(in); result.Reason != FlashErrImageMissing || result.Err != nil {
		t.Fatalf("extension mismatch = %+v", result)
	}
}

func validPublishInput(uploads string) PublishInput {
	return PublishInput{
		Quiz: PublishQuiz{
			Visibility:  "public",
			Code:        sql.NullString{String: "012345", Valid: true},
			InviteToken: "abcdefghijklmnopqrstuv",
			MaxAttempts: 1,
			ShowScore:   true,
			ReviewMode:  "questions_only",
		},
		Questions: []PublishQuestion{{
			ID:       1,
			Position: 1,
			Type:     "single",
			Prompt:   "Pertanyaan yang cukup panjang",
			Options: []PublishOption{
				{Position: 1, Body: "Pilihan A", Correct: true},
				{Position: 2, Body: "Pilihan B"},
			},
		}},
		UploadsDir: uploads,
	}
}

// ---- Images ---------------------------------------------------------------

func TestValidImageName(t *testing.T) {
	for _, name := range []string{
		strings.Repeat("a", 32) + ".png",
		strings.Repeat("0", 32) + ".jpg",
		strings.Repeat("f", 32) + ".webp",
	} {
		if !ValidImageName(name) {
			t.Fatalf("ValidImageName(%q) = false", name)
		}
	}
	for _, name := range []string{
		"",
		strings.Repeat("a", 32) + ".jpeg",
		strings.Repeat("a", 32) + ".gif",
		strings.Repeat("A", 32) + ".png",
		strings.Repeat("a", 31) + ".png",
		strings.Repeat("a", 33) + ".png",
		strings.Repeat("a", 32) + ".png.png",
		"../" + strings.Repeat("a", 32) + ".png",
		strings.Repeat("a", 32) + ".png/x",
		strings.Repeat("a", 32),
		"..",
	} {
		if ValidImageName(name) {
			t.Fatalf("ValidImageName(%q) = true", name)
		}
	}
}

func TestValidateImageNameBeforeDatabase(t *testing.T) {
	// The grammar is checked before the pool, so a malformed name is 404 even
	// when no database is configured (AUT-015).
	svc := New(nil, "")
	if _, err := svc.LookupImage(context.Background(), "bad"); !apperr.IsNotFound(err) {
		t.Fatalf("malformed lookup = %v, want 404", err)
	}
	if _, err := svc.LookupImage(context.Background(), strings.Repeat("a", 32)+".png"); err == nil || apperr.IsNotFound(err) {
		t.Fatalf("pool-less lookup = %v, want internal", err)
	}
}

func TestImageSafety(t *testing.T) {
	valid := strings.Repeat("a", 32) + ".png"
	if !safeImageName.MatchString(valid) {
		t.Fatalf("generated grammar rejected a valid name")
	}
	for _, bad := range []string{
		strings.Repeat("A", 32) + ".png",
		strings.Repeat("a", 31) + ".png",
		strings.Repeat("a", 33) + ".png",
		strings.Repeat("a", 32) + ".jpeg",
		strings.Repeat("a", 32) + ".png.png",
		"../" + valid,
		valid + "/x",
		"",
	} {
		if safeImageName.MatchString(bad) {
			t.Fatalf("unsafe name accepted: %q", bad)
		}
	}

	dir := t.TempDir()
	if err := removeImage(dir, "../escape.png"); err == nil {
		t.Fatalf("removeImage accepted traversal")
	}

	name, err := newImageName(".webp")
	if err != nil {
		t.Fatal(err)
	}
	if !safeImageName.MatchString(name) {
		t.Fatalf("newImageName = %q", name)
	}
}

func TestStageAndCheckImage(t *testing.T) {
	dir := t.TempDir()
	blob, msg := validateUpload(Upload{Filename: "client.PNG", Data: pngStub()})
	if msg != "" || blob == nil {
		t.Fatalf("validateUpload = %v, %q", blob, msg)
	}
	name, err := stageImage(dir, blob)
	if err != nil {
		t.Fatalf("stageImage: %v", err)
	}
	if !safeImageName.MatchString(name) {
		t.Fatalf("staged name = %q", name)
	}
	info, err := os.Stat(filepath.Join(dir, name))
	if err != nil || info.Size() != int64(len(pngStub())) {
		t.Fatalf("staged file = %v, %v", info, err)
	}

	if reason, err := checkImageFile(dir, name, "image/png", int64(len(pngStub()))); reason != "" || err != nil {
		t.Fatalf("checkImageFile valid = %q, %v", reason, err)
	}
	if reason, err := checkImageFile(dir, name, "image/jpeg", int64(len(pngStub()))); reason != FlashErrImageMissing || err != nil {
		t.Fatalf("checkImageFile mime mismatch = %q, %v", reason, err)
	}
	if reason, err := checkImageFile(dir, name, "image/png", 1); reason != FlashErrImageMissing || err != nil {
		t.Fatalf("checkImageFile size drift = %q, %v", reason, err)
	}
	if reason, err := checkImageFile(dir, strings.Repeat("c", 32)+".png", "image/png", 1); reason != FlashErrImageMissing || err != nil {
		t.Fatalf("checkImageFile missing = %q, %v", reason, err)
	}
	if _, err := checkImageFile(dir, "../escape.png", "image/png", 1); err == nil {
		t.Fatalf("checkImageFile accepted traversal")
	}

	if err := removeImage(dir, name); err != nil {
		t.Fatalf("removeImage: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
		t.Fatalf("staged file survived removeImage")
	}
	if err := removeImage(dir, name); err != nil {
		t.Fatalf("removeImage must tolerate a missing file: %v", err)
	}
}

// ---- Ordering and delete stamps ------------------------------------------

func TestParseReorder(t *testing.T) {
	valid := map[string][]int64{
		"3,1,2":  {3, 1, 2},
		" 3 , 1": {3, 1},
		"7":      {7},
	}
	for raw, want := range valid {
		got, err := ParseReorder(raw)
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		if len(got) != len(want) {
			t.Fatalf("%q = %v, want %v", raw, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%q = %v, want %v", raw, got, want)
			}
		}
	}
	for _, raw := range []string{"", " ", "1,,2", "1,x", "0", "-1", "1,1", "1.0", ","} {
		if _, err := ParseReorder(raw); !errors.Is(err, ErrReorder) {
			t.Fatalf("%q: want ErrReorder, got %v", raw, err)
		}
	}
}

func TestPlanShift(t *testing.T) {
	tests := []struct {
		maxPos, n, want int
		ok              bool
	}{
		{0, 0, 0, true},
		{0, 2, 3, true},
		{5, 3, 9, true},
		{65530, 10, 0, false},
		{65000, 200, 0, false},
	}
	for _, tc := range tests {
		got, ok := PlanShift(tc.maxPos, tc.n)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("PlanShift(%d,%d) = %d,%v want %d,%v", tc.maxPos, tc.n, got, ok, tc.want, tc.ok)
		}
	}
}

func TestNextStamp(t *testing.T) {
	t0 := time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)
	got, ok := NextStamp(t0, nil)
	if !ok || !got.Equal(t0) {
		t.Fatalf("NextStamp nil = %v,%v", got, ok)
	}
	later := t0.Add(3 * time.Second)
	got, ok = NextStamp(t0, []sql.NullTime{{Time: later, Valid: true}, {}})
	if !ok || !got.Equal(later.Add(time.Second)) {
		t.Fatalf("NextStamp later = %v,%v", got, ok)
	}
	got, ok = NextStamp(t0, []sql.NullTime{{Time: t0.Add(-time.Hour), Valid: true}})
	if !ok || !got.Equal(t0) {
		t.Fatalf("NextStamp earlier = %v,%v", got, ok)
	}
	if _, ok := NextStamp(t0, []sql.NullTime{{Time: maxDateTime, Valid: true}}); ok {
		t.Fatalf("NextStamp must abort past DATETIME max")
	}
}

// ---- Flash and list normalisation ----------------------------------------

func TestFlashForKey(t *testing.T) {
	for _, key := range []string{
		FlashQuizCreated, FlashSaved, FlashQuestionSaved, FlashPublished,
		FlashClosed, FlashReordered, FlashQuizDeleted, FlashQuestionDeleted,
		FlashAlreadyOpen, FlashErrNoQuestions, FlashErrTargets,
		FlashErrTargetMissing, FlashErrInvalidQuestion, FlashErrImageMissing,
		FlashErrInvalidSettings, FlashErrClosedTerminal, FlashErrNotOpen,
		FlashErrNotDraft, FlashErrDeleteNotDraft, FlashErrReorder,
		FlashErrQuestionLimit, FlashErrForm,
	} {
		if FlashForKey(key) == nil {
			t.Fatalf("FlashForKey(%q) is nil", key)
		}
	}
	// AUT-033 pins err-form as a danger flash key with the MsgFormInvalid copy.
	// The HTTP layer also uses the same string as the 422 `_form` field key;
	// the two uses share the copy but never the mechanism.
	if got := FlashForKey(FlashErrForm); got == nil || got.Kind != "danger" || got.Title != MsgFormInvalid {
		t.Fatalf("FlashForKey(err-form) = %+v", got)
	}
	if FlashForKey("unknown") != nil {
		t.Fatalf("unknown flash key must be ignored")
	}
	if got := ReasonText(FlashErrNoQuestions); got != MsgNoQuestions {
		t.Fatalf("ReasonText = %q", got)
	}
}

func TestNormalizeTabAndSort(t *testing.T) {
	for _, state := range []string{"draft", "open", "closed"} {
		if got := NormalizeTab(state); got != state {
			t.Fatalf("NormalizeTab(%q) = %q", state, got)
		}
	}
	for _, state := range []string{"", "all", "DRAFT", "bogus"} {
		if got := NormalizeTab(state); got != "all" {
			t.Fatalf("NormalizeTab(%q) = %q", state, got)
		}
	}
	for _, order := range []viewmodel.SortOrder{viewmodel.SortOldest, viewmodel.SortNameAsc, viewmodel.SortNameDesc} {
		if got := NormalizeSort(string(order)); got != order {
			t.Fatalf("NormalizeSort(%q) = %q", order, got)
		}
	}
	if got := NormalizeSort("bogus"); got != viewmodel.SortNewest {
		t.Fatalf("NormalizeSort(bogus) = %q", got)
	}
}

// ---- Helpers --------------------------------------------------------------

func assertField(t *testing.T, err *apperr.Error, field, msg string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want a validation error for %q", field)
	}
	if err.Kind() != apperr.Validation {
		t.Fatalf("kind = %v, want validation", err.Kind())
	}
	if got := err.Fields()[field]; got != msg {
		t.Fatalf("field %q = %q, want %q (all: %v)", field, got, msg, err.Fields())
	}
}

func pngStub() []byte {
	return []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
}

func gifBytes() []byte {
	return []byte("GIF89a")
}
