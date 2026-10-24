package httpserver

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/attempts"
	"quiz-v2/internal/httpserver/fixtures"
	"quiz-v2/internal/participation"
	"quiz-v2/internal/viewmodel"
)

// This file is the "attempt" route group: the live quiz-taking flow
// (question, review, submit, result). These pages render through
// layouts/base.html (the pages/attempt/ path convention) — no teacher or
// student chrome, the attempt surfaces carry their own top bar.
//
// Load/ActForm/Act are the production handlers (spec 04); Build/Action remain
// the APP_DEMO fixture world, which serveRoute prefers when demo is on.

// attemptQueryPosition reads ?q= (1-based). Zero means "no explicit position"
// so the fixture can resume at the first unanswered question. Invalid input
// follows the same path; non-positive values clamp to question 1.
func attemptQueryPosition(r PageRequest) int {
	raw, ok := r.Query["q"]
	if !ok || raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	if n < 1 {
		return 1
	}
	return n
}

// attemptAnswerAction is the UI-only save-and-advance action. The form posts
// the current question id and a closed nav key; the redirect target is derived
// from ?q= on the action URL. "next" on the last question opens the review.
func attemptAnswerAction(r PageRequest) string {
	id := r.IntParam("id")
	position, _ := strconv.Atoi(r.Query["q"])
	if position < 1 {
		position, _ = strconv.Atoi(r.Form.Get("position"))
	}
	if position < 1 {
		position = 1
	}

	data := fixtures.AttemptQuestionData(id, position)
	total := data.Total
	if total < 1 {
		total = 1
	}
	if position > total {
		position = total
	}

	base := "/attempt/" + r.Param("id")
	nav := r.Form.Get("nav")
	if nav == "" {
		nav = r.Form.Get("action")
	}
	switch nav {
	case "prev":
		if position <= 2 {
			return base
		}
		return base + "?q=" + strconv.Itoa(position-1)
	case "save":
		return base + "?q=" + strconv.Itoa(position) + "&flash=saved"
	case "autosave":
		// Automatic commit from the question page: stay on the question and
		// stay silent — the live answered count in the header is the feedback.
		return base + "?q=" + strconv.Itoa(position)
	case "review":
		return base + "/review"
	default: // next
		if position >= total {
			return base + "/review"
		}
		return base + "?q=" + strconv.Itoa(position+1)
	}
}

// attemptBeacon is POST /attempt/:id/beacon (ATT-156..ATT-160). It is the only
// POST without a CSRF token; the pipeline already enforced same-origin. The
// response contract is empty bodies: 204 success, 400 malformed, 403 origin
// (pipeline), 404 foreign, 429 with Retry-After, 500 store failure.
func attemptBeacon(x *Ctx) error {
	// Demo mode has no live attempt to protect. The conformance suite still
	// exercises this route, and the fixture world answers its 204.
	if x.Demo || x.DB == nil {
		x.c.Status(fiber.StatusNoContent)
		return nil
	}
	kind, ok := attempts.ParseBeacon(x.c.ContentType(), x.c.Body())
	if !ok {
		x.c.Status(fiber.StatusBadRequest)
		return nil
	}
	_, err := attemptService(x).WithLimiter(x.Limiter()).Beacon(x.c.Context(), attempts.BeaconInput{
		AttemptID: attemptIDParam(x),
		Owner:     attemptOwner(x),
		Kind:      kind,
	})
	if err != nil {
		switch {
		case errors.Is(err, attempts.ErrNotFound):
			x.c.Status(fiber.StatusNotFound)
			return nil
		case errors.Is(err, attempts.ErrBeaconKind):
			x.c.Status(fiber.StatusBadRequest)
			return nil
		}
		var appErr *apperr.Error
		if errors.As(err, &appErr) {
			switch appErr.Kind() {
			case apperr.NotFound:
				x.c.Status(fiber.StatusNotFound)
			case apperr.RateLimited:
				x.c.Set("Retry-After", "60")
				x.c.Status(fiber.StatusTooManyRequests)
			default:
				x.c.Status(fiber.StatusInternalServerError)
			}
			return nil
		}
		x.c.Status(fiber.StatusInternalServerError)
		return nil
	}
	x.c.Status(fiber.StatusNoContent)
	return nil
}

// attemptService builds the per-request domain service (it holds no request
// state; a nil pool is answered as 500 by every method).
func attemptService(x *Ctx) *attempts.Service { return attempts.New(x.DB) }

