package participation

// catalogue.go implements the PAR teacher read surfaces: the anonymous
// participant list (PAR-163…PAR-172), the anonymous edit form (PAR-173…PAR-178)
// and the majors catalogue (PAR-179…PAR-188). Every page is built inside one
// read-only repeatable-read snapshot (Service.readTx) so the rows, counts and
// pagination describe the same moment.
//
// The anonymous list is one row per live identity (PAR-164), never one row per
// registration. Search and sort therefore run on the identity row and its
// aggregates, not on a name grouping.

import (
	"context"
	"database/sql"
	"net/url"
	"strings"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// anonymousPerPage is the pinned management page size for the anonymous list
// (PAR-168). It is deliberately not the fixture page size.
const anonymousPerPage = 10

// anonymousQueryCap is the rune ceiling applied to ?q= (PAR-165).
const anonymousQueryCap = 100

// AnonymousListInput is the GET /teacher/anonymous material. Query and Page are
// raw query values: the search is trimmed/capped, and a bad page clamps, never
// a 422 (PAR-165, PAR-168, §5.0.5).
type AnonymousListInput struct {
	Actor ActorInput
	Query string
	Page  string
}

// AnonymousFormInput is the GET /teacher/anonymous/:id material. ID is an
// anonymous_participants.id, not a registration id (PAR-096, PAR-171).
type AnonymousFormInput struct {
	Actor ActorInput
	ID    int64
}

// AnonymousList serves GET /teacher/anonymous (PAR-163…PAR-172).
func (s *Service) AnonymousList(ctx context.Context, in AnonymousListInput) (viewmodel.AnonymousList, error) {
	var out viewmodel.AnonymousList
	search := truncateRunes(strings.TrimSpace(in.Query), anonymousQueryCap)
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		if _, err := teacherActor(ctx, tx, in.Actor, "PAR-163"); err != nil {
			return err
		}

		where := "ap.deleted_at = ?"
		whereArgs := []any{store.LiveAt}
		if search != "" {
			where += ` AND ap.display_name LIKE ? ESCAPE '\\'`
			whereArgs = append(whereArgs, likePattern(search))
		}

		var total int
		countArgs := append([]any{}, whereArgs...)
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM anonymous_participants ap WHERE "+where,
			countArgs...).Scan(&total); err != nil {
			return internal(err)
		}

		page := clampPage(in.Page, anonymousPerPage, total)
		offset := (page - 1) * anonymousPerPage

		// Last activity is the maximum of the identity's own last_seen_at, its
		// live registrations' created_at, and its live attempts' updated_at /
		// submitted_at (PAR-166). created_at is never null, so the expression is
		// always a real timestamp.
		rowsSQL := `
			SELECT ap.id, ap.display_name, ap.grade_level, m.name,
			       COALESCE(reg.joined, 0) AS quizzes_joined,
			       GREATEST(
			           COALESCE(ap.last_seen_at, ap.created_at),
			           COALESCE(reg.last_reg, '1970-01-01 00:00:00'),
			           COALESCE(act.last_attempt, '1970-01-01 00:00:00')
			       ) AS last_activity
			FROM anonymous_participants ap
			LEFT JOIN majors m ON m.id = ap.major_id AND m.deleted_at = ?
			LEFT JOIN (
			    SELECT r.anonymous_participant_id AS pid,
			           COUNT(DISTINCT r.quiz_id) AS joined,
			           MAX(r.created_at) AS last_reg
			    FROM registrations r
			    WHERE r.deleted_at = ?
			      AND r.anonymous_participant_id IS NOT NULL
			    GROUP BY r.anonymous_participant_id
			) reg ON reg.pid = ap.id
			LEFT JOIN (
			    SELECT r.anonymous_participant_id AS pid,
			           MAX(GREATEST(qa.updated_at, COALESCE(qa.submitted_at, qa.updated_at))) AS last_attempt
			    FROM quiz_attempts qa
			    JOIN registrations r ON r.id = qa.registration_id
			    WHERE qa.deleted_at = ?
			      AND r.deleted_at = ?
			      AND r.anonymous_participant_id IS NOT NULL
			    GROUP BY r.anonymous_participant_id
			) act ON act.pid = ap.id
			WHERE ` + where + `
			ORDER BY last_activity DESC, ap.id ASC
			LIMIT ? OFFSET ?`
		rowArgs := []any{store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt}
		rowArgs = append(rowArgs, whereArgs...)
		rowArgs = append(rowArgs, anonymousPerPage, offset)

		rows, err := tx.QueryContext(ctx, rowsSQL, rowArgs...)
		if err != nil {
			return internal(err)
		}
		defer rows.Close()

		items := []viewmodel.AnonymousRow{}
		for rows.Next() {
			var (
				id        int64
				grade     int
				majorName sql.NullString
				joined    int
				last      time.Time
			)
			var row viewmodel.AnonymousRow
			if err := rows.Scan(&id, &row.FullName, &grade, &majorName, &joined, &last); err != nil {
				return internal(err)
			}
			row.ID = id
			row.GradeLevel = grade
			row.MajorName = majorName.String
			row.ClassLabel = viewmodel.ClassLabel(grade, majorName.String)
			row.QuizzesJoined = joined
			local := viewmodel.NewLocalTime(last)
			row.LastActivity = &local
			row.EditHref = anonymousPath(uint32(id))
			row.DeleteHref = anonymousPath(uint32(id)) + "/delete"
			items = append(items, row)
		}
		if err := rows.Err(); err != nil {
			return internal(err)
		}

		q := url.Values{}
		if search != "" {
			q.Set("q", search)
		}
		out = viewmodel.AnonymousList{
			Rows: viewmodel.List[viewmodel.AnonymousRow]{
				Items:   items,
				Total:   total,
				Page:    page,
				PerPage: anonymousPerPage,
			},
			Pagination: viewmodel.NewPagination("/teacher/anonymous", q, page, anonymousPerPage, total),
			Query:      viewmodel.QueryState{Q: search},
			Empty:      anonymousEmpty(search != ""),
		}
		return nil
	})
	return out, err
}

