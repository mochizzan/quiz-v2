package attempts

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"quiz-v2/internal/viewmodel"
)

// TestStreamStart pins ATT-191's pre-header decision: owned open and closed
// quizzes may start (closed sends one event), while draft/foreign/missing are
// an empty 404.
func TestStreamStart(t *testing.T) {
	f := newTeacherFixture(t)
	open := f.newQuiz(f.teacherID, "Siaran", "open", true, false, 1, "2026-10-20 09:00:00")
	closed := f.newQuiz(f.teacherID, "Siaran Selesai", "closed", true, false, 1, "2026-10-20 09:30:00")
	draft := f.newQuiz(f.teacherID, "Siaran Draf", "draft", true, false, 1, "2026-10-20 10:00:00")

	for _, tc := range []struct {
		name  string
		quiz  int64
		state viewmodel.QuizState
	}{
		{"open", open, viewmodel.QuizOpen},
		{"closed", closed, viewmodel.QuizClosed},
	} {
		start, err := f.svc.StreamStart(f.ctx, StreamInput{QuizID: tc.quiz, AccountID: f.teacherID})
		if err != nil {
			t.Fatalf("%s StreamStart: %v", tc.name, err)
		}
		if start.QuizID != tc.quiz || start.State != tc.state {
			t.Errorf("%s start = %+v, want id %d state %s", tc.name, start, tc.quiz, tc.state)
		}
	}

	for _, tc := range []struct {
		name string
		in   StreamInput
	}{
		{"draft", StreamInput{QuizID: draft, AccountID: f.teacherID}},
		{"foreign", StreamInput{QuizID: open, AccountID: f.otherTeacherID}},
		{"missing", StreamInput{QuizID: 999999, AccountID: f.teacherID}},
	} {
		if _, err := f.svc.StreamStart(f.ctx, tc.in); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s StreamStart error = %v, want ErrNotFound", tc.name, err)
		}
	}
}

// TestStreamInitialEvents pins ATT-192/ATT-193/ATT-197: an open quiz emits a
// snapshot immediately, and a quiz that is already closed emits one
// quiz.closed and ends.
func TestStreamInitialEvents(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Siaran Awal", "open", true, false, 1, "2026-10-20 09:00:00")
	essay := f.question(quiz, 1, "essay", "Jelaskan")
	ayu := f.newStudent("Ayu Lestari", 10)
	reg := f.register(quiz, ayu, "2026-10-23 08:00:00")
	attempt := f.attempt(reg, 1, 9, nil, nil, "2026-10-23 08:30:00", "2026-10-23 09:00:00")
	f.answer(attempt, essay, "Draf", nil, nil, nil, nil)
	f.focus(attempt, "blur", "2026-10-23 08:40:00")

	var events []streamEvent
	err := f.svc.Stream(f.ctx, StreamInput{QuizID: quiz, AccountID: f.teacherID},
		make(chan struct{}),
		func(name string, data []byte) error {
			events = append(events, streamEvent{name: name, data: string(data)})
			return errStopStream
		})
	if !errors.Is(err, errStopStream) {
		t.Fatalf("open Stream error = %v, want the emit sentinel", err)
	}
	if len(events) != 1 || events[0].name != "snapshot" {
		t.Fatalf("initial events = %+v, want one snapshot", events)
	}
	var snap streamSnapshot
	if err := json.Unmarshal([]byte(events[0].data), &snap); err != nil {
		t.Fatalf("snapshot JSON: %v", err)
	}
	if snap.Quiz.ID != quiz || snap.Quiz.Title != "Siaran Awal" || snap.Quiz.State != "open" {
		t.Errorf("snapshot quiz = %+v", snap.Quiz)
	}
	if snap.Progress.Registered != 1 || snap.Progress.InProgress != 1 ||
		snap.Progress.Completed != 0 || snap.Progress.Answered != 1 || snap.Progress.Total != 1 {
		t.Errorf("snapshot progress = %+v", snap.Progress)
	}
	if len(snap.Participants) != 1 {
		t.Fatalf("participants = %+v", snap.Participants)
	}
	got := snap.Participants[0]
	if got.RegistrationID != reg || got.AttemptID == nil || *got.AttemptID != attempt ||
		got.Name != "Ayu Lestari" || got.Initials != "AL" || got.ClassLabel != "X RPL" ||
		got.Answered != 1 || got.Total != 1 || got.State != "in_progress" ||
		got.FocusLoss != 1 || !strings.HasSuffix(got.LastSeen, "Z") {
		t.Errorf("participant = %+v", got)
	}
	if !strings.HasSuffix(snap.GeneratedAt, "Z") {
		t.Errorf("generated_at = %q, want a UTC Z timestamp", snap.GeneratedAt)
	}

	// Closed at connect: one quiz.closed event, then the method returns nil.
	f.setQuizState(quiz, "closed")
	events = nil
	err = f.svc.Stream(f.ctx, StreamInput{QuizID: quiz, AccountID: f.teacherID},
		make(chan struct{}),
		func(name string, data []byte) error {
			events = append(events, streamEvent{name: name, data: string(data)})
			return nil
		})
	if err != nil {
		t.Fatalf("closed Stream error = %v", err)
	}
	if len(events) != 1 || events[0].name != "quiz.closed" {
		t.Fatalf("closed events = %+v, want one quiz.closed", events)
	}
	if events[0].data != `{"quiz_id":`+idString(quiz)+`}` {
		t.Errorf("quiz.closed data = %s", events[0].data)
	}
}

