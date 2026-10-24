package identity

import "quiz-v2/internal/viewmodel"

// Closed flash keys owned by this domain (IDN-080, IDN-C19, IDN-C22). Keys the
// shared view model already knows ride the signed one-shot cookie; the two new
// keys are carried in the redirect query and resolved by FlashForKey, because
// the view model's closed set is owned by another package.
const (
	// FlashApproved is the teacher-approval success key.
	FlashApproved = "approved"
	// FlashAlreadyApproved is the idempotent-approval key (IDN-C22).
	FlashAlreadyApproved = "already_approved"
	// FlashDeletedStudent is the student-deletion success key (IDN-C19).
	FlashDeletedStudent = "deleted_student"
	// FlashSaved is the shared account/profile save key.
	FlashSaved = "saved"
	// FlashToken is the shared reset-token-issued key.
	FlashToken = "token"
	// FlashPasswordReset is the shared reset-consumed key.
	FlashPasswordReset = "password-reset"
	// FlashPasswordChanged is the shared change-password key.
	FlashPasswordChanged = "password-changed"
	// FlashLoggedOut is the shared sign-out key.
	FlashLoggedOut = "logout"
	// FlashWelcome is the shared post-login/registration key.
	FlashWelcome = "welcome"
	// FlashMustChange is the shared temporary-password key.
	FlashMustChange = "must_change"
	// FlashSessionExpired is the shared revoked-session key.
	FlashSessionExpired = "session_expired"
)

// Domain copy from IDN-080 that the shared view model does not own. Duplicated
// R-19 strings are cited from internal/apperr instead of re-spelled here.
const (
	// MsgDeletedStudent is `deleted_student`.
	MsgDeletedStudent = "Data siswa dihapus."
	// MsgAlreadyApproved is `already_approved`.
	MsgAlreadyApproved = "Akun ini sudah disetujui."
	// MsgApprovalUnavailable is `approval_unavailable`, the static line the
	// pending page may show when no active teacher can approve yet (IDN-123).
	MsgApprovalUnavailable = "Akun guru baru dapat disetujui setelah sekolah memiliki guru aktif."
)

// FlashForKey resolves this domain's closed flash-key set, including the two
// keys the shared view model does not know. An unknown key returns nil and is
// never reflected into a response.
func FlashForKey(key string) *viewmodel.Flash {
	switch key {
	case FlashDeletedStudent:
		return &viewmodel.Flash{Kind: "success", Title: MsgDeletedStudent}
	case FlashAlreadyApproved:
		return &viewmodel.Flash{Kind: "info", Title: MsgAlreadyApproved}
	default:
		return viewmodel.FlashFor(key)
	}
}
