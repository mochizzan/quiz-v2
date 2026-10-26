package attempts

// focus_beacon_href_test.go pins the page-to-endpoint half of ATT-156: both
// attempt screens carry POST /attempt/:id/beacon, so the client that posts the
// focus events always targets the attempt it is showing. The template hook and
// the client itself are checked structurally in
// httpserver/beacon_client_test.go; this guards the read that fills the field —
// the half a refactor drops silently, which is how a fully implemented beacon
// ended up attached to nothing.

import (
	"strconv"
	"testing"

	"quiz-v2/internal/viewmodel"
)

func TestAttemptScreensCarryTheirBeaconEndpoint(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Kuis Fokus", "open", true, false, 1, "2026-10-20 09:00:00")
	f.question(quiz, 1, "single", "Pilih satu")

	ayu := f.newStudent("Ayu Lestari", 10)
	owner := OwnerRef{Role: viewmodel.RoleStudent, AccountID: f.accountOf(ayu)}
	reg := f.register(quiz, ayu, "2026-10-23 08:00:00")
	attempt := f.attempt(reg, 1, 9, nil, nil, "2026-10-23 08:30:00", "2026-10-23 08:30:00")

	want := "/attempt/" + strconv.FormatInt(attempt, 10) + "/beacon"

	question, err := f.svc.QuestionPage(f.ctx, QuestionInput{AttemptID: uint32(attempt), Owner: owner})
	if err != nil {
		t.Fatalf("QuestionPage: %v", err)
	}
	if question.BeaconHref != want {
		t.Errorf("question beacon href = %q, want %q", question.BeaconHref, want)
	}

	review, err := f.svc.ReviewPage(f.ctx, ReviewInput{AttemptID: uint32(attempt), Owner: owner})
	if err != nil {
		t.Fatalf("ReviewPage: %v", err)
	}
	if review.BeaconHref != want {
		t.Errorf("review beacon href = %q, want %q", review.BeaconHref, want)
	}
}
