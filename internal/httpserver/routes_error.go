package httpserver

import (
	"quiz-v2/internal/viewmodel"
)

// The two error screens are the only static pages: their copy is fixed, and a
// 500 page must still render when the database is unreachable (OPS-206).
func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/error/404",
		Template: "pages/error/404.html",
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			return x.Page("Halaman tidak ditemukan", "", viewmodel.ErrorPage{
				Code:     404,
				Title:    "Halaman tidak ditemukan",
				Message:  "Alamat yang Anda tuju tidak tersedia atau sudah dipindahkan.",
				HomeHref: "/login",
			}), nil
		},
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/error/500",
		Template: "pages/error/500.html",
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			return x.Page("Terjadi kesalahan", "", viewmodel.ErrorPage{
				Code:     500,
				Title:    "Terjadi kesalahan",
				Message:  "Permintaan Anda belum dapat diproses. Coba muat ulang beberapa saat lagi.",
				HomeHref: "/login",
			}), nil
		},
	})
}
