package participation

// join.go implements the entry half of the PAR domain: the code-entry screen
// (PAR-021…PAR-026), code resolution (PAR-027…PAR-035), the invitation landing
// with its student auto-registration side effect (PAR-036…PAR-044), and the
// anonymous identity form with its create/reuse command (PAR-045…PAR-060).
//
// The join ticket is the only capability these routes mint. Nothing on a GET
// creates an identity except the student auto-registration, which goes through
// the same transaction as POST /quiz/:id/register (PAR-069).

import (
	"context"
	"database/sql"
	"errors"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// ViewerResult is the resolved viewer of a public GET plus the cookie delta the
// adapter must apply. A stale identity cookie is cleared on the next GET
// (PAR-006); the request still renders as a visitor.
type ViewerResult struct {
	Actor   Actor
	Cookies Cookies
}

// Viewer resolves the request's participant class without writing any row. It
// exists for the two public GET screens that must react to a stale identity
// cookie and to the teacher redirect of PAR-021.
func (s *Service) Viewer(ctx context.Context, in ActorInput) (ViewerResult, error) {
	var out ViewerResult
	if s.db == nil {
		return out, internal(errors.New("participation: no database"))
	}
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		actor, err := resolveActor(ctx, tx, in)
		if err != nil {
			return err
		}
		out.Actor = actor
		if actor.IdentityStale {
			out.Cookies.IdentityClear = true
		}
		return nil
	})
	return out, err
}

// ResolveCodeInput is POST /join material: the raw submitted code and the raw
// viewer identity. The code is trimmed only of ASCII whitespace (PAR-028).
type ResolveCodeInput struct {
	Actor ActorInput
	Code  string
}

// ResolveCode serves POST /join (PAR-027…PAR-035). Every outcome is a 303: an
// unresolved code returns to the join screen with its closed error key, a
// student lands on the intro, and a visitor/anonymous viewer receives a ticket
// and is sent to the identify form.
func (s *Service) ResolveCode(ctx context.Context, in ResolveCodeInput) (Out, error) {
	var out Out
	if s.db == nil {
		return out, internal(errors.New("participation: no database"))
	}
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		actor, err := resolveActor(ctx, tx, in.Actor)
		if err != nil {
			return err
		}
		// A teacher never resolves a participant entry (PAR-027, R-05).
		if actor.IsTeacher() {
			out.Result = Result{Path: "/teacher"}
			return nil
		}

		code := trimCode(in.Code)
		if !validCode(code) {
			out.Result = Result{Path: "/join?error=unknown_code"}
			return nil
		}
		quiz, ok, err := readQuizByCode(ctx, tx, code)
		if err != nil {
			return err
		}
		if !ok {
			out.Result = Result{Path: "/join?error=unknown_code"}
			return nil
		}
		// A private quiz cannot carry a code (INV-07); treat corruption as
		// not-found and log it rather than confirming the row exists.
		if viewmodel.Visibility(quiz.Visibility) != viewmodel.VisibilityPublic {
			s.oops("PAR-030", "public code resolved to a non-public quiz", nil)
			out.Result = Result{Path: "/join?error=unknown_code"}
			return nil
		}
		if viewmodel.QuizState(quiz.State) != viewmodel.QuizOpen {
			out.Result = Result{Path: "/join?error=not_open"}
			return nil
		}

		_, hasReg, err := registrationForActor(ctx, tx, actor, quiz.ID)
		if err != nil {
			return err
		}
		if hasReg {
			out.Result = Result{Path: quizPath(uint32(quiz.ID)), Flash: FlashAlreadyRegistered}
			return nil
		}
		if actor.IsStudent() {
			// Codes do not auto-register students (R-12): the intro offers the
			// explicit register command.
			out.Result = Result{Path: quizPath(uint32(quiz.ID))}
			return nil
		}

		ticketValue, err := mintTicket(s.key, uint32(quiz.ID), MethodCode, code, s.now())
		if err != nil {
			return internal(err)
		}
		out.Cookies.TicketValue = ticketValue
		out.Result = Result{Path: "/join/identify?quiz=" + itoa(uint64(quiz.ID))}
		return nil
	})
	return out, err
}