// AnonymousFormPage serves GET /teacher/anonymous/:id (PAR-173…PAR-178). Only a
// live row prefills the form; anything else is the shared 404.
func (s *Service) AnonymousFormPage(ctx context.Context, in AnonymousFormInput) (viewmodel.AnonymousForm, error) {
	var form viewmodel.AnonymousForm
	if in.ID <= 0 {
		return form, apperr.NotFoundf().WithRule("PAR-173")
	}
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		if _, err := teacherActor(ctx, tx, in.Actor, "PAR-173"); err != nil {
			return err
		}
		loaded, err := readAnonymousForm(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		form = loaded
		return nil
	})
	return form, err
}

// MajorsPageInput is the GET /teacher/majors material.
type MajorsPageInput struct {
	Actor ActorInput
}

// MajorsPage serves GET /teacher/majors (PAR-179…PAR-188).
func (s *Service) MajorsPage(ctx context.Context, in MajorsPageInput) (viewmodel.MajorsPage, error) {
	var out viewmodel.MajorsPage
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		if _, err := teacherActor(ctx, tx, in.Actor, "PAR-179"); err != nil {
			return err
		}
		page, err := readMajorsPage(ctx, tx)
		if err != nil {
			return err
		}
		out = page
		return nil
	})
	return out, err
}

// teacherActor admits only an active teacher session to a catalogue surface.
// The resolution runs inside the snapshot or write transaction so a session
// change cannot race the check. Any other actor is the same 404.
func teacherActor(ctx context.Context, q store.Querier, in ActorInput, rule string) (Actor, error) {
	actor, err := resolveActor(ctx, q, in)
	if err != nil {
		return Actor{}, err
	}
	if !actor.IsTeacher() {
		return Actor{}, apperr.NotFoundf().WithRule(rule)
	}
	return actor, nil
}

