package attempts

// stream.go implements the live monitor stream (ATT-191–ATT-198). The stream is
// a read-only projection of loadMonitorSnapshot: one read-only transaction per
// tick, a diff by registration_id, and no replay buffer. The HTTP adapter owns
// the response writer; this file owns the event names, the payload shape, and
// the termination rules.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

const (
	streamTickInterval = 2 * time.Second
	streamHeartbeat    = 30 * time.Second
)

// StreamInput is GET /teacher/quizzes/:id/stream material.
type StreamInput struct {
	QuizID    int64
	AccountID int64
}

// StreamStart is the pre-header ownership and state decision: a missing,
// foreign, or draft quiz is ErrNotFound (empty 404); an owned open or closed
// quiz may start the response (ATT-191).
type StreamStart struct {
	QuizID int64
	State  viewmodel.QuizState
}

// StreamStart resolves the quiz before any response header is written.
func (s *Service) StreamStart(ctx context.Context, in StreamInput) (StreamStart, error) {
	var out StreamStart
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
			SELECT id, state FROM quizzes
			WHERE id = ? AND owner_account_id = ? AND deleted_at = ?`,
			in.QuizID, in.AccountID, store.LiveAt,
		).Scan(&out.QuizID, &out.State)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return internal(err)
		}
		if out.State == viewmodel.QuizDraft {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return StreamStart{}, err
	}
	return out, nil
}

// Stream writes the SSE events for one connection until the quiz closes, the
// client disconnects (emit error), the server shuts down (stop closed), or a
// tick fails (no event, ATT-261). The caller supplies the framing; this method
// supplies only the event name and the compact JSON payload.
func (s *Service) Stream(ctx context.Context, in StreamInput, stop <-chan struct{}, emit func(name string, data []byte) error) error {
	select {
	case <-stop:
		return nil
	default:
	}

	snap, err := s.monitorSnapshot(ctx, in)
	if err != nil {
		// A failed initial read closes with no event; the browser reconnects
		// and gets a fresh snapshot (ATT-261).
		return nil
	}
	if snap.State != viewmodel.QuizOpen {
		return emitJSON(emit, "quiz.closed", closedPayload{QuizID: snap.QuizRef.ID})
	}
	if err := emitJSON(emit, "snapshot", buildSnapshotPayload(snap)); err != nil {
		return err
	}

	lastEvent := time.Now()
	timer := time.NewTimer(streamTickInterval)
	defer timer.Stop()
	for {
		select {
		case <-stop:
			return nil
		case <-timer.C:
			next, err := s.monitorSnapshot(ctx, in)
			if err != nil {
				return nil
			}
			if next.State != viewmodel.QuizOpen {
				return emitJSON(emit, "quiz.closed", closedPayload{QuizID: next.QuizRef.ID})
			}
			snapshot, added, updated := diffMonitorSnapshot(snap, next)
			if snapshot {
				if err := emitJSON(emit, "snapshot", buildSnapshotPayload(next)); err != nil {
					return err
				}
				lastEvent = time.Now()
			} else {
				for i := range added {
					if err := emitJSON(emit, "participant.added", buildParticipant(added[i])); err != nil {
						return err
					}
					lastEvent = time.Now()
				}
				for i := range updated {
					if err := emitJSON(emit, "participant.updated", buildParticipantUpdate(updated[i])); err != nil {
						return err
					}
					lastEvent = time.Now()
				}
			}
			snap = next
			if time.Since(lastEvent) >= streamHeartbeat {
				if err := emitJSON(emit, "heartbeat", heartbeatPayload{
					At: time.Now().UTC().Format(time.RFC3339),
				}); err != nil {
					return err
				}
				lastEvent = time.Now()
			}
			timer.Reset(streamTickInterval)
		}
	}
}

// monitorSnapshot runs loadMonitorSnapshot in its own read-only transaction so
// every tick is one consistent snapshot (ATT-191, ATT-187).
func (s *Service) monitorSnapshot(ctx context.Context, in StreamInput) (monitorSnapshotData, error) {
	var snap monitorSnapshotData
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		snap, err = loadMonitorSnapshot(ctx, tx, in.QuizID, in.AccountID)
		return err
	})
	return snap, err
}

// diffMonitorSnapshot compares two snapshots by registration_id (ATT-195).
// A removed row or a changed identity label (name, initials, class, avatar)
// needs a fresh snapshot because the event vocabulary has no removed/label
// event. Added and updated rows keep ATT-204-free monitor order.
func diffMonitorSnapshot(prev, next monitorSnapshotData) (snapshot bool, added, updated []participantSnapshot) {
	prevByID := make(map[int64]participantSnapshot, len(prev.Rows))
	for i := range prev.Rows {
		prevByID[prev.Rows[i].RegistrationID] = prev.Rows[i]
	}
	nextByID := make(map[int64]bool, len(next.Rows))
	for i := range next.Rows {
		nextByID[next.Rows[i].RegistrationID] = true
	}
	for id := range prevByID {
		if !nextByID[id] {
			snapshot = true
			break
		}
	}

	for i := range next.Rows {
		row := next.Rows[i]
		old, ok := prevByID[row.RegistrationID]
		if !ok {
			added = append(added, row)
			continue
		}
		if old.Name != row.Name || old.Initials != row.Initials ||
			old.ClassLabel != row.ClassLabel || old.Avatar.URL != row.Avatar.URL {
			snapshot = true
			continue
		}
		if participantChanged(old, row) {
			updated = append(updated, row)
		}
	}
	if snapshot {
		return true, nil, nil
	}
	return false, added, updated
}

// participantChanged is the ATT-194 update subset: attempt_id, answered, total,
// state, focus_loss, last_seen. Identity labels and the avatar URL are
// deliberately absent: an avatar change is an identity change, so it reaches
// the client as a fresh snapshot (AVT-084), never as an update event. A
// score change is also a change even though the payload never carries the
// score: grading preserves attempt.updated_at, so this signal is the only way
// an open monitor learns that the page-only Skor column needs a reload.
func participantChanged(old, next participantSnapshot) bool {
	if !sameAttemptID(old.AttemptID, next.AttemptID) {
		return true
	}
	if old.Answered != next.Answered || old.Total != next.Total ||
		old.State != next.State || old.FocusLoss != next.FocusLoss {
		return true
	}
	if !sameScore(old.Score, next.Score) {
		return true
	}
	return !old.LastSeen.Equal(next.LastSeen)
}

func sameScore(a, b *viewmodel.Score) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Percent == b.Percent
}

func sameAttemptID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// ---- Payloads -------------------------------------------------------------

type streamQuiz struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	State string `json:"state"`
}

type streamProgress struct {
	Registered int `json:"registered"`
	InProgress int `json:"in_progress"`
	Completed  int `json:"completed"`
	Answered   int `json:"answered"`
	Total      int `json:"total"`
}

type streamParticipant struct {
	RegistrationID int64  `json:"registration_id"`
	AttemptID      *int64 `json:"attempt_id"`
	Name           string `json:"name"`
	Initials       string `json:"initials"`
	Avatar         string `json:"avatar"`
	ClassLabel     string `json:"class_label"`
	Answered       int    `json:"answered"`
	Total          int    `json:"total"`
	State          string `json:"state"`
	FocusLoss      int    `json:"focus_loss"`
	LastSeen       string `json:"last_seen"`
}

type streamParticipantUpdate struct {
	RegistrationID int64  `json:"registration_id"`
	AttemptID      *int64 `json:"attempt_id"`
	Answered       int    `json:"answered"`
	Total          int    `json:"total"`
	State          string `json:"state"`
	FocusLoss      int    `json:"focus_loss"`
	LastSeen       string `json:"last_seen"`
}

type streamSnapshot struct {
	Quiz         streamQuiz          `json:"quiz"`
	Progress     streamProgress      `json:"progress"`
	Participants []streamParticipant `json:"participants"`
	GeneratedAt  string              `json:"generated_at"`
}

type closedPayload struct {
	QuizID int64 `json:"quiz_id"`
}

type heartbeatPayload struct {
	At string `json:"at"`
}

func buildSnapshotPayload(snap monitorSnapshotData) streamSnapshot {
	participants := make([]streamParticipant, 0, len(snap.Rows))
	for i := range snap.Rows {
		participants = append(participants, buildParticipant(snap.Rows[i]))
	}
	return streamSnapshot{
		Quiz: streamQuiz{
			ID:    snap.QuizRef.ID,
			Title: snap.QuizRef.Title,
			State: string(snap.State),
		},
		Progress: streamProgress{
			Registered: snap.Progress.Registered,
			InProgress: snap.Progress.InProgress,
			Completed:  snap.Progress.Completed,
			Answered:   snap.Progress.Answered,
			Total:      snap.Progress.Total,
		},
		Participants: participants,
		GeneratedAt:  snap.GeneratedAt.UTC().Format(time.RFC3339),
	}
}

func buildParticipant(row participantSnapshot) streamParticipant {
	return streamParticipant{
		RegistrationID: row.RegistrationID,
		AttemptID:      row.AttemptID,
		Name:           row.Name,
		Initials:       row.Initials,
		Avatar:         row.Avatar.URL,
		ClassLabel:     row.ClassLabel,
		Answered:       row.Answered,
		Total:          row.Total,
		State:          string(row.State),
		FocusLoss:      row.FocusLoss,
		LastSeen:       row.LastSeen.UTC().Format(time.RFC3339),
	}
}

func buildParticipantUpdate(row participantSnapshot) streamParticipantUpdate {
	return streamParticipantUpdate{
		RegistrationID: row.RegistrationID,
		AttemptID:      row.AttemptID,
		Answered:       row.Answered,
		Total:          row.Total,
		State:          string(row.State),
		FocusLoss:      row.FocusLoss,
		LastSeen:       row.LastSeen.UTC().Format(time.RFC3339),
	}
}

func emitJSON(emit func(name string, data []byte) error, name string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return emit(name, data)
}