// InviteInput is GET /invite/:token material.
type InviteInput struct {
	Actor ActorInput
	Token string
}

// InviteResult is the landing page plus the cookie delta. A non-empty Page and a
// non-nil error never coexist except for the redirect error below.
type InviteResult struct {
	Page    viewmodel.InvitePage
	Cookies Cookies
}

// Invite serves GET /invite/:token (PAR-036…PAR-044). A student with no live
// registration is auto-registered when the quiz is open and they are eligible;
// every negative outcome is a 200 landing page because the page is the
// explanation (F-9.6).
func (s *Service) Invite(ctx context.Context, in InviteInput) (InviteResult, error) {
	var out InviteResult
	if s.db == nil {
		return out, internal(errors.New("participation: no database"))
	}

	var (
		redirect     *Redirect
		autoRegister bool
		resolved     quizRow
	)
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		actor, err := resolveActor(ctx, tx, in.Actor)
		if err != nil {
			return err
		}
		if actor.IdentityStale {
			out.Cookies.IdentityClear = true
		}

		if actor.IsTeacher() {
			if !validInvite(in.Token) {
				return apperr.NotFoundf().WithRule("PAR-041")
			}
			quiz, ok, err := readQuizByInvite(ctx, tx, in.Token)
			if err != nil {
				return err
			}
			if !ok || quiz.OwnerID != int64(actor.Account) {
				return apperr.NotFoundf().WithRule("PAR-041")
			}
			redirect = &Redirect{Path: "/teacher/quizzes/" + itoa(uint64(quiz.ID))}
			return nil
		}

		if !validInvite(in.Token) {
			out.Page = inviteLanding(in.Token, nil, viewmodel.JoinUnknownCode)
			return nil
		}
		quiz, ok, err := readQuizByInvite(ctx, tx, in.Token)
		if err != nil {
			return err
		}
		if !ok {
			out.Page = inviteLanding(in.Token, nil, viewmodel.JoinUnknownCode)
			return nil
		}
		resolved = quiz

		// PAR-038(a): a live registration wins over every state check.
		_, hasReg, err := registrationForActor(ctx, tx, actor, quiz.ID)
		if err != nil {
			return err
		}
		if hasReg {
			redirect = &Redirect{Path: quizPath(uint32(quiz.ID)), Flash: FlashAlreadyRegistered}
			return nil
		}

		if viewmodel.QuizState(quiz.State) != viewmodel.QuizOpen {
			out.Page = inviteLanding(in.Token, &quiz, viewmodel.JoinNotOpen)
			return nil
		}

		switch {
		case actor.IsStudent():
			if !eligible(quiz, actor.GradeLevel, actor.MajorID) {
				out.Page = inviteLanding(in.Token, &quiz, viewmodel.JoinWrongAudience)
				return nil
			}
			autoRegister = true

		case actor.IsAnonymous() || actor.IsVisitor():
			if viewmodel.Visibility(quiz.Visibility) != viewmodel.VisibilityPublic {
				out.Page = inviteLanding(in.Token, &quiz, viewmodel.JoinAnonymousNotAllowed)
				return nil
			}
			ticketValue, err := mintTicket(s.key, uint32(quiz.ID), MethodInvite, in.Token, s.now())
			if err != nil {
				return internal(err)
			}
			out.Cookies.TicketValue = ticketValue
			redirect = &Redirect{Path: "/join/identify?quiz=" + itoa(uint64(quiz.ID))}

		default: // teachers were handled above; no other actor exists.
			return apperr.NotFoundf().WithRule("PAR-036")
		}
		return nil
	})
	if err != nil {
		return out, err
	}

	if autoRegister {
		// PAR-069 reuses the register transaction verbatim; the only
		// differences are the success flash and target, which Register
		// already produces for the same quiz.
		res, regErr := s.Register(ctx, RegisterInput{QuizID: resolved.ID, Actor: in.Actor})
		if regErr != nil {
			if apperr.IsNotFound(regErr) {
				// The quiz closed or the profile vanished between the landing
				// read and the registration lock: answer with the landing page
				// that explains the now-current state (8.1.1).
				return s.inviteStale(ctx, in.Token, resolved.ID, out)
			}
			return out, regErr
		}
		redirect = &Redirect{Path: res.Path, Flash: res.Flash}
	}
	if redirect != nil {
		return out, redirect
	}
	return out, nil
}

