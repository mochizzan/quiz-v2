package identity

// avatar_fill_test.go pins AVT-083 for the identity read surfaces: every
// query that already joins accounts selects avatar_name in that same join, so
// the roster row, the student edit header, and the active teacher directory
// carry the photo without a second query. A student or teacher without a
// photo stays on the initials fallback, and pending teachers keep initials.

import (
	"testing"

	"quiz-v2/internal/viewmodel"
)

func TestAvatarFillsIdentityReads(t *testing.T) {
	f := newFixture(t)
	major := f.seedMajor("RPL", "RPL")
	activeTeacher := f.seedTeacher("guru@avatar.test", "Guru Utama", viewmodel.AccountActive)
	f.seedTeacher("calon@avatar.test", "Calon Guru", viewmodel.AccountPending)

	ayuAccount, ayu := f.seedStudent("ayu@avatar.test", "Ayu Lestari", 10, major, 0)
	_, budi := f.seedStudent("budi@avatar.test", "Budi Santoso", 11, major, 0)

	ayuPhoto := testAvatarName("1")
	teacherPhoto := testAvatarName("2")
	f.exec(`UPDATE accounts SET avatar_name = ? WHERE id = ?`, ayuPhoto, ayuAccount)
	f.exec(`UPDATE accounts SET avatar_name = ? WHERE id = ?`, teacherPhoto, activeTeacher)
	wantAyu := "/avatars/" + ayuPhoto
	wantTeacher := "/avatars/" + teacherPhoto

	t.Run("roster", func(t *testing.T) {
		list, err := f.svc.Students(f.ctx, RosterInput{ActorID: activeTeacher})
		if err != nil {
			t.Fatalf("Students: %v", err)
		}
		byName := map[string]viewmodel.Avatar{}
		for _, row := range list.Students.Items {
			byName[row.FullName] = row.Avatar
		}
		if ayuRow := byName["Ayu Lestari"]; ayuRow.URL != wantAyu ||
			ayuRow.Class != "app-avatar--sm" {
			t.Errorf("Ayu roster avatar = %+v, want %s at app-avatar--sm", ayuRow, wantAyu)
		}
		if budiRow := byName["Budi Santoso"]; budiRow.URL != "" ||
			budiRow.Initials != "BS" || budiRow.Class != "app-avatar--sm" {
			t.Errorf("Budi roster avatar = %+v, want initials-only at app-avatar--sm", budiRow)
		}
	})

	t.Run("student edit header", func(t *testing.T) {
		form, err := f.svc.StudentForm(f.ctx, ayu)
		if err != nil {
			t.Fatalf("StudentForm: %v", err)
		}
		if form.Avatar.URL != wantAyu || form.Avatar.Class != "" {
			t.Errorf("student form avatar = %+v, want %s at the default size", form.Avatar, wantAyu)
		}

		plain, err := f.svc.StudentForm(f.ctx, budi)
		if err != nil {
			t.Fatalf("StudentForm(budi): %v", err)
		}
		if plain.Avatar.URL != "" || plain.Avatar.Initials != "BS" {
			t.Errorf("photo-less form avatar = %+v, want the initials fallback", plain.Avatar)
		}
	})

	t.Run("teacher directory", func(t *testing.T) {
		page, err := f.svc.Teachers(f.ctx)
		if err != nil {
			t.Fatalf("Teachers: %v", err)
		}
		if len(page.Active) != 1 {
			t.Fatalf("active teachers = %d, want 1", len(page.Active))
		}
		active := page.Active[0]
		if active.Avatar.URL != wantTeacher || active.Avatar.Class != "app-avatar--sm" ||
			active.Avatar.Initials != "GU" {
			t.Errorf("active teacher avatar = %+v, want %s at app-avatar--sm", active.Avatar, wantTeacher)
		}
		// Pending rows keep the initials fallback (AVT-083): an unapproved
		// account cannot have uploaded a photo.
		if len(page.Pending) != 1 {
			t.Fatalf("pending teachers = %d, want 1", len(page.Pending))
		}
		pending := page.Pending[0]
		if pending.Avatar.URL != "" || pending.Avatar.Initials != "CG" {
			t.Errorf("pending teacher avatar = %+v, want the initials fallback", pending.Avatar)
		}
		if pending.Initials != "CG" {
			t.Errorf("pending teacher initials = %q, want CG", pending.Initials)
		}
	})
}