// attemptOwner is the ATT-024 actor material: the validated session (when any)
// plus the raw participant identity cookie. The domain re-resolves both inside
// the transaction that needs them, so a stale summary cannot pass an ownership
// check.
func attemptOwner(x *Ctx) attempts.OwnerRef {
	ref := attempts.OwnerRef{IdentityToken: x.Cookie(participation.IdentityCookie)}
	if x.Session != nil {
		ref.Role = x.Session.Role
		ref.AccountID = x.Session.AccountID
	}
	return ref
}

// attemptIDParam parses :id. A malformed id becomes 0, which the domain
// resolves as a missing attempt (404), never as a panic.
func attemptIDParam(x *Ctx) uint32 {
	n, err := strconv.ParseUint(x.Param("id"), 10, 32)
	if err != nil {
		return 0
	}
	return uint32(n)
}

// attemptRedirect converts a domain redirect or conflict signal into the
// transport redirect. Conflicts always land on the result page
// (ATT-020, ATT-126).
func attemptRedirect(x *Ctx, err error) (Redirect, bool) {
	if sig, ok := attempts.AsRedirect(err); ok {
		return x.Redirect(sig.Path, sig.Flash), true
	}
	if key, ok := attempts.ConflictKey(err); ok {
		return x.Redirect("/attempt/"+x.Param("id")+"/result", key), true
	}
	return Redirect{}, false
}

// attemptLoadError maps a read-side signal to a 303 and passes every other
// error through to the generic error pages.
func attemptLoadError(x *Ctx, err error) error {
	if r, ok := attemptRedirect(x, err); ok {
		return x.SeeOther(r.Path, r.Flash)
	}
	return err
}

// attemptTitle builds the page title the attempt chrome uses.
func attemptTitle(prefix, quizTitle string) string {
	if quizTitle != "" {
		return prefix + " · " + quizTitle
	}
	return prefix
}

// attemptQuestionPageData wraps the question screen in the shared chrome.
func attemptQuestionPageData(x *Ctx, data attempts.QuestionPage) viewmodel.PageData {
	return x.Page(attemptTitle("Mengerjakan kuis", data.Header.QuizTitle), "", data)
}

// attemptQuestionLoad serves GET /attempt/:id (ATT-111).
func attemptQuestionLoad(x *Ctx) (viewmodel.PageData, error) {
	data, err := attemptService(x).QuestionPage(x.Context(), attempts.QuestionInput{
		AttemptID: attemptIDParam(x),
		QRaw:      x.QueryValue("q"),
		Owner:     attemptOwner(x),
	})
	if err != nil {
		return viewmodel.PageData{}, attemptLoadError(x, err)
	}
	return attemptQuestionPageData(x, data), nil
}

// attemptReviewLoad serves GET /attempt/:id/review (ATT-131).
func attemptReviewLoad(x *Ctx) (viewmodel.PageData, error) {
	data, err := attemptService(x).ReviewPage(x.Context(), attempts.ReviewInput{
		AttemptID: attemptIDParam(x),
		Owner:     attemptOwner(x),
	})
	if err != nil {
		return viewmodel.PageData{}, attemptLoadError(x, err)
	}
	return x.Page(attemptTitle("Tinjau jawaban", data.Header.QuizTitle), "", data), nil
}

// attemptResultLoad serves GET /attempt/:id/result (ATT-151).
func attemptResultLoad(x *Ctx) (viewmodel.PageData, error) {
	data, err := attemptService(x).ResultPage(x.Context(), attempts.ResultInput{
		AttemptID: attemptIDParam(x),
		Owner:     attemptOwner(x),
	})
	if err != nil {
		return viewmodel.PageData{}, attemptLoadError(x, err)
	}
	return x.Page(attemptTitle("Hasil kuis", data.Header.QuizTitle), "", data), nil
}

// attemptDraftPage rebuilds the question screen after a 422 (ATT-126): the
// posted values are carried in, every other value comes from storage. A draft
// load that itself fails leaves the validation error to the generic page.
func attemptDraftPage(x *Ctx, svc *attempts.Service, in attempts.AnswerInput) (viewmodel.PageData, bool) {
	draft := &attempts.QuestionDraft{Essay: in.EssayRaw}
	if qid, err := strconv.ParseInt(strings.TrimSpace(in.QuestionRaw), 10, 64); err == nil {
		draft.QuestionID = qid
	}
	for _, raw := range in.Options {
		if id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil {
			draft.Options = append(draft.Options, id)
		}
	}
	data, err := svc.QuestionPage(x.Context(), attempts.QuestionInput{
		AttemptID: in.AttemptID,
		QRaw:      x.QueryValue("q"),
		Owner:     in.Owner,
		Draft:     draft,
	})
	if err != nil {
		return viewmodel.PageData{}, false
	}
	return attemptQuestionPageData(x, data), true
}

