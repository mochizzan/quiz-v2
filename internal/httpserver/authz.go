package httpserver

import "fmt"

// AuthClass is the authorization class every registered route declares. There
// is no default-allow: a route without a class is a startup error (IDN §2.2).
//
// Classes decide *entry*, not ownership. An `owner-teacher` route admits any
// active teacher and the handler returns 404 for a non-owner, because 403 would
// confirm the resource exists (R-05, OPS-204).
type AuthClass string

const (
	// ClassPublic needs no session. The handler decides what to render.
	ClassPublic AuthClass = "public"
	// ClassGuestOnly is for signed-out visitors; an authenticated user is sent
	// to their role home.
	ClassGuestOnly AuthClass = "guest-only"
	// ClassAnyAuth admits either role.
	ClassAnyAuth AuthClass = "any-auth"
	// ClassStudent admits active students only; other roles get 404.
	ClassStudent AuthClass = "student"
	// ClassTeacher admits active teachers only; other roles get 404.
	ClassTeacher AuthClass = "teacher"
	// ClassOwnerTeacher admits an active teacher and leaves the ownership check
	// to the handler, which answers 404.
	ClassOwnerTeacher AuthClass = "owner-teacher"
	// ClassAnonymousOrPublic admits PAR's anonymous participants and any signed
	// in actor. Ownership is the handler's job.
	ClassAnonymousOrPublic AuthClass = "anonymous-or-public"
)

// AllClasses lists every valid class, for validation and tests.
var AllClasses = []AuthClass{
	ClassPublic, ClassGuestOnly, ClassAnyAuth, ClassStudent,
	ClassTeacher, ClassOwnerTeacher, ClassAnonymousOrPublic,
}

func (c AuthClass) valid() bool {
	for _, known := range AllClasses {
		if c == known {
			return true
		}
	}
	return false
}

// routeKey is the registry key for one method+path pair.
func routeKey(method, path string) string { return method + " " + path }