// inviteStale re-reads a quiz after a failed auto-registration race and picks
// the landing explanation. A vanished quiz is unknown_code; a closed one is
// not_open; an open one is wrong_audience (the only remaining 404 reason is a
// changed profile).
func (s *Service) inviteStale(ctx context.Context, token string, quizID int64, out InviteResult) (InviteResult, error) {
	quiz, err := readQuizByID(ctx, s.db.Querier(), quizID, "")
	if err != nil {
		if apperr.IsNotFound(err) {
			out.Page = inviteLanding(token, nil, viewmodel.JoinUnknownCode)
			return out, nil
		}
		return out, err
	}
	if viewmodel.QuizState(quiz.State) != viewmodel.QuizOpen {
		out.Page = inviteLanding(token, &quiz, viewmodel.JoinNotOpen)
		return out, nil
	}
	out.Page = inviteLanding(token, &quiz, viewmodel.JoinWrongAudience)
	return out, nil
}

// inviteLanding builds the one landing model. quiz is nil only for an unknown
// token; outcome is empty for the (unreachable in the current UI) valid landing.
func inviteLanding(token string, quiz *quizRow, outcome viewmodel.JoinError) viewmodel.InvitePage {
	p := viewmodel.InvitePage{
		Token:         token,
		LoginHref:     "/login",
		DashboardHref: "/student",
		JoinHref:      "/join",
	}
	if quiz != nil {
		ref := viewmodel.QuizRef{ID: quiz.ID, Title: quiz.Title}
		p.Quiz = &ref
		p.Audience = audienceLabel(*quiz)
		p.JoinHref = "/join/identify?quiz=" + itoa(uint64(quiz.ID))
	}
	if outcome != "" {
		e := outcome
		p.Outcome = &e
	}
	return p
}

// IdentifyPageInput is GET /join/identify material. QuizID is the untrusted
// ?quiz= value; Switch is the PAR-052 cookie escape hatch.
type IdentifyPageInput struct {
	Actor  ActorInput
	QuizID int64
	Switch bool
}

// IdentifyPageResult is the rendered form plus the cookie delta.
type IdentifyPageResult struct {
	Page    viewmodel.IdentifyPage
	Cookies Cookies
}