// TestStreamLiveTick pins ATT-195/ATT-197 on a real two-second tick: a changed
// participant produces participant.updated, a closed quiz produces quiz.closed,
// and the stop channel ends the stream cleanly.
func TestStreamLiveTick(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Siaran Hidup", "open", true, false, 1, "2026-10-20 09:00:00")
	essay := f.question(quiz, 1, "essay", "Jelaskan")
	ayu := f.newStudent("Ayu Lestari", 10)
	reg := f.register(quiz, ayu, "2026-10-23 08:00:00")
	attempt := f.attempt(reg, 1, 10, nil, nil, "2026-10-23 08:30:00", "2026-10-23 09:00:00")
	f.answer(attempt, essay, "Draf", nil, nil, nil, nil)

	events := make(chan streamEvent, 8)
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- f.svc.Stream(f.ctx, StreamInput{QuizID: quiz, AccountID: f.teacherID},
			stop, func(name string, data []byte) error {
				events <- streamEvent{name: name, data: string(data)}
				return nil
			})
	}()

	first := waitEvent(t, events)
	if first.name != "snapshot" {
		t.Fatalf("first event = %q, want snapshot", first.name)
	}

	// A focus event changes focus_loss, which is in the update subset.
	f.focus(attempt, "blur", "2026-10-23 09:01:00")
	update := waitEvent(t, events)
	if update.name != "participant.updated" {
		t.Fatalf("second event = %q, want participant.updated", update.name)
	}
	var payload streamParticipantUpdate
	if err := json.Unmarshal([]byte(update.data), &payload); err != nil {
		t.Fatalf("update JSON: %v", err)
	}
	if payload.RegistrationID != reg || payload.AttemptID == nil || *payload.AttemptID != attempt ||
		payload.FocusLoss != 1 || payload.Answered != 1 || payload.State != "in_progress" ||
		!strings.HasSuffix(payload.LastSeen, "Z") {
		t.Errorf("update payload = %+v", payload)
	}
	if strings.Contains(update.data, `"name"`) || strings.Contains(update.data, `"class_label"`) {
		t.Errorf("update leaked identity labels: %s", update.data)
	}

	// Closing the quiz ends the stream after exactly one quiz.closed.
	f.setQuizState(quiz, "closed")
	closed := waitEvent(t, events)
	if closed.name != "quiz.closed" {
		t.Fatalf("third event = %q, want quiz.closed", closed.name)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Stream returned %v after quiz.closed", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stream did not end after quiz.closed")
	}
	if len(events) != 0 {
		t.Errorf("events after quiz.closed = %d, want none", len(events))
	}
}

// TestStreamStopChannel pins ATT-197: a closed stop channel ends the stream
// with no event.
func TestStreamStopChannel(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Siaran Berhenti", "open", true, false, 1, "2026-10-20 09:00:00")
	stop := make(chan struct{})
	close(stop)
	called := false
	if err := f.svc.Stream(f.ctx, StreamInput{QuizID: quiz, AccountID: f.teacherID},
		stop, func(name string, data []byte) error {
			called = true
			return nil
		}); err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	if called {
		t.Error("stopped stream emitted an event")
	}
}

