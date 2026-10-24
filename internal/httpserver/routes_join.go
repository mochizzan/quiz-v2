package httpserver

// This file is the "join" route group, owned by the participation domain: the
// join-by-code entry, the invitation landing, and the anonymous identity form.
// None of these require an account, and all of them accept an already signed-in
// student, which is why their class is anonymous-or-public.
//
// Load/Act/ActForm are the production handlers (PAR, spec 03); Build/Action
// remain the APP_DEMO fixture world. The POST handlers return 303 with a closed
// flash key; only a 422 re-renders the identify form with field errors.

import (
	"errors"
	"strconv"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/httpserver/fixtures"
	"quiz-v2/internal/participation"
	"quiz-v2/internal/viewmodel"
)

// parApplyCookies writes a domain command's cookie delta. The domain returns a
// delta only after its transaction committed (PAR-058, A§9.1.5), so a 404, 422,
// or 429 never touches the browser's identity or ticket.
func parApplyCookies(x *Ctx, c participation.Cookies) {
	if c.IdentityClear {
		x.ClearCookie(participation.IdentityCookie, participation.IdentityPath)
	}
	if c.IdentityToken != "" {
		x.SetCookie(participation.IdentityCookie, c.IdentityToken,
			participation.IdentityMaxAge, participation.IdentityPath)
	}
	if c.TicketClear {
		x.ClearCookie(participation.TicketCookie, participation.TicketPath)
	}
	if c.TicketValue != "" {
		x.SetCookie(participation.TicketCookie, c.TicketValue,
			participation.TicketMaxAge, participation.TicketPath)
	}
}

// parRate enforces one fixed-window PAR bucket before any lookup or write
// (A§8.7.8). Over-limit maps to 429 with Retry-After; the identity and ticket
// cookies survive so a later retry needs no re-entry.
func parRate(x *Ctx, key string, limit int) error {
	ok, retry := x.Limiter().Allow(key, limit, participation.RateWindow)
	if ok {
		return nil
	}
	return apperr.RateLimitedf(retry).WithRule("OPS-205")
}

// parRateKeyIP keys a public action by client address. The limiter never sees
// a raw header or a participant name.
func parRateKeyIP(x *Ctx) string {
	if x.IP == "" {
		return "unknown"
	}
	return x.IP
}

// joinLoad serves GET /join (PAR-021…PAR-026). A teacher is redirected before
// the page renders; a stale identity cookie is cleared and the request
// continues as a visitor.
func joinLoad(x *Ctx) (viewmodel.PageData, error) {
	viewer, err := parService(x).Viewer(x.Context(), parActor(x))
	if err != nil {
		return viewmodel.PageData{}, err
	}
	if viewer.Cookies.IdentityClear {
		parApplyCookies(x, viewer.Cookies)
	}
	if viewer.Actor.IsTeacher() {
		return viewmodel.PageData{}, x.SeeOther("/teacher", "")
	}
	return x.Page("Gabung dengan kode kuis", "", viewmodel.JoinPage{
		Error: participation.JoinErrorFor(x.QueryValue("error")),
	}), nil
}

// joinAct serves POST /join (PAR-027…PAR-035).
func joinAct(x *Ctx) (Redirect, error) {
	if err := parRate(x, participation.RateKey("join", parRateKeyIP(x)), participation.RateJoinLimit); err != nil {
		return Redirect{}, err
	}
	out, err := parService(x).ResolveCode(x.Context(), participation.ResolveCodeInput{
		Actor: parActor(x),
		Code:  x.RawValue("code"),
	})
	if err != nil {
		return Redirect{}, parRedirect(x, err)
	}
	parApplyCookies(x, out.Cookies)
	return x.Redirect(out.Result.Path, out.Result.Flash), nil
}

