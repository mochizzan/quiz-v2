package participation

// avatar_fill_test.go pins AVT-083 for PAR's student read surface: the owning
// teacher's stored photo name is read in the accounts join every quiz loader
// already performs, and reaches QuizCard.TeacherAvatar as a URL — or as the
// teacher's initials when the account has no photo, so a card never renders a
// broken image or an empty badge.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"quiz-v2/internal/store"
	"quiz-v2/internal/store/storetest"
	"quiz-v2/internal/viewmodel"
)

// readAvatarName builds a name the avatar grammar accepts: 32 lowercase hex
// characters plus .webp. Distinct seeds keep names distinct, because the schema
// keys (avatar_name, deleted_at) unique across live rows (AVT-083).
func readAvatarName(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:16]) + ".webp"
}

// giveReadAvatar stores a photo on one account and returns the stored name.
func giveReadAvatar(t *testing.T, db *store.DB, accountID int64, seed string) string {
	t.Helper()
	name := readAvatarName(seed)
	if _, err := db.Exec("UPDATE accounts SET avatar_name = ? WHERE id = ?", name, accountID); err != nil {
		t.Fatalf("store avatar for account %d: %v", accountID, err)
	}
	return name
}

// TestStudentDashboardCardFillsTeacherAvatar is the query half of AVT-083: the
// photo name rides along in the join that already supplies the teacher label,
// and both card variants come out of the same read.
func TestStudentDashboardCardFillsTeacherAvatar(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	ctx := context.Background()
	svc := readTestService()
	svc.db = db

	teacher := seedReadAccount(t, db, "guru-foto@example.test", "Bu Guru Foto", "teacher")
	photo := giveReadAvatar(t, db, teacher, "guru-foto")

	bare := seedReadAccount(t, db, "guru-polos@example.test", "Pak Guru Polos", "teacher")

	major := seedReadMajor(t, db, "Ilmu Pengetahuan Alam", "IPA")
	studentAccount := seedReadAccount(t, db, "siswa@example.test", "Ayu Lestari", "student")
	student := seedReadStudent(t, db, studentAccount, major, 11)
	actor := ActorInput{AccountID: studentAccount, Role: viewmodel.RoleStudent, FullName: "Ayu Lestari"}

	// A live card: registered with an open attempt.
	live := seedReadQuiz(t, db, readQuizSeed{
		owner: teacher, title: "Kuis Berfoto", visibility: "public", state: "open",
		code: "111111", invite: "invite-111111", published: readTestAgo(10 * 24 * time.Hour),
		allowRetake: true, maxAttempts: 2, showScore: true,
	})
	seedReadQuestions(t, db, live, 1)
	reg := seedReadRegistration(t, db, live, student, readTestAgo(5*time.Hour))
	seedReadAttempt(t, db, reg, 1, readTestAgo(5*time.Hour), readTestAgo(time.Hour), nil, nil)

	// A "new to you" card: open, eligible, and never registered.
	latest := seedReadQuiz(t, db, readQuizSeed{
		owner: bare, title: "Kuis Polos", visibility: "public", state: "open",
		code: "222222", invite: "invite-222222", published: readTestAgo(3 * 24 * time.Hour),
		allowRetake: false, maxAttempts: 1, showScore: true,
	})
	seedReadQuestions(t, db, latest, 1)

	dash, err := svc.StudentDashboard(ctx, StudentDashboardInput{Actor: actor})
	if err != nil {
		t.Fatalf("StudentDashboard: %v", err)
	}

	if len(dash.Live) != 1 {
		t.Fatalf("Live = %d, want 1", len(dash.Live))
	}
	if got := dash.Live[0].TeacherAvatar.URL; got != "/avatars/"+photo {
		t.Errorf("live card TeacherAvatar.URL = %q, want %q", got, "/avatars/"+photo)
	}
	if !strings.Contains(dash.Live[0].TeacherAvatar.Class, "app-avatar") {
		t.Errorf("live card TeacherAvatar.Class = %q", dash.Live[0].TeacherAvatar.Class)
	}

	if len(dash.Latest) != 1 {
		t.Fatalf("Latest = %d, want 1", len(dash.Latest))
	}
	photoless := dash.Latest[0].TeacherAvatar
	if photoless.URL != "" {
		t.Errorf("photo-less teacher rendered a URL %q", photoless.URL)
	}
	if photoless.Initials == "" {
		t.Error("photo-less teacher must fall back to initials")
	}
}
