// Package authoring implements the teacher-owned quiz aggregate: the quiz
// shell, its lifecycle, questions and options, question images, and the
// authoring read models.
//
// Every command in this package follows the business-logic spec
// docs/superpowers/specs/2026-10-23-business-logic-02-authoring-and-lifecycle.md
// (AUT-001…AUT-040). The package imports the view model, the store seam, and
// apperr; it never imports internal/httpserver, so a handler can be tested
// without Fiber.
package authoring

import "quiz-v2/internal/viewmodel"

// Closed flash keys owned by this domain (AUT-033). Keys that viewmodel.FlashFor
// already knows are returned as a signed flash cookie by the HTTP layer; the
// new keys are carried in the redirect query and resolved by FlashForKey,
// because the view model's closed set is owned by another package.
const (
	FlashQuizCreated     = "quiz-created"
	FlashSaved           = "saved"
	FlashQuestionSaved   = "question-saved"
	FlashPublished       = "published"
	FlashClosed          = "closed"
	FlashReordered       = "reordered"
	FlashQuizDeleted     = "quiz-deleted"
	FlashQuestionDeleted = "question-deleted"
	FlashAlreadyOpen     = "already-published"

	FlashErrNoQuestions     = "err-no-questions"
	FlashErrTargets         = "err-targets"
	FlashErrTargetMissing   = "err-target-missing"
	FlashErrInvalidQuestion = "err-invalid-questions"
	FlashErrImageMissing    = "err-image-missing"
	FlashErrInvalidSettings = "err-invalid-settings"
	FlashErrClosedTerminal  = "err-closed-terminal"
	FlashErrNotOpen         = "err-not-open"
	FlashErrNotDraft        = "err-not-draft"
	FlashErrDeleteNotDraft  = "err-delete-not-draft"
	FlashErrReorder         = "err-reorder"
	FlashErrQuestionLimit   = "err-question-limit"
	FlashErrForm            = "err-form"
)

// Exact Indonesian copy owned by AUT-033 and the commands that emit it.
const (
	MsgQuizDeleted     = "Draf kuis dihapus."
	MsgQuestionDeleted = "Soal dihapus."
	MsgAlreadyOpen     = "Kuis sudah dibuka."

	MsgNoQuestions     = "Tambahkan minimal satu soal sebelum membuka kuis."
	MsgTargets         = "Lengkapi kelas dan jurusan sasaran sebelum membuka kuis."
	MsgTargetMissing   = "Jurusan sasaran tidak tersedia."
	MsgInvalidQuestion = "Perbaiki soal yang belum valid sebelum membuka kuis."
	MsgImageMissing    = "Satu atau lebih gambar soal tidak ditemukan. Unggah ulang gambar sebelum membuka kuis."
	MsgInvalidSettings = "Pengaturan kuis tidak valid."
	MsgClosedTerminal  = "Kuis yang sudah ditutup tidak dapat dibuka kembali."
	MsgNotOpen         = "Kuis belum dibuka."
	MsgNotDraft        = "Soal dan pengaturan hanya dapat diubah saat kuis masih draf."
	MsgDeleteNotDraft  = "Hanya draf yang dapat dihapus."
	MsgReorder         = "Daftar soal berubah. Muat ulang halaman."
	MsgQuestionLimit   = "Batas jumlah soal tercapai."
	MsgFormInvalid     = "Formulir tidak valid. Muat ulang halaman."
)

// QuestionEssayNote is the editor rail copy for an essay question (AUT-009).
const QuestionEssayNote = "Esai dinilai manual oleh guru. Tulis pertanyaan terbuka yang mendorong penjelasan, bukan jawaban ya atau tidak."

