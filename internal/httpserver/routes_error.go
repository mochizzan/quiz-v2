package httpserver

import (
	"quiz-v2/internal/viewmodel"
)

func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/error/404",
		Template: "pages/error/404.html",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Halaman tidak ditemukan", "", nil, viewmodel.ErrorPage{
				Code:     404,
				Title:    "Halaman tidak ditemukan",
				Message:  "Alamat yang Anda tuju tidak tersedia atau sudah dipindahkan.",
				HomeHref: "/login",
			})
		},
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/error/500",
		Template: "pages/error/500.html",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Terjadi kesalahan", "", nil, viewmodel.ErrorPage{
				Code:     500,
				Title:    "Terjadi kesalahan",
				Message:  "Permintaan Anda belum dapat diproses. Coba muat ulang beberapa saat lagi.",
				HomeHref: "/login",
			})
		},
	})
}