// routeClasses is the authorization matrix for the pinned routes plus the
// operational routes outside the pin. It is deliberately one table rather
// than a field on each registration: the matrix is auditable in one place and
// cannot drift route by route (IDN-015, IDN-016, OPS-220).
//
// Owners: IDN, AUT, PAR, ATT, OPS, AVT — see contract §6.
var routeClasses = map[string]AuthClass{
	// IDN — identity and access.
	routeKey("GET", "/"):                                  ClassPublic,
	routeKey("GET", "/login"):                             ClassGuestOnly,
	routeKey("POST", "/login"):                            ClassGuestOnly,
	routeKey("GET", "/register/teacher"):                  ClassGuestOnly,
	routeKey("POST", "/register/teacher"):                 ClassGuestOnly,
	routeKey("GET", "/register/student"):                  ClassGuestOnly,
	routeKey("POST", "/register/student"):                 ClassGuestOnly,
	routeKey("GET", "/reset-password"):                    ClassGuestOnly,
	routeKey("POST", "/reset-password"):                   ClassGuestOnly,
	routeKey("GET", "/change-password"):                   ClassAnyAuth,
	routeKey("POST", "/change-password"):                  ClassAnyAuth,
	routeKey("GET", "/auth/pending"):                      ClassPublic,
	routeKey("POST", "/logout"):                           ClassPublic,
	routeKey("GET", "/profile"):                           ClassStudent,
	routeKey("POST", "/profile"):                          ClassStudent,
	routeKey("GET", "/teacher/students"):                  ClassTeacher,
	routeKey("GET", "/teacher/students/:id"):              ClassTeacher,
	routeKey("POST", "/teacher/students/:id"):             ClassTeacher,
	routeKey("POST", "/teacher/students/:id/delete"):      ClassTeacher,
	routeKey("POST", "/teacher/students/:id/reset-token"): ClassTeacher,
	routeKey("GET", "/teacher/teachers"):                  ClassTeacher,
	routeKey("POST", "/teacher/teachers/:id/approve"):     ClassTeacher,
	routeKey("GET", "/teacher/profile"):                   ClassTeacher,
	routeKey("POST", "/teacher/profile"):                  ClassTeacher,

	// AVT — profile photos. Both mutations are any-auth (a student and a
	// teacher own the same avatar); the store is account-scoped, so class is
	// not where ownership is decided. The asset route sits with its owner and
	// outside the contract-route pin (AVT-062, OPS-220).
	routeKey("POST", "/profile/photo"):        ClassAnyAuth,
	routeKey("POST", "/profile/photo/remove"): ClassAnyAuth,
	routeKey("GET", "/avatars/:name"):         ClassPublic,

	// PAR — participation and eligibility.
	routeKey("GET", "/join"):                          ClassAnonymousOrPublic,
	routeKey("POST", "/join"):                         ClassAnonymousOrPublic,
	routeKey("GET", "/invite/:token"):                 ClassAnonymousOrPublic,
	routeKey("GET", "/join/identify"):                 ClassAnonymousOrPublic,
	routeKey("POST", "/join/identify"):                ClassAnonymousOrPublic,
	routeKey("GET", "/quiz/:id"):                      ClassAnonymousOrPublic,
	routeKey("POST", "/quiz/:id/register"):            ClassAnonymousOrPublic,
	routeKey("POST", "/quiz/:id/cancel"):              ClassAnonymousOrPublic,
	routeKey("POST", "/quiz/:id/attempts"):            ClassAnonymousOrPublic,
	routeKey("GET", "/student"):                       ClassStudent,
	routeKey("GET", "/student/history"):               ClassStudent,
	routeKey("GET", "/student/history/:quizID"):       ClassStudent,
	routeKey("GET", "/teacher/anonymous"):             ClassTeacher,
	routeKey("GET", "/teacher/anonymous/:id"):         ClassTeacher,
	routeKey("POST", "/teacher/anonymous/:id"):        ClassTeacher,
	routeKey("POST", "/teacher/anonymous/:id/delete"): ClassTeacher,
	routeKey("GET", "/teacher/majors"):                ClassTeacher,
	routeKey("POST", "/teacher/majors"):               ClassTeacher,
	routeKey("POST", "/teacher/majors/:id"):           ClassTeacher,
	routeKey("POST", "/teacher/majors/:id/delete"):    ClassTeacher,

	// AUT — authoring and lifecycle.
	routeKey("GET", "/teacher/quizzes"):                            ClassTeacher,
	routeKey("GET", "/teacher/quizzes/new"):                        ClassTeacher,
	routeKey("POST", "/teacher/quizzes"):                           ClassTeacher,
	routeKey("GET", "/teacher/quizzes/:id"):                        ClassOwnerTeacher,
	routeKey("POST", "/teacher/quizzes/:id"):                       ClassOwnerTeacher,
	routeKey("POST", "/teacher/quizzes/:id/publish"):               ClassOwnerTeacher,
	routeKey("POST", "/teacher/quizzes/:id/close"):                 ClassOwnerTeacher,
	routeKey("POST", "/teacher/quizzes/:id/delete"):                ClassOwnerTeacher,
	routeKey("GET", "/teacher/quizzes/:id/questions/new"):          ClassOwnerTeacher,
	routeKey("POST", "/teacher/quizzes/:id/questions"):             ClassOwnerTeacher,
	routeKey("GET", "/teacher/quizzes/:id/questions/:qid"):         ClassOwnerTeacher,
	routeKey("POST", "/teacher/quizzes/:id/questions/:qid"):        ClassOwnerTeacher,
	routeKey("POST", "/teacher/quizzes/:id/questions/:qid/delete"): ClassOwnerTeacher,
	routeKey("POST", "/teacher/quizzes/:id/questions/reorder"):     ClassOwnerTeacher,
	routeKey("GET", "/uploads/:name"):                              ClassPublic,

	// ATT — attempts, grading, results.
	routeKey("GET", "/attempt/:id"):                 ClassAnonymousOrPublic,
	routeKey("POST", "/attempt/:id/answer"):         ClassAnonymousOrPublic,
	routeKey("GET", "/attempt/:id/review"):          ClassAnonymousOrPublic,
	routeKey("POST", "/attempt/:id/submit"):         ClassAnonymousOrPublic,
	routeKey("GET", "/attempt/:id/result"):          ClassAnonymousOrPublic,
	routeKey("POST", "/attempt/:id/beacon"):         ClassAnonymousOrPublic,
	routeKey("GET", "/teacher"):                     ClassTeacher,
	routeKey("GET", "/teacher/quizzes/:id/results"): ClassOwnerTeacher,
	routeKey("GET", "/teacher/quizzes/:id/monitor"): ClassOwnerTeacher,
	routeKey("GET", "/teacher/quizzes/:id/stream"):  ClassOwnerTeacher,
	routeKey("GET", "/teacher/grading"):             ClassOwnerTeacher,
	routeKey("GET", "/teacher/grading/:answerID"):   ClassOwnerTeacher,
	routeKey("POST", "/teacher/grading/:answerID"):  ClassOwnerTeacher,
	routeKey("GET", "/teacher/recap/:attemptID"):    ClassOwnerTeacher,

	// OPS — operational and error preview routes.
	routeKey("GET", "/error/403"): ClassPublic,
	routeKey("GET", "/error/404"): ClassPublic,
	routeKey("GET", "/error/413"): ClassPublic,
	routeKey("GET", "/error/422"): ClassPublic,
	routeKey("GET", "/error/429"): ClassPublic,
	routeKey("GET", "/error/500"): ClassPublic,
	routeKey("GET", "/healthz"):   ClassPublic,
}

