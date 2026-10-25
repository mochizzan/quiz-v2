package participation

// quiz.go implements the participant-facing quiz surface of the PAR domain:
// the intro read model (PAR-087…PAR-095) and the three participation commands
// — explicit student registration (PAR-061…PAR-069), cancellation
// (PAR-070…PAR-077), and start/resume with the ATT handoff (PAR-078…PAR-086).
//
// Access is derived from the actor resolved inside the transaction, never from
// a form field (PAR-071). A missing or out-of-audience resource is 404; a legal
// but refused transition is a Result carrying a closed flash key (PAR-020).

import (
	"context"
	"database/sql"
	"errors"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// The closed PAR flash keys of Â§5.0.
const (
	// FlashAlreadyRegistered is the info flash of an idempotent join.
	FlashAlreadyRegistered = "already_registered"
	// FlashNotOpen reports a draft/closed quiz state refusal.
	FlashNotOpen = "not-open"
	// FlashNotRegistered is the student start refusal without a registration.
	FlashNotRegistered = "not-registered"
	// FlashCancelBlocked reports a cancellation with attempts already started.
	FlashCancelBlocked = "cancel-blocked"
	// FlashAttemptsExhausted reports a spent attempt quota.
	FlashAttemptsExhausted = "attempts-exhausted"
	// FlashQuizEmpty reports the zero-question defense-in-depth gate.
	FlashQuizEmpty = "quiz-empty"
	// FlashRegistered is the success flash of a new registration.
	FlashRegistered = "registered"
	// FlashCancelled is the info flash of a cancelled registration.
	FlashCancelled = "cancelled"
	// FlashStarted is the success flash of a newly created attempt.
	FlashStarted = "started"
	// FlashIdentitySwitched is the info flash after PAR-052 clears the cookie.
	FlashIdentitySwitched = "identity-switched"
)

// QuizIntroInput is the GET /quiz/:id material.
type QuizIntroInput struct {
	QuizID int64
	Actor  ActorInput
}

// RegisterInput is POST /quiz/:id/register material.
type RegisterInput struct {
	QuizID int64
	Actor  ActorInput
}

// CancelInput is POST /quiz/:id/cancel material.
type CancelInput struct {
	QuizID int64
	Actor  ActorInput
}

// StartQuizInput is POST /quiz/:id/attempts material.
type StartQuizInput struct {
	QuizID int64
	Actor  ActorInput
}

// studentProfile is the locked eligibility snapshot of one student.
type studentProfile struct {
	GradeLevel uint8
	MajorID    uint32
}

// lockStudentProfile shared-locks the student row so a concurrent account edit
// cannot change eligibility under a decision (PAR-064, PAR-082). A profile that
// vanished after actor resolution is a 404, not a 500.
func lockStudentProfile(ctx context.Context, tx *sql.Tx, studentID uint32) (studentProfile, error) {
	var (
		grade int
		major sql.NullInt64
	)
	err := tx.QueryRowContext(ctx, `
		SELECT grade_level, major_id
		FROM students
		WHERE id = ? AND deleted_at = ?
		LOCK IN SHARE MODE`,
		studentID, store.LiveAt,
	).Scan(&grade, &major)
	if errors.Is(err, sql.ErrNoRows) {
		return studentProfile{}, apperr.NotFoundf().WithRule("PAR-064")
	}
	if err != nil {
		return studentProfile{}, internal(err)
	}
	return studentProfile{GradeLevel: uint8(grade), MajorID: uint32OrZero(major)}, nil
}

// registrationForActor looks a live registration up by the resolved identity.
func registrationForActor(ctx context.Context, q store.Querier, actor Actor, quizID int64) (registrationRow, bool, error) {
	switch {
	case actor.IsStudent():
		return readStudentRegistration(ctx, q, quizID, int64(actor.StudentID))
	case actor.IsAnonymous():
		return readAnonRegistration(ctx, q, quizID, int64(actor.IdentityID))
	default:
		return registrationRow{}, false, nil
	}
}

// lockRegistration takes the exclusive registration lock that serializes a
// start against a cancel and another start (Â§9.1, PAR-073, PAR-082).
func lockRegistration(ctx context.Context, tx *sql.Tx, id int64) error {
	var got int64
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM registrations
		WHERE id = ? AND deleted_at = ?
		FOR UPDATE`,
		id, store.LiveAt,
	).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.NotFoundf().WithRule("PAR-072")
	}
	if err != nil {
		return internal(err)
	}
	return nil
}

// refusalPath is the redirect target for a refused participation command, chosen
// so the flash it carries is actually rendered. A student has no intro page once
// the quiz leaves the open state (PAR-088), so a closed-quiz refusal lands on the
// history surface: the per-quiz page when a registration exists, otherwise the
// history list (the per-quiz page is a 404 without a registration, PAR-155).
// Anonymous viewers keep the intro, which stays readable on a closed quiz.
func refusalPath(actor Actor, quiz quizRow, hasReg bool) string {
	if !actor.IsStudent() || viewmodel.QuizState(quiz.State) == viewmodel.QuizOpen {
		return quizPath(uint32(quiz.ID))
	}
	if hasReg {
		return historyPath(uint32(quiz.ID))
	}
	return "/student/history"
}

// quizIntroModel builds the PAR-091 model inside the caller's read snapshot.
func (s *Service) quizIntroModel(ctx context.Context, tx *sql.Tx, actor Actor, quiz quizRow, reg registrationRow, hasReg bool, ticket *ticket) (viewmodel.QuizIntro, error) {
	questionCount, err := liveQuestionCount(ctx, tx, quiz.ID)
	if err != nil {
		return viewmodel.QuizIntro{}, err
	}

	records := []attemptRecord{}
	if hasReg {
		byReg, err := readAttemptsByRegistration(ctx, tx, []int64{reg.ID})
		if err != nil {
			return viewmodel.QuizIntro{}, err
		}
		records = byReg[reg.ID]
	}
	rows := s.buildAttemptRows(records, quiz.ShowScore)
	open := viewmodel.QuizState(quiz.State) == viewmodel.QuizOpen
	limit, normalised := maxAttemptsFor(quiz)
	if normalised {
		s.warn("PAR-081", "quiz carries an invalid attempt quota; failing closed at one", nil)
	}

	var (
		state       viewmodel.ParticipationState
		inProgress  *attemptRecord
		lastDone    *attemptRecord
		attemptsUse = len(records)
	)
	for i := range records {
		if records[i].Submitted.Valid {
			lastDone = &records[i]
		} else if inProgress == nil {
			inProgress = &records[i]
		}
	}
	if hasReg {
		state = deriveState(attemptsUse, inProgress != nil)
	}

	card := viewmodel.QuizCard{
		ID:            quiz.ID,
		Href:          quizPath(uint32(quiz.ID)),
		Title:         quiz.Title,
		Description:   quiz.Description,
		TeacherName:   quiz.TeacherName,
		QuestionCount: questionCount,
		State:         viewmodel.QuizState(quiz.State),
		Visibility:    viewmodel.Visibility(quiz.Visibility),
		Audience:      audienceLabel(quiz),
		StartsAt:      localPtr(quiz.StartsAt),
		EndsAt:        localPtr(quiz.EndsAt),
		Participation: state,
		AttemptsUsed:  attemptsUse,
		MaxAttempts:   limit,
		ReviewMode:    viewmodel.ReviewMode(quiz.ReviewMode),
		ShowScore:     quiz.ShowScore,
	}
	if viewmodel.Visibility(quiz.Visibility) == viewmodel.VisibilityPublic && quiz.Code.Valid && quiz.Code.String != "" {
		code := quiz.Code.String
		card.Code = &code
	}
	if hasReg {
		card.JoinedAt = localPtr(sql.NullTime{Time: reg.CreatedAt, Valid: true})
	}

	out := viewmodel.QuizIntro{
		Quiz:          card,
		Description:   quiz.Description,
		StartsAt:      card.StartsAt,
		EndsAt:        card.EndsAt,
		Participation: state,
		Attempts:      rows,
		AttemptsUsed:  attemptsUse,
		MaxAttempts:   limit,
		JoinedAt:      card.JoinedAt,
	}
	out.BackHref = "/join"
	if actor.IsStudent() {
		out.BackHref = "/student"
	}

	// PAR-092: eligibility is checked against the live profile for a student;
	// an anonymous identity is only ever inside a public quiz.
	eligibleNow := true
	if actor.IsStudent() {
		eligibleNow = eligible(quiz, actor.GradeLevel, actor.MajorID)
	} else if actor.IsAnonymous() {
		eligibleNow = viewmodel.Visibility(quiz.Visibility) == viewmodel.VisibilityPublic
	}
	if actor.IsStudent() && hasReg && open && !eligibleNow {
		e := viewmodel.JoinWrongAudience
		out.Error = &e
	}

	out.CanRegister = actor.IsStudent() && open && eligibleNow && !hasReg
	out.CanCancel = hasReg && open && attemptsUse == 0
	out.CanStart = hasReg && open && inProgress == nil && eligibleNow &&
		attemptsUse < limit && (attemptsUse == 0 || quiz.AllowRetake)
	out.CanRetry = out.CanStart && attemptsUse >= 1

	// PAR-092 precedence: resume â†’ register â†’ start/retry â†’ last result â†’
	// history. A resume is offered even on a closed quiz (PAR-080, R-10).
	switch {
	case inProgress != nil:
		action := viewmodel.Primary("Lanjutkan", attemptPath(uint32(inProgress.ID)))
		out.Primary = &action
	case out.CanRegister:
		action := viewmodel.Primary("Daftar", "/quiz/"+itoa(uint64(quiz.ID))+"/register")
		out.Primary = &action
	case out.CanStart:
		label := "Mulai"
		if attemptsUse > 0 {
			label = "Coba lagi"
		}
		action := viewmodel.Primary(label, "/quiz/"+itoa(uint64(quiz.ID))+"/attempts")
		out.Primary = &action
	case lastDone != nil:
		action := viewmodel.Outline("Lihat hasil", attemptResultPath(uint32(lastDone.ID)))
		out.Primary = &action
	case hasReg && actor.IsStudent():
		action := viewmodel.Outline("Riwayat", historyPath(uint32(quiz.ID)))
		out.Primary = &action
	}

	if out.CanCancel {
		action := viewmodel.Danger("Batalkan pendaftaran", "/quiz/"+itoa(uint64(quiz.ID))+"/cancel")
		out.Secondary = &action
	} else if actor.IsStudent() && hasReg && inProgress == nil && lastDone != nil {
		action := viewmodel.Outline("Riwayat", historyPath(uint32(quiz.ID)))
		if out.Primary == nil || out.Primary.Href != action.Href {
			out.Secondary = &action
		}
	}
	return out, nil
}

// QuizIntro serves GET /quiz/:id (PAR-087…PAR-095). A redirect is returned as
// *Redirect: the teacher owner goes to the authoring detail, a ticket holder
// goes to the identify form.
func (s *Service) QuizIntro(ctx context.Context, in QuizIntroInput) (viewmodel.QuizIntro, error) {
	var out viewmodel.QuizIntro
	if in.QuizID <= 0 {
		return out, apperr.NotFoundf().WithRule("PAR-087")
	}
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		actor, err := resolveActor(ctx, tx, in.Actor)
		if err != nil {
			return err
		}
		quiz, err := readQuizByID(ctx, tx, in.QuizID, "")
		if err != nil {
			return err
		}

		// A ticket is only meaningful for a viewer with no identity or no live
		// registration (PAR-016, Â§4.4). It never creates state here.
		var validTicket *ticket
		if in.Actor.TicketValue != "" {
			t, err := parseTicket(s.key, in.Actor.TicketValue, s.now())
			if err == nil && int64(t.QuizID) == quiz.ID &&
				viewmodel.Visibility(quiz.Visibility) == viewmodel.VisibilityPublic {
				validTicket = &t
			}
		}

		switch {
		case actor.IsTeacher():
			if quiz.OwnerID != int64(actor.Account) {
				return apperr.NotFoundf().WithRule("PAR-090")
			}
			return &Redirect{Path: "/teacher/quizzes/" + itoa(uint64(quiz.ID))}

		case actor.IsStudent():
			// PAR-088: a student has no entry page for a draft or a closed
			// quiz, registered or not. A closed quiz stays reachable through
			// the per-quiz history page, which is where an attempt that was
			// in progress at closing time resumes (ATT-104 step 4).
			if viewmodel.QuizState(quiz.State) != viewmodel.QuizOpen {
				return apperr.NotFoundf().WithRule("PAR-088")
			}
			reg, hasReg, err := readStudentRegistration(ctx, tx, quiz.ID, int64(actor.StudentID))
			if err != nil {
				return err
			}
			if !hasReg {
				if !eligible(quiz, actor.GradeLevel, actor.MajorID) {
					return apperr.NotFoundf().WithRule("PAR-088")
				}
			}
			out, err = s.quizIntroModel(ctx, tx, actor, quiz, reg, hasReg, validTicket)
			return err

		case actor.IsAnonymous():
			if viewmodel.QuizState(quiz.State) == viewmodel.QuizDraft {
				return apperr.NotFoundf().WithRule("PAR-089")
			}
			reg, hasReg, err := readAnonRegistration(ctx, tx, quiz.ID, int64(actor.IdentityID))
			if err != nil {
				return err
			}
			if !hasReg {
				if validTicket != nil {
					return &Redirect{Path: "/join/identify?quiz=" + itoa(uint64(quiz.ID))}
				}
				return apperr.NotFoundf().WithRule("PAR-089")
			}
			out, err = s.quizIntroModel(ctx, tx, actor, quiz, reg, true, validTicket)
			return err

		default:
			if validTicket != nil {
				return &Redirect{Path: "/join/identify?quiz=" + itoa(uint64(quiz.ID))}
			}
			return apperr.NotFoundf().WithRule("PAR-087")
		}
	})
	return out, err
}

// Register serves POST /quiz/:id/register (PAR-061…PAR-068). It is idempotent:
// an existing live registration is the already_registered info outcome.
func (s *Service) Register(ctx context.Context, in RegisterInput) (Result, error) {
	var res Result
	if in.QuizID <= 0 {
		return res, apperr.NotFoundf().WithRule("PAR-063")
	}
	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		actor, err := resolveActor(ctx, tx, in.Actor)
		if err != nil {
			return err
		}
		if !actor.IsStudent() {
			return apperr.NotFoundf().WithRule("PAR-061")
		}
		profile, err := lockStudentProfile(ctx, tx, actor.StudentID)
		if err != nil {
			return err
		}
		quiz, err := readQuizByID(ctx, tx, in.QuizID, " LOCK IN SHARE MODE")
		if err != nil {
			return err
		}
		if _, hasReg, err := readStudentRegistration(ctx, tx, quiz.ID, int64(actor.StudentID)); err != nil {
			return err
		} else if hasReg {
			res = Result{Path: refusalPath(actor, quiz, true), Flash: FlashAlreadyRegistered}
			return nil
		}
		if viewmodel.QuizState(quiz.State) != viewmodel.QuizOpen {
			return apperr.NotFoundf().WithRule("PAR-062")
		}
		if !eligible(quiz, profile.GradeLevel, profile.MajorID) {
			return apperr.NotFoundf().WithRule("PAR-062")
		}
		count, err := liveQuestionCount(ctx, tx, quiz.ID)
		if err != nil {
			return err
		}
		if count == 0 {
			res = Result{Path: quizPath(uint32(quiz.ID)), Flash: FlashQuizEmpty}
			return nil
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO registrations (quiz_id, student_id) VALUES (?, ?)`,
			quiz.ID, actor.StudentID)
		if store.IsDuplicate(err) {
			res = Result{Path: quizPath(uint32(quiz.ID)), Flash: FlashAlreadyRegistered}
			return nil
		}
		if err != nil {
			return internal(err)
		}
		res = Result{Path: quizPath(uint32(quiz.ID)), Flash: FlashRegistered}
		return nil
	})
	return res, err
}