// Field-level copy (AUT-002, AUT-005, AUT-010, AUT-023).
const (
	MsgTitleRequired       = "Judul kuis wajib diisi."
	MsgTitleTooLong        = "Judul kuis maksimal 150 karakter."
	MsgDescriptionTooLong  = "Deskripsi maksimal 1.000 karakter."
	MsgStartInvalid        = "Waktu mulai tidak valid."
	MsgEndInvalid          = "Waktu selesai tidak valid."
	MsgEndNotAfterStart    = "Waktu selesai harus setelah waktu mulai."
	MsgVisibilityInvalid   = "Visibilitas tidak valid."
	MsgGradeRequired       = "Pilih kelas sasaran untuk kuis privat."
	MsgGradeInvalid        = "Kelas sasaran tidak valid."
	MsgMajorRequired       = "Pilih jurusan sasaran untuk kuis privat."
	MsgMajorUnavailable    = "Jurusan yang dipilih tidak tersedia."
	MsgSettingsInvalid     = "Pengaturan tidak valid."
	MsgAttemptsRange       = "Batas percobaan harus antara 1 dan 10."
	MsgReviewModeInvalid   = "Tampilan hasil tidak valid."
	MsgQuestionTypeInvalid = "Tipe soal tidak valid."
	MsgPromptRequired      = "Teks soal wajib diisi."
	MsgPromptTooShort      = "Teks soal minimal 8 karakter."
	MsgPromptTooLong       = "Teks soal maksimal 2.000 karakter."
	MsgAltTooLong          = "Teks alternatif gambar maksimal 255 karakter."
	MsgOptionsMin          = "Soal pilihan memerlukan minimal dua pilihan."
	MsgOptionsMax          = "Pilihan jawaban maksimal 26."
	MsgOptionRequired      = "wajib diisi."
	MsgOptionTooLong       = "maksimal 500 karakter."
	MsgSingleKey           = "Pilih tepat satu jawaban benar."
	MsgMultipleKey         = "Pilih minimal dua jawaban benar."
	MsgOptionKeyInvalid    = "Kunci jawaban tidak valid."
	MsgOneImage            = "Unggah satu gambar saja."
	MsgImageEmpty          = "Berkas gambar kosong."
	MsgImageTooLarge       = "Gambar maksimal 10 MB."
	MsgImageType           = "Gambar harus berupa JPEG, PNG, atau WebP."
	MsgImageUnreadable     = "Gambar tidak dapat dibaca."
)

// FlashForKey resolves the closed flash key set, including the keys the shared
// view model does not know. An unknown key returns nil and is never reflected
// into a response.
func FlashForKey(key string) *viewmodel.Flash {
	switch key {
	case FlashQuizDeleted:
		return &viewmodel.Flash{Kind: "success", Title: MsgQuizDeleted}
	case FlashQuestionDeleted:
		return &viewmodel.Flash{Kind: "success", Title: MsgQuestionDeleted}
	case FlashAlreadyOpen:
		return &viewmodel.Flash{Kind: "info", Title: MsgAlreadyOpen}
	case FlashErrNoQuestions:
		return &viewmodel.Flash{Kind: "danger", Title: MsgNoQuestions}
	case FlashErrTargets:
		return &viewmodel.Flash{Kind: "danger", Title: MsgTargets}
	case FlashErrTargetMissing:
		return &viewmodel.Flash{Kind: "danger", Title: MsgTargetMissing}
	case FlashErrInvalidQuestion:
		return &viewmodel.Flash{Kind: "danger", Title: MsgInvalidQuestion}
	case FlashErrImageMissing:
		return &viewmodel.Flash{Kind: "danger", Title: MsgImageMissing}
	case FlashErrInvalidSettings:
		return &viewmodel.Flash{Kind: "danger", Title: MsgInvalidSettings}
	case FlashErrClosedTerminal:
		return &viewmodel.Flash{Kind: "danger", Title: MsgClosedTerminal}
	case FlashErrNotOpen:
		return &viewmodel.Flash{Kind: "danger", Title: MsgNotOpen}
	case FlashErrNotDraft:
		return &viewmodel.Flash{Kind: "danger", Title: MsgNotDraft}
	case FlashErrDeleteNotDraft:
		return &viewmodel.Flash{Kind: "danger", Title: MsgDeleteNotDraft}
	case FlashErrReorder:
		return &viewmodel.Flash{Kind: "danger", Title: MsgReorder}
	case FlashErrQuestionLimit:
		return &viewmodel.Flash{Kind: "danger", Title: MsgQuestionLimit}
	case FlashErrForm:
		// AUT-033 lists err-form among the closed danger keys. The same string
		// is also the 422 page-level field key in the HTTP layer; the two
		// uses share the copy but never the mechanism.
		return &viewmodel.Flash{Kind: "danger", Title: MsgFormInvalid}
	default:
		return viewmodel.FlashFor(key)
	}
}

// ReasonText returns the copy for a publish-gate reason key.
func ReasonText(key string) string {
	if f := FlashForKey(key); f != nil {
		return f.Title
	}
	return ""
}