// IdentifyPage serves GET /join/identify (PAR-045…PAR-052).
func (s *Service) IdentifyPage(ctx context.Context, in IdentifyPageInput) (IdentifyPageResult, error) {
	var out IdentifyPageResult
	if s.db == nil {
		return out, internal(errors.New("participation: no database"))
	}
	var redirect *Redirect
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		actor, err := resolveActor(ctx, tx, in.Actor)
		if err != nil {
			return err
		}
		if actor.IdentityStale {
			out.Cookies.IdentityClear = true
		}
		switch {
		case actor.IsTeacher():
			return apperr.NotFoundf().WithRule("PAR-045")
		case actor.IsStudent():
			// A student has no anonymous form: the ticket only chooses the
			// intro the redirect lands on (PAR-045).
			if t, ok := s.parseTicket(in.Actor.TicketValue); ok {
				redirect = &Redirect{Path: quizPath(t.QuizID)}
			} else {
				redirect = &Redirect{Path: "/join"}
			}
			return nil
		}

		t, err := s.ticketFor(ctx, tx, in.Actor.TicketValue)
		if err != nil {
			return err
		}
		if in.Switch && actor.IsAnonymous() {
			// PAR-052: clear only the caller's identity cookie; the row and its
			// attempts stay visible to teachers. The ticket must be rotated in
			// the same response (PAR-019, PAR-020): the identity token is
			// derived from the ticket nonce, so leaving the old ticket in place
			// would derive the same token again and silently reuse the old
			// identity on submit. Overwriting the ticket invalidates the old
			// value while keeping the identify form reachable.
			rotated, err := mintTicket(s.key, t.QuizID, t.Method, t.Secret, s.now())
			if err != nil {
				return internal(err)
			}
			out.Cookies.IdentityClear = true
			out.Cookies.TicketValue = rotated
			redirect = &Redirect{
				Path:  "/join/identify?quiz=" + itoa(uint64(t.QuizID)),
				Flash: FlashIdentitySwitched,
			}
			return nil
		}
		if in.QuizID != int64(t.QuizID) {
			redirect = &Redirect{Path: "/join/identify?quiz=" + itoa(uint64(t.QuizID))}
			return nil
		}

		quiz, err := readQuizByID(ctx, tx, int64(t.QuizID), "")
		if err != nil {
			return err
		}
		if viewmodel.QuizState(quiz.State) != viewmodel.QuizOpen {
			if actor.IsAnonymous() {
				_, has, err := readAnonRegistration(ctx, tx, quiz.ID, int64(actor.IdentityID))
				if err != nil {
					return err
				}
				if has {
					redirect = &Redirect{Path: quizPath(uint32(quiz.ID))}
					return nil
				}
			}
			out.Cookies.TicketClear = true
			redirect = &Redirect{Path: "/join?error=not_open"}
			return nil
		}

		majors, err := readMajors(ctx, tx)
		if err != nil {
			return err
		}
		page := identifyPageModel(quiz, majors, "", 0, 0)
		if actor.IsAnonymous() {
			page.FullName = actor.DisplayName
			page.GradeLevel = int(actor.IdentityGrade)
			page.MajorID = int64(actor.IdentityMajor)
		}
		out.Page = page
		return nil
	})
	if err != nil {
		return out, err
	}
	if redirect != nil {
		return out, redirect
	}
	return out, nil
}

// IdentifyInput is POST /join/identify material.
type IdentifyInput struct {
	QuizID int64
	Actor  ActorInput
	Form   IdentityForm
}

// IdentifyResult is the form page for a 422 plus the command outcome for a 303.
// Exactly one is populated.
type IdentifyResult struct {
	Page viewmodel.IdentifyPage
	Out  Out
}