// TestDiffMonitorSnapshot pins ATT-195's comparison table.
func TestDiffMonitorSnapshot(t *testing.T) {
	attempt := int64(9001)
	otherAttempt := int64(9002)
	seen := time.Date(2026, 10, 23, 2, 42, 10, 0, time.UTC)

	row := func(id int64, opts ...func(*participantSnapshot)) participantSnapshot {
		row := participantSnapshot{
			RegistrationID: id,
			AttemptID:      &attempt,
			Name:           "Ayu Lestari",
			Initials:       "AL",
			ClassLabel:     "XII IPA",
			State:          viewmodel.ParticipationInProgress,
			Answered:       7,
			Total:          30,
			FocusLoss:      1,
			LastSeen:       seen,
		}
		for _, opt := range opts {
			opt(&row)
		}
		return row
	}
	snap := func(rows ...participantSnapshot) monitorSnapshotData {
		return monitorSnapshotData{Rows: rows}
	}

	t.Run("no change", func(t *testing.T) {
		snapshot, added, updated := diffMonitorSnapshot(
			snap(row(501), row(502)),
			snap(row(501), row(502)),
		)
		if snapshot || len(added) != 0 || len(updated) != 0 {
			t.Errorf("no-change diff = (%v, %+v, %+v)", snapshot, added, updated)
		}
	})

	t.Run("added", func(t *testing.T) {
		snapshot, added, updated := diffMonitorSnapshot(
			snap(row(501)),
			snap(row(501), row(502)),
		)
		if snapshot || len(updated) != 0 || len(added) != 1 || added[0].RegistrationID != 502 {
			t.Errorf("added diff = (%v, %+v, %+v)", snapshot, added, updated)
		}
	})

	t.Run("removed needs a snapshot", func(t *testing.T) {
		snapshot, added, updated := diffMonitorSnapshot(
			snap(row(501), row(502)),
			snap(row(501)),
		)
		if !snapshot || len(added) != 0 || len(updated) != 0 {
			t.Errorf("removed diff = (%v, %+v, %+v)", snapshot, added, updated)
		}
	})

	t.Run("identity change needs a snapshot", func(t *testing.T) {
		snapshot, _, _ := diffMonitorSnapshot(
			snap(row(501)),
			snap(row(501, func(r *participantSnapshot) { r.ClassLabel = "XI IPA" })),
		)
		if !snapshot {
			t.Error("class-label change did not request a snapshot")
		}
	})

	t.Run("update subset", func(t *testing.T) {
		next := row(501, func(r *participantSnapshot) {
			r.AttemptID = &otherAttempt
			r.Answered = 8
			r.FocusLoss = 2
			r.LastSeen = seen.Add(21 * time.Second)
		})
		snapshot, added, updated := diffMonitorSnapshot(snap(row(501)), snap(next))
		if snapshot || len(added) != 0 || len(updated) != 1 {
			t.Fatalf("update diff = (%v, %+v, %+v)", snapshot, added, updated)
		}
		if updated[0].Answered != 8 || !updated[0].LastSeen.Equal(seen.Add(21*time.Second)) {
			t.Errorf("updated row = %+v", updated[0])
		}
	})

	t.Run("attempt nil to set", func(t *testing.T) {
		prev := row(501, func(r *participantSnapshot) { r.AttemptID = nil })
		snapshot, _, updated := diffMonitorSnapshot(snap(prev), snap(row(501)))
		if snapshot || len(updated) != 1 {
			t.Errorf("attempt-start diff = (%v, %+v)", snapshot, updated)
		}
	})

	// Grading preserves attempt.updated_at, so a late essay score is only
	// visible to an open monitor through this comparison. The event payload
	// still carries the ATT-194 subset; the page reloads for the score.
	t.Run("late score is an update", func(t *testing.T) {
		prev := row(501, func(r *participantSnapshot) {
			r.State = viewmodel.ParticipationCompleted
		})
		next := row(501, func(r *participantSnapshot) {
			r.State = viewmodel.ParticipationCompleted
			score := viewmodel.NewScore(80)
			r.Score = &score
		})
		snapshot, _, updated := diffMonitorSnapshot(snap(prev), snap(next))
		if snapshot || len(updated) != 1 {
			t.Errorf("score diff = (%v, %+v)", snapshot, updated)
		}
	})
}