// anonymousEmpty is the PAR-169 copy, byte for byte. Only the filtered variant
// carries a reset action.
func anonymousEmpty(filtered bool) viewmodel.EmptyState {
	if !filtered {
		return viewmodel.EmptyState{
			Title: "Belum ada peserta anonim.",
			Body:  "Peserta yang bergabung tanpa akun akan muncul di sini.",
			Icon:  "users",
		}
	}
	reset := viewmodel.Outline("Atur ulang pencarian", "/teacher/anonymous")
	return viewmodel.EmptyState{
		Title:  "Tidak ada peserta anonim yang cocok.",
		Body:   "Coba kata kunci lain atau atur ulang pencarian.",
		Icon:   "users",
		Action: &reset,
	}
}

// readMajorsPage builds the catalogue from one snapshot: live majors in name
// order, three grouped live-reference counts, and the PAR-184 empty copy.
func readMajorsPage(ctx context.Context, q store.Querier) (viewmodel.MajorsPage, error) {
	majors, err := readMajors(ctx, q)
	if err != nil {
		return viewmodel.MajorsPage{}, err
	}
	students, anons, quizzes, err := readMajorReferenceMaps(ctx, q)
	if err != nil {
		return viewmodel.MajorsPage{}, err
	}
	rows := make([]viewmodel.MajorRow, 0, len(majors))
	for _, m := range majors {
		st, an, qz := students[m.ID], anons[m.ID], quizzes[m.ID]
		rows = append(rows, viewmodel.MajorRow{
			ID:           m.ID,
			Name:         m.Name,
			StudentCount: st,
			QuizCount:    qz,
			CanDelete:    st == 0 && an == 0 && qz == 0,
			RenameAction: "/teacher/majors/" + itoa(uint64(m.ID)),
			DeleteAction: "/teacher/majors/" + itoa(uint64(m.ID)) + "/delete",
		})
	}
	return viewmodel.MajorsPage{
		Majors:       rows,
		CreateAction: "/teacher/majors",
		Empty: viewmodel.EmptyState{
			Title: "Belum ada jurusan.",
			Body:  "Tambahkan jurusan pertama agar siswa dan kuis dapat memakainya.",
			Icon:  "bank",
		},
	}, nil
}

// readMajorReferenceMaps counts every live reference family in one grouped
// query each (PAR-181). Draft quizzes count, and so does every non-deleted
// quiz state (OQ-12). The anonymous count is folded into CanDelete because the
// row model has no separate field for it.
func readMajorReferenceMaps(ctx context.Context, q store.Querier) (students, anons, quizzes map[int64]int, err error) {
	students, err = readCountMap(ctx, q,
		"SELECT major_id, COUNT(*) FROM students WHERE deleted_at = ? GROUP BY major_id")
	if err != nil {
		return nil, nil, nil, err
	}
	anons, err = readCountMap(ctx, q,
		"SELECT major_id, COUNT(*) FROM anonymous_participants WHERE deleted_at = ? GROUP BY major_id")
	if err != nil {
		return nil, nil, nil, err
	}
	quizzes, err = readCountMap(ctx, q, `
		SELECT target_major_id, COUNT(*)
		FROM quizzes
		WHERE target_major_id IS NOT NULL AND deleted_at = ?
		GROUP BY target_major_id`)
	if err != nil {
		return nil, nil, nil, err
	}
	return students, anons, quizzes, nil
}

// readCountMap runs one grouped count and returns it keyed by major id.
func readCountMap(ctx context.Context, q store.Querier, query string) (map[int64]int, error) {
	rows, err := q.QueryContext(ctx, query, store.LiveAt)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, internal(err)
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return out, nil
}