// Identify serves POST /join/identify (PAR-053…PAR-060). It validates before
// taking any lock, then re-reads and re-locks the identity, major, and quiz in
// the canonical order before writing the identity profile and registration.
func (s *Service) Identify(ctx context.Context, in IdentifyInput) (IdentifyResult, error) {
	var out IdentifyResult
	if s.db == nil {
		return out, internal(errors.New("participation: no database"))
	}
	actor, err := resolveActor(ctx, s.db.Querier(), in.Actor)
	if err != nil {
		return out, err
	}
	switch {
	case actor.IsTeacher():
		return out, apperr.NotFoundf().WithRule("PAR-053")
	case actor.IsStudent():
		if t, ok := s.parseTicket(in.Actor.TicketValue); ok {
			return out, &Redirect{Path: quizPath(t.QuizID)}
		}
		return out, &Redirect{Path: "/join"}
	}

	t, ok := s.parseTicket(in.Actor.TicketValue)
	if !ok || in.QuizID <= 0 || int64(t.QuizID) != in.QuizID {
		return out, apperr.NotFoundf().WithRule("PAR-054")
	}
	quiz, ok, err := s.quizForTicket(ctx, s.db.Querier(), t)
	if err != nil {
		return out, err
	}
	if !ok || quiz.ID != in.QuizID {
		return out, apperr.NotFoundf().WithRule("PAR-054")
	}
	if viewmodel.Visibility(quiz.Visibility) != viewmodel.VisibilityPublic {
		s.oops("PAR-054", "join ticket resolved to a non-public quiz", nil)
		return out, apperr.NotFoundf().WithRule("PAR-054")
	}

	majors, err := readMajors(ctx, s.db.Querier())
	if err != nil {
		return out, err
	}

	name, nameMsg := validateName(in.Form.FullName)
	grade, gradeMsg := validateGrade(in.Form.GradeLevel)
	majorID, majorMsg := validateMajorID(in.Form.MajorID)
	out.Page = identifyPageModel(quiz, majors, name, int(grade), majorID)

	// PAR-057: a closed quiz with no live registration refuses before the
	// profile write; with one, the profile may still be refreshed.
	if viewmodel.QuizState(quiz.State) != viewmodel.QuizOpen {
		hasReg := false
		if actor.IsAnonymous() {
			_, hasReg, err = readAnonRegistration(ctx, s.db.Querier(), quiz.ID, int64(actor.IdentityID))
			if err != nil {
				return out, err
			}
		}
		if !hasReg {
			out.Out.Result = Result{Path: "/join?error=not_open"}
			out.Out.Cookies.TicketClear = true
			return out, nil
		}
	}

	fields := map[string]string{}
	if nameMsg != "" {
		fields["full_name"] = nameMsg
	}
	if gradeMsg != "" {
		fields["grade_level"] = gradeMsg
	}
	if majorMsg != "" {
		fields["major_id"] = majorMsg
	}
	if len(fields) > 0 {
		return out, apperr.Validationf(fields).WithRule("PAR-055")
	}
	if _, ok, err := readMajorByID(ctx, s.db.Querier(), majorID, ""); err != nil {
		return out, err
	} else if !ok {
		return out, apperr.Field("major_id", ErrMajor).WithRule("PAR-055")
	}

	if err := s.identifyCommit(ctx, in, t, name, grade, majorID, &out); err != nil {
		return out, err
	}
	return out, nil
}