// inviteLoad serves GET /invite/:token (PAR-036…PAR-044).
func inviteLoad(x *Ctx) (viewmodel.PageData, error) {
	res, err := parService(x).Invite(x.Context(), participation.InviteInput{
		Actor: parActor(x),
		Token: x.Param("token"),
	})
	if err != nil {
		// A landing redirect can still carry the invite ticket (or a stale
		// identity clear). Only a 303 gets cookies; a 404/500 touches nothing.
		var r *participation.Redirect
		if errors.As(err, &r) {
			parApplyCookies(x, res.Cookies)
		}
		return viewmodel.PageData{}, parRedirect(x, err)
	}
	parApplyCookies(x, res.Cookies)
	return x.Page("Undangan kuis", "", res.Page), nil
}

// identifyLoad serves GET /join/identify (PAR-045…PAR-052). A missing or
// mismatched ?quiz= is a 303 to the ticket's canonical quiz, not a 404.
func identifyLoad(x *Ctx) (viewmodel.PageData, error) {
	res, err := parService(x).IdentifyPage(x.Context(), participation.IdentifyPageInput{
		Actor:  parActor(x),
		QuizID: parQueryID(x, "quiz"),
		Switch: x.QueryValue("switch") == "1",
	})
	if err != nil {
		// The switch redirect carries the identity clear and the rotated ticket,
		// and the closed-quiz redirect carries the ticket clear; dropping the
		// delta would leave the browser able to derive the old identity again.
		var r *participation.Redirect
		if errors.As(err, &r) {
			parApplyCookies(x, res.Cookies)
		}
		return viewmodel.PageData{}, parRedirect(x, err)
	}
	parApplyCookies(x, res.Cookies)
	return x.Page("Identitas peserta", "", res.Page), nil
}

// identifyAct serves POST /join/identify (PAR-053…PAR-060). A 422 re-renders
// the same form with the submitted non-secret values and the ticket intact.
func identifyAct(x *Ctx) (viewmodel.PageData, Redirect, error) {
	if err := parRate(x, participation.RateKey("identify", parRateKeyIP(x)), participation.RateIdentifyLimit); err != nil {
		return viewmodel.PageData{}, Redirect{}, err
	}
	res, err := parService(x).Identify(x.Context(), participation.IdentifyInput{
		QuizID: parQueryID(x, "quiz"),
		Actor:  parActor(x),
		Form: participation.IdentityForm{
			FullName:   x.RawValue("full_name"),
			GradeLevel: x.RawValue("grade_level"),
			MajorID:    x.RawValue("major_id"),
		},
	})
	if err != nil {
		var appErr *apperr.Error
		if errors.As(err, &appErr) && appErr.Kind() == apperr.Validation {
			return x.Page("Identitas peserta", "", res.Page), Redirect{}, err
		}
		return viewmodel.PageData{}, Redirect{}, parRedirect(x, err)
	}
	parApplyCookies(x, res.Out.Cookies)
	return viewmodel.PageData{}, x.Redirect(res.Out.Result.Path, res.Out.Result.Flash), nil
}

// parQueryID parses a positive integer query value; anything else is 0, which
// the domain treats as absent (query values never 422, A§5.0.5).
func parQueryID(x *Ctx, name string) int64 {
	n, err := strconv.ParseInt(x.QueryValue(name), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// init registers the five participation entry routes.
func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/join",
		Template: "pages/auth/join.html",
		Load:     joinLoad,
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Gabung dengan kode kuis", "", nil, fixtures.Join("", nil))
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/join",
		Act:    joinAct,
		Action: func(req PageRequest) string {
			if id, ok := fixtures.QuizForJoinCode(req.Form.Get("code")); ok {
				return "/quiz/" + strconv.FormatInt(id, 10)
			}
			return "/join?state=unknown"
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/invite/:token",
		Template: "pages/auth/invite.html",
		Load:     inviteLoad,
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Undangan kuis", "", nil, fixtures.Invite(req.Param("token")))
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/join/identify",
		Template: "pages/auth/identify.html",
		Load:     identifyLoad,
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Identitas peserta", "", nil, fixtures.Identify("", 0, 0, nil))
		},
	})
	registerPage(PageRoute{
		Method:  "POST",
		Path:    "/join/identify",
		ActForm: identifyAct,
		Action:  action("/quiz/1?state=anonymous", ""),
	})
}