// csrfExempt holds the single POST that does not use the double-submit token.
// It is ownership-checked and same-origin-checked by the attempt domain instead
// (R-16, IDN-017, INV-28).
var csrfExempt = map[string]bool{
	routeKey("POST", "/attempt/:id/beacon"): true,
}

// streamRoutes holds the routes whose request body is long-lived and owned by
// their handler rather than parsed by prepare(). AVT-036 keeps this set tiny
// and explicit: a streaming route opts out of the ordinary form parse and the
// POST CSRF dance, so it must be named in this table rather than flagged
// anywhere. The upload handler still enforces CSRF itself from the first body
// part (AVT-016); the flag changes only who reads the body, never who authorises
// the request.
var streamRoutes = map[string]bool{
	routeKey("POST", "/profile/photo"): true,
}

// classFor returns the declared class for a route. ok is false when the route
// has no entry, which is a startup failure rather than a default.
func classFor(method, path string) (AuthClass, bool) {
	c, ok := routeClasses[routeKey(method, path)]
	return c, ok
}

// validateRouteClasses fails when any registered route is unclassified, cites
// the CK, or is exempt from CSRF without being the beacon. It runs once while
// the app is built, so a missing row can never become a default-allow.
func validateRouteClasses(routes []PageRoute) error {
	for _, r := range routes {
		c, ok := classFor(r.Method, r.Path)
		if !ok {
			return fmt.Errorf("httpserver: route %s has no authorization class", routeKey(r.Method, r.Path))
		}
		if !c.valid() {
			return fmt.Errorf("httpserver: route %s has unknown class %q", routeKey(r.Method, r.Path), c)
		}
		// A declared Auth field is documentation; when present it must agree
		// with the authoritative table, or the two have already drifted.
		if r.Auth != "" && r.Auth != c {
			return fmt.Errorf("httpserver: route %s declares class %q but the matrix says %q",
				routeKey(r.Method, r.Path), r.Auth, c)
		}
		if r.CSRFExempt && !csrfExempt[routeKey(r.Method, r.Path)] {
			return fmt.Errorf("httpserver: route %s may not be CSRF-exempt", routeKey(r.Method, r.Path))
		}
		// Streaming changes who consumes the body, so it is allowed only on a
		// route named in streamRoutes. A Stream flag on an unlisted route would
		// silently skip the form parse and CSRF for a normal form POST.
		if r.Stream && !streamRoutes[routeKey(r.Method, r.Path)] {
			return fmt.Errorf("httpserver: route %s streams its body but is not in streamRoutes", routeKey(r.Method, r.Path))
		}
		if !r.Stream && streamRoutes[routeKey(r.Method, r.Path)] {
			return fmt.Errorf("httpserver: route %s is in streamRoutes but does not declare Stream", routeKey(r.Method, r.Path))
		}
	}
	return nil
}

// classedRoutes counts the table rows, used by tests to prove the matrix covers
// exactly the audited surface.
func classedRoutes() int { return len(routeClasses) }

// ClassedRoutes is classedRoutes for the conformance suite.
func ClassedRoutes() int { return classedRoutes() }