// identifyCommit is the locked write half of PAR-056. It re-resolves the actor
// and re-verifies the ticket under the quiz lock, so rotation, closure, or an
// identity deletion between validation and commit is caught before any write.
func (s *Service) identifyCommit(ctx context.Context, in IdentifyInput, t ticket, name string, grade uint8, majorID int64, out *IdentifyResult) error {
	return s.writeTx(ctx, func(tx *sql.Tx) error {
		actor, err := resolveActor(ctx, tx, in.Actor)
		if err != nil {
			return err
		}
		switch {
		case actor.IsTeacher():
			return apperr.NotFoundf().WithRule("PAR-053")
		case actor.IsStudent():
			return &Redirect{Path: quizPath(t.QuizID)}
		}

		// (1) identity root: exclusive-lock it when the cookie still resolves.
		identityID := int64(0)
		token := ""
		if actor.IsAnonymous() {
			var got int64
			err := tx.QueryRowContext(ctx, `
				SELECT id FROM anonymous_participants
				WHERE id = ? AND deleted_at = ?
				FOR UPDATE`,
				actor.IdentityID, store.LiveAt).Scan(&got)
			switch {
			case err == nil:
				identityID, token = got, actor.IdentityToken
			case errors.Is(err, sql.ErrNoRows):
				// Deleted between resolution and lock: proceed as a new
				// identity derived from the ticket (PAR-011).
			default:
				return internal(err)
			}
		}

		// (2) major shared lock: a delete cannot slide under the write.
		if _, ok, err := readMajorByID(ctx, tx, majorID, " LOCK IN SHARE MODE"); err != nil {
			return err
		} else if !ok {
			return apperr.Field("major_id", ErrMajor).WithRule("PAR-055")
		}

		// (3) quiz shared lock and the current state/secret.
		quiz, err := readQuizByID(ctx, tx, int64(t.QuizID), " LOCK IN SHARE MODE")
		if err != nil {
			return err
		}
		if viewmodel.Visibility(quiz.Visibility) != viewmodel.VisibilityPublic {
			s.oops("PAR-054", "join ticket quiz stopped being public", nil)
			return apperr.NotFoundf().WithRule("PAR-054")
		}
		if !ticketMatchesSecret(quiz, t) {
			return apperr.NotFoundf().WithRule("PAR-016")
		}

		open := viewmodel.QuizState(quiz.State) == viewmodel.QuizOpen
		if open {
			count, err := liveQuestionCount(ctx, tx, quiz.ID)
			if err != nil {
				return err
			}
			if count == 0 {
				// Keep the ticket so the teacher can fix the data (PAR-057).
				out.Out.Result = Result{
					Path:  "/join/identify?quiz=" + itoa(uint64(t.QuizID)),
					Flash: FlashQuizEmpty,
				}
				return nil
			}
		} else {
			// A closed quiz may only refresh the profile of a participant who
			// already has a live registration; no identity is created here.
			if identityID == 0 {
				out.Out.Result = Result{Path: "/join?error=not_open"}
				out.Out.Cookies.TicketClear = true
				return nil
			}
			if _, has, err := readAnonRegistration(ctx, tx, quiz.ID, identityID); err != nil {
				return err
			} else if !has {
				out.Out.Result = Result{Path: "/join?error=not_open"}
				out.Out.Cookies.TicketClear = true
				return nil
			}
		}

		// (4) identity token. A live token never rotates (PAR-009); a new one
		// is derived from the ticket nonce so a double submit converges on
		// uq_anon_cookie (PAR-019, PAR-060).
		if token == "" {
			token = newIdentityToken(s.key, t.Nonce)
		}
		if identityID == 0 {
			identityID, err = s.insertIdentity(ctx, tx, name, grade, majorID, secure.HashToken(token))
			if err != nil {
				return err
			}
		}
		// Last writer wins between two identify submits and between a teacher
		// edit and an identify submit (9.2.2, 9.3.6).
		if _, err := tx.ExecContext(ctx, `
			UPDATE anonymous_participants
			SET display_name = ?, grade_level = ?, major_id = ?,
			    last_seen_at = UTC_TIMESTAMP()
			WHERE id = ? AND deleted_at = ?`,
			name, grade, majorID, identityID, store.LiveAt); err != nil {
			return internal(err)
		}

		// (5) registration. A closed quiz lands read-only with no flash; an
		// open one reports the new row or the idempotent info outcome.
		flash := ""
		if open {
			_, has, err := readAnonRegistration(ctx, tx, quiz.ID, identityID)
			if err != nil {
				return err
			}
			if has {
				flash = FlashAlreadyRegistered
			} else {
				inserted := false
				for attempt := 0; attempt < 2 && !inserted; attempt++ {
					_, err := tx.ExecContext(ctx, `
						INSERT INTO registrations (quiz_id, anonymous_participant_id)
						VALUES (?, ?)`, quiz.ID, identityID)
					switch {
					case err == nil:
						flash = FlashRegistered
						inserted = true
					case store.IsDuplicate(err):
						_, has, rerr := readAnonRegistration(ctx, tx, quiz.ID, identityID)
						if rerr != nil {
							return rerr
						}
						if has {
							flash = FlashAlreadyRegistered
							inserted = true
						}
						// No live row: the winner was cancelled between the
						// insert and the select, so retry once (9.2.1).
					default:
						return internal(err)
					}
				}
				if !inserted {
					return internal(errors.New("participation: registration insert kept colliding"))
				}
			}
		}

		out.Out.Result = Result{Path: quizPath(uint32(quiz.ID)), Flash: flash}
		out.Out.Cookies.IdentityToken = token
		out.Out.Cookies.TicketClear = true
		return nil
	})
}