// attemptAnswerAct serves POST /attempt/:id/answer (ATT-121…ATT-126). The form
// field names are the template contract: question_id, selected_option_ids[]
// (repeated), essay_text, nav.
func attemptAnswerAct(x *Ctx) (viewmodel.PageData, Redirect, error) {
	svc := attemptService(x)
	form := x.Form
	options := form["selected_option_ids[]"]
	_, hasOptions := form["selected_option_ids[]"]
	essay := form.Get("essay_text")
	_, hasEssay := form["essay_text"]

	in := attempts.AnswerInput{
		AttemptID:   attemptIDParam(x),
		QuestionRaw: form.Get("question_id"),
		NavRaw:      form.Get("nav"),
		Options:     options,
		HasOptions:  hasOptions,
		EssayRaw:    essay,
		HasEssay:    hasEssay,
		Owner:       attemptOwner(x),
	}
	res, err := svc.SaveAnswer(x.Context(), in)
	if err == nil {
		return viewmodel.PageData{}, x.Redirect(res.Path, res.Flash), nil
	}
	if r, ok := attemptRedirect(x, err); ok {
		return viewmodel.PageData{}, r, nil
	}
	var appErr *apperr.Error
	if errors.As(err, &appErr) && appErr.Kind() == apperr.Validation {
		if data, ok := attemptDraftPage(x, svc, in); ok {
			return data, Redirect{}, err
		}
	}
	return viewmodel.PageData{}, Redirect{}, err
}

// attemptSubmitAct serves POST /attempt/:id/submit (ATT-141). Submit answers
// with a 303 signal on success, success-by-idempotency, and a locked attempt
// alike; every other error is a page error.
func attemptSubmitAct(x *Ctx) (Redirect, error) {
	err := attemptService(x).Submit(x.Context(), attempts.SubmitInput{
		AttemptID: attemptIDParam(x),
		Owner:     attemptOwner(x),
	})
	if err == nil {
		return x.Redirect("/attempt/"+x.Param("id")+"/result", attempts.FlashSubmitted), nil
	}
	if r, ok := attemptRedirect(x, err); ok {
		return r, nil
	}
	return Redirect{}, err
}

func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/attempt/:id",
		Template: "pages/attempt/question.html",
		Load:     attemptQuestionLoad,
		Build: func(req PageRequest) viewmodel.PageData {
			data := fixtures.AttemptQuestionData(req.IntParam("id"), attemptQueryPosition(req))
			title := "Mengerjakan kuis"
			if data.Header.QuizTitle != "" {
				title = "Mengerjakan kuis · " + data.Header.QuizTitle
			}
			return page(req, title, "", nil, data)
		},
	})

	registerPage(PageRoute{
		Method:  "POST",
		Path:    "/attempt/:id/answer",
		ActForm: attemptAnswerAct,
		Action:  attemptAnswerAction,
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/attempt/:id/review",
		Template: "pages/attempt/review.html",
		Load:     attemptReviewLoad,
		Build: func(req PageRequest) viewmodel.PageData {
			data := fixtures.AttemptReviewData(req.IntParam("id"))
			title := "Tinjau jawaban"
			if data.Header.QuizTitle != "" {
				title = "Tinjau jawaban · " + data.Header.QuizTitle
			}
			return page(req, title, "", nil, data)
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/attempt/:id/submit",
		Act:    attemptSubmitAct,
		Action: func(req PageRequest) string {
			return withParam("/attempt/:id/result?flash=submitted", req)
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/attempt/:id/result",
		Template: "pages/attempt/result.html",
		Load:     attemptResultLoad,
		Build: func(req PageRequest) viewmodel.PageData {
			data := fixtures.AttemptResultData(req.IntParam("id"), req.State())
			title := "Hasil kuis"
			if data.Header.QuizTitle != "" {
				title = "Hasil kuis · " + data.Header.QuizTitle
			}
			return page(req, title, "", nil, data)
		},
	})

	registerPage(PageRoute{
		Method:     "POST",
		Path:       "/attempt/:id/beacon",
		Auth:       ClassAnonymousOrPublic,
		CSRFExempt: true,
		HandlerCtx: attemptBeacon,
	})
}