// TestStreamPayloads pins the exact ATT-193/ATT-194 key sets and UTC Z forms.
func TestStreamPayloads(t *testing.T) {
	seen := time.Date(2026, 10, 23, 9, 42, 10, 0, viewmodel.WIB)
	attempt := int64(9001)
	snap := monitorSnapshotData{
		QuizRef: viewmodel.QuizRef{ID: 12, Title: "Ulangan Bab 3"},
		State:   viewmodel.QuizOpen,
		Rows: []participantSnapshot{{
			RegistrationID: 501,
			AttemptID:      &attempt,
			Name:           "Ayu Lestari",
			Initials:       "AL",
			ClassLabel:     "XII IPA",
			State:          viewmodel.ParticipationInProgress,
			Answered:       7,
			Total:          30,
			FocusLoss:      1,
			LastSeen:       seen,
		}},
		Progress:    viewmodel.MonitorProgress{Registered: 1, InProgress: 1, Answered: 7, Total: 30},
		GeneratedAt: seen,
	}
	data, err := json.Marshal(buildSnapshotPayload(snap))
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	assertKeys(t, got, []string{"quiz", "progress", "participants", "generated_at"})
	quizObj := got["quiz"].(map[string]any)
	assertKeys(t, quizObj, []string{"id", "title", "state"})
	progress := got["progress"].(map[string]any)
	assertKeys(t, progress, []string{"registered", "in_progress", "completed", "answered", "total"})
	participants := got["participants"].([]any)
	if len(participants) != 1 {
		t.Fatalf("participants = %d", len(participants))
	}
	participant := participants[0].(map[string]any)
	assertKeys(t, participant, []string{
		"registration_id", "attempt_id", "name", "initials", "class_label",
		"answered", "total", "state", "focus_loss", "last_seen",
	})
	if participant["last_seen"] != "2026-10-23T02:42:10Z" {
		t.Errorf("last_seen = %v, want 2026-10-23T02:42:10Z", participant["last_seen"])
	}
	if got["generated_at"] != "2026-10-23T02:42:10Z" {
		t.Errorf("generated_at = %v", got["generated_at"])
	}

	update, err := json.Marshal(buildParticipantUpdate(snap.Rows[0]))
	if err != nil {
		t.Fatalf("marshal update: %v", err)
	}
	var gotUpdate map[string]any
	if err := json.Unmarshal(update, &gotUpdate); err != nil {
		t.Fatalf("unmarshal update: %v", err)
	}
	assertKeys(t, gotUpdate, []string{
		"registration_id", "attempt_id", "answered", "total", "state",
		"focus_loss", "last_seen",
	})

	// A participant that has not started serializes attempt_id as null.
	notStarted := snap
	notStarted.Rows = []participantSnapshot{{
		RegistrationID: 502,
		Name:           "Budi Santoso",
		Initials:       "BS",
		ClassLabel:     "XI IPA",
		State:          viewmodel.ParticipationRegistered,
		Total:          30,
		LastSeen:       seen,
	}}
	raw, err := json.Marshal(buildParticipant(notStarted.Rows[0]))
	if err != nil {
		t.Fatalf("marshal not-started: %v", err)
	}
	if !strings.Contains(string(raw), `"attempt_id":null`) {
		t.Errorf("not-started participant = %s, want null attempt_id", raw)
	}
}

type streamEvent struct {
	name string
	data string
}

var errStopStream = errors.New("stop stream")

func waitEvent(t *testing.T, events <-chan streamEvent) streamEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(6 * time.Second):
		t.Fatal("timed out waiting for a stream event")
		return streamEvent{}
	}
}

func assertKeys(t *testing.T, obj map[string]any, want []string) {
	t.Helper()
	if len(obj) != len(want) {
		t.Errorf("keys = %v, want %v", mapKeys(obj), want)
		return
	}
	for _, key := range want {
		if _, ok := obj[key]; !ok {
			t.Errorf("missing key %q in %v", key, mapKeys(obj))
		}
	}
}

func mapKeys(obj map[string]any) []string {
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	return keys
}