// insertIdentity inserts a new anonymous identity and returns its id. A lost
// uq_anon_cookie race selects the winner's row under lock; if that row was
// deleted in between, the insert is retried once (9.2.2).
func (s *Service) insertIdentity(ctx context.Context, tx *sql.Tx, name string, grade uint8, majorID int64, hash []byte) (int64, error) {
	const insert = `
		INSERT INTO anonymous_participants
			(display_name, grade_level, major_id, cookie_token_hash, last_seen_at)
		VALUES (?, ?, ?, ?, UTC_TIMESTAMP())`
	res, err := tx.ExecContext(ctx, insert, name, grade, majorID, hash)
	if err == nil {
		return res.LastInsertId()
	}
	if !store.IsDuplicate(err) {
		return 0, internal(err)
	}
	var id int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM anonymous_participants
		WHERE cookie_token_hash = ? AND deleted_at = ?
		FOR UPDATE`,
		hash, store.LiveAt).Scan(&id)
	switch {
	case err == nil:
		return id, nil
	case !errors.Is(err, sql.ErrNoRows):
		return 0, internal(err)
	}
	res, err = tx.ExecContext(ctx, insert, name, grade, majorID, hash)
	if err != nil {
		return 0, internal(err)
	}
	return res.LastInsertId()
}

// parseTicket is the boolean wrapper around the token rule (PAR-016).
func (s *Service) parseTicket(raw string) (ticket, bool) {
	t, err := parseTicket(s.key, raw, s.now())
	return t, err == nil
}

// ticketFor parses one ticket and proves its secret still resolves to the same
// public quiz (PAR-016). Every failure is one 404.
func (s *Service) ticketFor(ctx context.Context, q store.Querier, raw string) (ticket, error) {
	t, ok := s.parseTicket(raw)
	if !ok {
		return ticket{}, apperr.NotFoundf().WithRule("PAR-016")
	}
	quiz, ok, err := s.quizForTicket(ctx, q, t)
	if err != nil {
		return ticket{}, err
	}
	if !ok || quiz.ID != int64(t.QuizID) {
		return ticket{}, apperr.NotFoundf().WithRule("PAR-016")
	}
	if viewmodel.Visibility(quiz.Visibility) != viewmodel.VisibilityPublic {
		s.oops("PAR-016", "join ticket resolved to a non-public quiz", nil)
		return ticket{}, apperr.NotFoundf().WithRule("PAR-016")
	}
	return t, nil
}

// quizForTicket resolves the ticket's secret by its method.
func (s *Service) quizForTicket(ctx context.Context, q store.Querier, t ticket) (quizRow, bool, error) {
	switch t.Method {
	case MethodCode:
		return readQuizByCode(ctx, q, t.Secret)
	case MethodInvite:
		return readQuizByInvite(ctx, q, t.Secret)
	default:
		return quizRow{}, false, nil
	}
}

// ticketMatchesSecret re-checks the locked quiz row against the ticket payload
// (PAR-016, 8.5.4).
func ticketMatchesSecret(q quizRow, t ticket) bool {
	switch t.Method {
	case MethodCode:
		return q.Code.Valid && q.Code.String == t.Secret
	case MethodInvite:
		return q.InviteToken == t.Secret
	default:
		return false
	}
}

// identifyPageModel builds the one identify form model shared by GET and 422.
func identifyPageModel(quiz quizRow, majors []viewmodel.MajorRef, name string, grade int, majorID int64) viewmodel.IdentifyPage {
	return viewmodel.IdentifyPage{
		Quiz:       viewmodel.QuizRef{ID: quiz.ID, Title: quiz.Title},
		FullName:   name,
		GradeLevel: grade,
		MajorID:    majorID,
		Majors:     majors,
	}
}