// Cancel serves POST /quiz/:id/cancel (PAR-070…PAR-077). The registration is
// derived from the actor, never from the body. A quiz that is no longer open
// and a registration that already has attempts are legal but refused
// transitions, not errors.
func (s *Service) Cancel(ctx context.Context, in CancelInput) (Result, error) {
	var res Result
	if in.QuizID <= 0 {
		return res, apperr.NotFoundf().WithRule("PAR-071")
	}
	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		actor, err := resolveActor(ctx, tx, in.Actor)
		if err != nil {
			return err
		}
		if !actor.IsStudent() && !actor.IsAnonymous() {
			return apperr.NotFoundf().WithRule("PAR-070")
		}
		quiz, err := readQuizByID(ctx, tx, in.QuizID, " LOCK IN SHARE MODE")
		if err != nil {
			return err
		}
		if viewmodel.QuizState(quiz.State) == viewmodel.QuizDraft {
			return apperr.NotFoundf().WithRule("PAR-071")
		}
		reg, hasReg, err := registrationForActor(ctx, tx, actor, quiz.ID)
		if err != nil {
			return err
		}
		if !hasReg {
			return apperr.NotFoundf().WithRule("PAR-071")
		}
		if err := lockRegistration(ctx, tx, reg.ID); err != nil {
			return err
		}
		if viewmodel.QuizState(quiz.State) != viewmodel.QuizOpen {
			res = Result{Path: refusalPath(actor, quiz, true), Flash: FlashNotOpen}
			return nil
		}
		byReg, err := readAttemptsByRegistration(ctx, tx, []int64{reg.ID})
		if err != nil {
			return err
		}
		if len(byReg[reg.ID]) > 0 {
			res = Result{Path: refusalPath(actor, quiz, true), Flash: FlashCancelBlocked}
			return nil
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE registrations SET deleted_at = UTC_TIMESTAMP()
			WHERE id = ? AND deleted_at = ?`,
			reg.ID, store.LiveAt); err != nil {
			return internal(err)
		}
		target := quizPath(uint32(quiz.ID))
		if actor.IsAnonymous() {
			// PAR-075: the anonymous intro needs a live registration, so the
			// cancel lands on the join screen instead of a 404.
			target = "/join"
		}
		res = Result{Path: target, Flash: FlashCancelled}
		return nil
	})
	return res, err
}

// StartAttempt serves POST /quiz/:id/attempts (PAR-078…PAR-086). Resume wins
// over every guard; a fresh attempt is handed to ATT inside the same
// transaction, so PAR never inserts an attempt or answer row itself (PAR-202).
func (s *Service) StartAttempt(ctx context.Context, in StartQuizInput) (Result, error) {
	var res Result
	if in.QuizID <= 0 {
		return res, apperr.NotFoundf().WithRule("PAR-078")
	}
	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		actor, err := resolveActor(ctx, tx, in.Actor)
		if err != nil {
			return err
		}
		if !actor.IsStudent() && !actor.IsAnonymous() {
			return apperr.NotFoundf().WithRule("PAR-078")
		}
		// Lock order: participant â†’ quiz â†’ registration (Â§9.1).
		var profile studentProfile
		if actor.IsStudent() {
			profile, err = lockStudentProfile(ctx, tx, actor.StudentID)
			if err != nil {
				return err
			}
		}
		quiz, err := readQuizByID(ctx, tx, in.QuizID, " LOCK IN SHARE MODE")
		if err != nil {
			return err
		}
		if viewmodel.QuizState(quiz.State) == viewmodel.QuizDraft {
			return apperr.NotFoundf().WithRule("PAR-081")
		}
		if actor.IsAnonymous() &&
			viewmodel.Visibility(quiz.Visibility) != viewmodel.VisibilityPublic {
			return apperr.NotFoundf().WithRule("PAR-079")
		}
		reg, hasReg, err := registrationForActor(ctx, tx, actor, quiz.ID)
		if err != nil {
			return err
		}
		if !hasReg {
			if actor.IsStudent() {
				res = Result{Path: refusalPath(actor, quiz, false), Flash: FlashNotRegistered}
				return nil
			}
			return apperr.NotFoundf().WithRule("PAR-079")
		}
		if err := lockRegistration(ctx, tx, reg.ID); err != nil {
			return err
		}

		byReg, err := readAttemptsByRegistration(ctx, tx, []int64{reg.ID})
		if err != nil {
			return err
		}
		records := byReg[reg.ID]
		for i := range records {
			if !records[i].Submitted.Valid {
				// PAR-080: resume, with no write and no flash, even on a
				// closed quiz.
				res = Result{Path: attemptPath(uint32(records[i].ID))}
				return nil
			}
		}

		if viewmodel.QuizState(quiz.State) != viewmodel.QuizOpen {
			res = Result{Path: refusalPath(actor, quiz, true), Flash: FlashNotOpen}
			return nil
		}
		if actor.IsStudent() {
			if !eligible(quiz, profile.GradeLevel, profile.MajorID) {
				return apperr.NotFoundf().WithRule("PAR-086")
			}
		}
		count, err := liveQuestionCount(ctx, tx, quiz.ID)
		if err != nil {
			return err
		}
		if count == 0 {
			res = Result{Path: quizPath(uint32(quiz.ID)), Flash: FlashQuizEmpty}
			return nil
		}
		limit, normalised := maxAttemptsFor(quiz)
		if normalised {
			s.warn("PAR-081", "quiz carries an invalid attempt quota; failing closed at one", nil)
		}
		if len(records) >= limit || (len(records) > 0 && !quiz.AllowRetake) {
			res = Result{Path: quizPath(uint32(quiz.ID)), Flash: FlashAttemptsExhausted}
			return nil
		}
		if s.AttemptStart == nil {
			return internal(errors.New("participation: ATT attempt-start seam is not configured"))
		}
		plan := StartPlan{
			RegistrationID: uint32(reg.ID),
			QuizID:         uint32(quiz.ID),
			AttemptNo:      len(records) + 1,
			StudentID:      actor.StudentID,
		}
		if actor.IsAnonymous() {
			plan.AnonymousParticipantID = actor.IdentityID
			if _, err := tx.ExecContext(ctx, `
				UPDATE anonymous_participants SET last_seen_at = UTC_TIMESTAMP()
				WHERE id = ? AND deleted_at = ?`,
				actor.IdentityID, store.LiveAt); err != nil {
				return internal(err)
			}
		}
		attemptID, err := s.AttemptStart(ctx, tx, plan)
		if err != nil {
			return err
		}
		if attemptID == 0 {
			return internal(errors.New("participation: ATT attempt-start returned no attempt id"))
		}
		res = Result{Path: attemptPath(attemptID), Flash: FlashStarted}
		return nil
	})
	return res, err
}
