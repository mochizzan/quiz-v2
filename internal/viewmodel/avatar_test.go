package viewmodel

import "testing"

// validName is a name the avatar grammar accepts: 32 lowercase hex characters
// and the .webp extension.
const validName = "0123456789abcdef0123456789abcdef.webp"

// TestAvatarURL: only a grammar-clean stored name becomes a URL; anything
// else renders no photo at all (AVT-080).
func TestAvatarURL(t *testing.T) {
	if got := AvatarURL(validName); got != "/avatars/"+validName {
		t.Errorf("AvatarURL(%q) = %q, want %q", validName, got, "/avatars/"+validName)
	}
	for _, name := range []string{
		"",
		"missing.webp",
		"0123456789ABCDEF0123456789ABCDEF.webp", // uppercase alias
		"0123456789abcdef0123456789abcde.webp",  // one short
		"0123456789abcdef0123456789abcdef.jpg",  // other extension
		"../../../etc/passwd",                   // traversal
		"/avatars/" + validName,                 // already a URL
	} {
		if got := AvatarURL(name); got != "" {
			t.Errorf("AvatarURL(%q) = %q, want empty", name, got)
		}
	}
}

// TestNewAvatar: the photo URL comes from the name and the initials from the
// display name, exactly as the fallback badge computes them.
func TestNewAvatar(t *testing.T) {
	got := NewAvatar(validName, "Siti Aminah", "app-avatar--sm")
	if got.URL != "/avatars/"+validName {
		t.Errorf("URL = %q, want the avatar route", got.URL)
	}
	if got.Initials != "SA" {
		t.Errorf("Initials = %q, want SA", got.Initials)
	}
	if got.Class != "app-avatar--sm" {
		t.Errorf("Class = %q, want the server-chosen class verbatim", got.Class)
	}

	empty := NewAvatar("", "Budi Santoso", "")
	if empty.URL != "" || empty.Initials != "BS" || empty.Class != "" {
		t.Errorf("NewAvatar with no name = %+v, want initials only", empty)
	}
}

// TestCurrentUserAvatar: the chrome method pairs the session's photo URL with
// the existing initials and no extra class, and the zero value renders the
// initials badge the UI already ships.
func TestCurrentUserAvatar(t *testing.T) {
	// The field holds the mapped URL: handlers set it through AvatarURL.
	u := CurrentUser{FullName: "Ayu Lestari", Initials: "AL", AvatarURL: "/avatars/" + validName}
	got := u.Avatar()
	if got.URL != "/avatars/"+validName || got.Initials != "AL" || got.Class != "" {
		t.Errorf("Avatar() = %+v, want the photo, the initials, no class", got)
	}

	bare := CurrentUser{FullName: "Ayu Lestari", Initials: "AL"}
	got = bare.Avatar()
	if got.URL != "" || got.Initials != "AL" || got.Class != "" {
		t.Errorf("Avatar() without a photo = %+v, want initials only", got)
	}
}

// TestFlashForPhotoKeys pins the AVT-094 copy verbatim: handlers and tests
// share these strings, and an unknown key stays ignored.
func TestFlashForPhotoKeys(t *testing.T) {
	for _, tc := range []struct {
		key  string
		kind string
		want string
	}{
		{"photo-empty", "error", "Pilih foto terlebih dahulu."},
		{"photo-type", "error", "Format foto harus JPG, PNG, WebP, atau HEIC."},
		{"photo-big", "error", "Foto terlalu besar untuk diproses."},
		{"photo-unreadable", "error", "Foto tidak dapat dibaca. Coba file lain."},
		{"photo-failed", "error", "Foto gagal disimpan. Coba lagi."},
		{"photo-busy", "error", "Server sedang sibuk. Coba lagi sebentar."},
		{"photo-remove-failed", "error", "Foto gagal dihapus. Coba lagi."},
		{"photo-saved", "success", "Foto profil diperbarui."},
		{"photo-removed", "success", "Foto profil dihapus."},
	} {
		f := FlashFor(tc.key)
		if f == nil {
			t.Errorf("FlashFor(%q) = nil", tc.key)
			continue
		}
		if f.Kind != tc.kind || f.Title != tc.want {
			t.Errorf("FlashFor(%q) = %+v, want kind %s title %q", tc.key, f, tc.kind, tc.want)
		}
	}
	for _, key := range []string{"photo", "photo-failed ", "photo-uploaded", "avatar-saved", "made-up-key"} {
		if FlashFor(key) != nil {
			t.Errorf("FlashFor(%q) must stay ignored", key)
		}
	}
}
