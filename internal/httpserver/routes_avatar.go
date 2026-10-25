package httpserver

import "time"

// The two profile-photo mutations. Both are any-auth: a student and a teacher
// own the same avatar, so the class only says "signed in" — ownership lives in
// the account-scoped store (AVT-062).
//
// POST /profile/photo is the app's one Stream route: prepare() hands the body
// to avatarUpload untouched, and the handler takes the CSRF token from the
// first part itself (AVT-016, AVT-036). Its ten-minute deadline is AVT-025's
// conversion budget; a slow single-part upload is not an OPS-100 overrun.
// POST /profile/photo/remove is an ordinary urlencoded form through the normal
// pipeline (AVT-061).
func init() {
	registerPage(PageRoute{
		Method:     "POST",
		Path:       "/profile/photo",
		Auth:       ClassAnyAuth,
		HandlerCtx: avatarUpload,
		Stream:     true,
		Deadline:   10 * time.Minute,
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/profile/photo/remove",
		Auth:   ClassAnyAuth,
		Act:    avatarRemove,
	})
}
