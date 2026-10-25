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
				Code:                404,
				Title:               "Halaman tidak ditemukan",
				Message:             "Alamat yang Anda tuju tidak tersedia atau sudah dipindahkan.",
				HomeHref:            "/login",
				ShowSecondaryAction: true,
			}), nil
		},
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/error/500",
		Template: "pages/error/500.html",
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			return x.Page("Terjadi kesalahan", "", viewmodel.ErrorPage{
				Code:                500,
				Title:               "Terjadi kesalahan",
				Message:             "Permintaan Anda belum dapat diproses. Coba muat ulang beberapa saat lagi.",
				HomeHref:            "/login",
				ShowSecondaryAction: true,
			}), nil
		},
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/error/403",
		Template: "pages/error/403.html",
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			return x.Page("Akses ditolak", "", viewmodel.ErrorPage{
				Code:                403,
				Title:               "Akses ditolak",
				Message:             "Anda tidak memiliki izin untuk mengakses halaman ini.",
				HomeHref:            "/login",
				ShowSecondaryAction: false,
			}), nil
		},
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/error/422",
		Template: "pages/error/422.html",
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			return x.Page("Permintaan tidak valid", "", viewmodel.ErrorPage{
				Code:                422,
				Title:               "Permintaan tidak valid",
				Message:             "Silakan periksa permintaan Anda dan coba lagi.",
				HomeHref:            "/login",
				ShowSecondaryAction: false,
			}), nil
		},
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/error/429",
		Template: "pages/error/429.html",
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			return x.Page("Terlalu banyak permintaan", "", viewmodel.ErrorPage{
				Code:                429,
				Title:               "Terlalu banyak permintaan",
				Message:             "Anda telah membuat terlalu banyak permintaan. Silakan coba lagi nanti.",
				HomeHref:            "/login",
				ShowSecondaryAction: false,
			}), nil
		},
	})
}
