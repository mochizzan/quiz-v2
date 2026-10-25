package authoring

import (
	"context"
	"database/sql"
	"net/url"
	"sort"
	"strings"

	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// quizListPageSize is the pinned cards-per-page value (AUT-051).
const quizListPageSize = 6

// MaxSearchRunes is the search-box cap: excess input is ignored, never a 422
// (AUT-001, AUT-023).
const MaxSearchRunes = 100

// ListInput is the raw list query (AUT-001). Every value is normalised, never
// rejected: a GET never answers 422 for a query parameter.
type ListInput struct {
	State string
	Query string
	Sort  string
	Page  int
}

// List builds GET /teacher/quizzes from one read snapshot: the tab counts, the
// filtered page and the tab badges cannot disagree within one response.
func (s *Service) List(ctx context.Context, ownerID int64, in ListInput) (viewmodel.TeacherQuizList, error) {
	var out viewmodel.TeacherQuizList
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = buildQuizList(ctx, tx, ownerID, in)
		return err
	})
	if err != nil {
		return viewmodel.TeacherQuizList{}, err
	}
	return out, nil
}

func buildQuizList(ctx context.Context, q store.Querier, ownerID int64, in ListInput) (viewmodel.TeacherQuizList, error) {
	tab := NormalizeTab(in.State)
	order := NormalizeSort(in.Sort)
	search := TruncateRunes(strings.TrimSpace(in.Query), MaxSearchRunes)
	page := in.Page
	if page < 1 {
		page = 1
	}

	counts, total, err := readStateCounts(ctx, q, ownerID)
	if err != nil {
		return viewmodel.TeacherQuizList{}, err
	}

	rows, err := readListRows(ctx, q, ownerID, tab, search, order)
	if err != nil {
		return viewmodel.TeacherQuizList{}, err
	}

	items := make([]viewmodel.QuizCard, 0, len(rows))
	for _, row := range rows {
		items = append(items, quizCard(row))
	}

	list := viewmodel.NewList(items, page, quizListPageSize)
	values := listQueryValues(tab, search, order)
	pagination := viewmodel.NewPagination(listPath, values, list.Page, quizListPageSize, list.Total)

	return viewmodel.TeacherQuizList{
		Quizzes:    list,
		Pagination: pagination,
		Tabs: []viewmodel.FilterTab{
			{Label: "Semua", Href: quizListHref("all", search, order), Active: tab == "all", Count: total},
			{Label: "Draf", Href: quizListHref("draft", search, order), Active: tab == "draft", Count: counts["draft"]},
			{Label: "Dibuka", Href: quizListHref("open", search, order), Active: tab == "open", Count: counts["open"]},
			{Label: "Ditutup", Href: quizListHref("closed", search, order), Active: tab == "closed", Count: counts["closed"]},
		},
		Query:      viewmodel.QueryState{Q: search, Tab: tab, Sort: order},
		Empty:      quizListEmpty(tab, search),
		CreateHref: "/teacher/quizzes/new",
	}, nil
}

// readStateCounts returns the owner-scoped totals by state plus the sum. Counts
// ignore the search box and the active tab (AUT-050).
func readStateCounts(ctx context.Context, q store.Querier, ownerID int64) (map[string]int, int, error) {
	rows, err := q.QueryContext(ctx, `SELECT state, COUNT(*) FROM quizzes
		WHERE owner_account_id = ? AND deleted_at = ? GROUP BY state`, ownerID, store.LiveAt)
	if err != nil {
		return nil, 0, internal(err)
	}
	defer rows.Close()

	counts := map[string]int{}
	total := 0
	for rows.Next() {
		var (
			state string
			n     int
		)
		if err := rows.Scan(&state, &n); err != nil {
			return nil, 0, internal(err)
		}
		counts[state] = n
		total += n
	}
	if err := rows.Err(); err != nil {
		return nil, 0, internal(err)
	}
	return counts, total, nil
}

// listRow is one card's database row. The nullable columns stay nullable.
type listRow struct {
	ID            int64
	Title         string
	Description   string
	Visibility    string
	State         string
	Code          sql.NullString
	StartsAt      sql.NullTime
	EndsAt        sql.NullTime
	TargetGrade   sql.NullInt64
	TargetMajorID sql.NullInt64
	MajorName     string
	CreatedAt     sql.NullTime
	QuestionCount int
}

// readListRows reads every matching live row. The teacher's own quiz count is
// small, and loading all rows lets one snapshot feed the page and its clamped
// pagination without a second COUNT query.
func readListRows(ctx context.Context, q store.Querier, ownerID int64, tab, search string, order viewmodel.SortOrder) ([]listRow, error) {
	query := `SELECT q.id, q.title, q.description, q.visibility, q.state, q.code,
		q.starts_at, q.ends_at, q.target_grade, q.target_major_id,
		COALESCE(m.name, ''), q.created_at,
		(SELECT COUNT(*) FROM questions z WHERE z.quiz_id = q.id AND z.deleted_at = ?)
		FROM quizzes q LEFT JOIN majors m ON m.id = q.target_major_id
		WHERE q.owner_account_id = ? AND q.deleted_at = ?`
	args := []any{store.LiveAt, ownerID, store.LiveAt}
	if tab != "all" {
		query += " AND q.state = ?"
		args = append(args, tab)
	}
	if search != "" {
		// LOCATE is a literal substring test: no LIKE wildcards can be injected
		// by the search box, and the title column collation is already
		// case-insensitive.
		query += " AND LOCATE(?, LOWER(q.title)) > 0"
		args = append(args, strings.ToLower(search))
	}
	query += " ORDER BY " + listOrderClause(order)

	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()

	var out []listRow
	for rows.Next() {
		var row listRow
		if err := rows.Scan(&row.ID, &row.Title, &row.Description, &row.Visibility, &row.State,
			&row.Code, &row.StartsAt, &row.EndsAt, &row.TargetGrade, &row.TargetMajorID,
			&row.MajorName, &row.CreatedAt, &row.QuestionCount); err != nil {
			return nil, internal(err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return out, nil
}

// listOrderClause is the closed sort whitelist with the fixed tie-breaks. It is
// a constant string, never user text (AUT-051).
func listOrderClause(order viewmodel.SortOrder) string {
	switch order {
	case viewmodel.SortOldest:
		return "q.created_at ASC, q.id ASC"
	case viewmodel.SortNameAsc:
		return "LOWER(q.title) ASC, q.id ASC"
	case viewmodel.SortNameDesc:
		return "LOWER(q.title) DESC, q.id DESC"
	default:
		return "q.created_at DESC, q.id DESC"
	}
}

// quizCard projects one row into the list card, including the state-dependent
// actions pinned by AUT-001.
func quizCard(row listRow) viewmodel.QuizCard {
	href := quizPath(row.ID)
	card := viewmodel.QuizCard{
		ID:            row.ID,
		Href:          href,
		Title:         row.Title,
		Description:   row.Description,
		QuestionCount: row.QuestionCount,
		State:         viewmodel.QuizState(row.State),
		Visibility:    viewmodel.Visibility(row.Visibility),
		Audience:      audience(row.Visibility, row.TargetGrade, row.TargetMajorID, row.MajorName),
		Code:          codePtr(row.Code),
		StartsAt:      localPtr(row.StartsAt),
		EndsAt:        localPtr(row.EndsAt),
	}
	switch card.State {
	case viewmodel.QuizDraft:
		card.Primary = viewmodel.Primary("Kelola", href)
	case viewmodel.QuizOpen:
		card.Primary = viewmodel.Primary("Monitor", quizPath(row.ID)+"/monitor")
		secondary := viewmodel.Outline("Hasil", quizPath(row.ID)+"/results")
		card.Secondary = &secondary
	case viewmodel.QuizClosed:
		card.Primary = viewmodel.Primary("Hasil", quizPath(row.ID)+"/results")
	}
	return card
}

// quizListEmpty returns the exact empty-state copy for the active filter.
func quizListEmpty(tab, search string) viewmodel.EmptyState {
	empty := viewmodel.EmptyState{
		Title:  "Belum ada kuis",
		Body:   "Kuis yang Anda buat akan muncul di sini. Mulai dari satu draf, lalu tambahkan soal.",
		Icon:   "quiz",
		Action: actionPtr(viewmodel.Primary("Buat kuis", "/teacher/quizzes/new")),
	}
	switch {
	case search != "":
		empty = viewmodel.EmptyState{
			Title:  "Tidak ada kuis yang cocok",
			Body:   "Coba kata kunci lain atau hapus saringan yang aktif.",
			Icon:   "search",
			Action: actionPtr(viewmodel.Outline("Hapus pencarian", "/teacher/quizzes")),
		}
	case tab != "all":
		empty = viewmodel.EmptyState{
			Title:  "Belum ada kuis di tab ini",
			Body:   "Kuis dengan status ini akan muncul di sini.",
			Icon:   "quiz",
			Action: actionPtr(viewmodel.Outline("Lihat semua kuis", "/teacher/quizzes")),
		}
	}
	return empty
}

// listQueryValues echoes only non-default filters into pagination links.
func listQueryValues(tab, search string, order viewmodel.SortOrder) url.Values {
	values := url.Values{}
	if search != "" {
		values.Set("q", search)
	}
	if tab != "all" {
		values.Set("state", tab)
	}
	if order != viewmodel.SortNewest {
		values.Set("sort", string(order))
	}
	return values
}

// quizListHref builds one tab link with the active search and sort preserved.
func quizListHref(tab, search string, order viewmodel.SortOrder) string {
	encoded := listQueryValues(tab, search, order).Encode()
	if encoded == "" {
		return listPath
	}
	return listPath + "?" + encoded
}

// NormalizeTab maps the state query to the closed tab set (AUT-001).
func NormalizeTab(state string) string {
	switch s := strings.TrimSpace(state); s {
	case "draft", "open", "closed":
		return s
	default:
		return "all"
	}
}

// NormalizeSort maps the sort query to the closed order set (AUT-051).
func NormalizeSort(order string) viewmodel.SortOrder {
	switch s := viewmodel.SortOrder(strings.TrimSpace(order)); s {
	case viewmodel.SortOldest, viewmodel.SortNameAsc, viewmodel.SortNameDesc:
		return s
	default:
		return viewmodel.SortNewest
	}
}

// Detail builds GET /teacher/quizzes/:id from one snapshot (AUT-004/AUT-052).
// The teacher name is the actor's own display name, supplied by the session.
func (s *Service) Detail(ctx context.Context, ownerID, quizID int64, teacherName string) (viewmodel.QuizDetail, error) {
	var out viewmodel.QuizDetail
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = buildDetail(ctx, tx, ownerID, quizID, teacherName, s.uploads)
		return err
	})
	if err != nil {
		return viewmodel.QuizDetail{}, err
	}
	return out, nil
}

func buildDetail(ctx context.Context, q store.Querier, ownerID, quizID int64, teacherName, uploadsDir string) (viewmodel.QuizDetail, error) {
	row, err := readOwnedQuiz(ctx, q, quizID, ownerID, false)
	if err != nil {
		return viewmodel.QuizDetail{}, err
	}
	majors, err := readMajorNames(ctx, q)
	if err != nil {
		return viewmodel.QuizDetail{}, err
	}
	questions, err := readQuestions(ctx, q, quizID)
	if err != nil {
		return viewmodel.QuizDetail{}, err
	}

	// The detail page and the publish command run the same gate, so the page
	// must resolve major liveness in its own snapshot too; otherwise every
	// private draft would report a missing target (AUT-025/AUT-027).
	majorLive := false
	if viewmodel.Visibility(row.Visibility) == viewmodel.VisibilityPrivate && row.TargetMajorID.Valid {
		majorLive, err = readMajorLive(ctx, q, row.TargetMajorID.Int64)
		if err != nil {
			return viewmodel.QuizDetail{}, err
		}
	}

	rows := make([]viewmodel.QuestionRow, 0, len(questions))
	for _, item := range questions {
		rows = append(rows, viewmodel.QuestionRow{
			ID:           item.ID,
			Position:     item.Position,
			Type:         viewmodel.QuestionType(item.Kind),
			Body:         item.Prompt,
			OptionCount:  len(item.Options),
			HasImage:     item.ImagePath.Valid && item.ImagePath.String != "",
			CorrectLabel: correctLabel(item),
			EditHref:     questionPath(quizID, item.ID),
			DeleteHref:   questionPath(quizID, item.ID) + "/delete",
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Position != rows[j].Position {
			return rows[i].Position < rows[j].Position
		}
		return rows[i].ID < rows[j].ID
	})

	detail := viewmodel.QuizDetail{
		Quiz:               viewmodel.QuizRef{ID: row.ID, Title: row.Title},
		Description:        row.Description,
		TeacherName:        teacherName,
		State:              viewmodel.QuizState(row.State),
		Visibility:         viewmodel.Visibility(row.Visibility),
		Audience:           audience(row.Visibility, row.TargetGrade, row.TargetMajorID, majors[int64OrZero(row.TargetMajorID)]),
		Code:               codePtr(row.Code),
		InviteURL:          "/invite/" + row.InviteToken,
		StartsAt:           localPtr(row.StartsAt),
		EndsAt:             localPtr(row.EndsAt),
		CreatedAt:          viewmodel.NewLocalTime(row.CreatedAt),
		QuestionCount:      len(questions),
		Questions:          rows,
		Editable:           row.State == string(viewmodel.QuizDraft),
		ResultsHref:        quizPath(quizID) + "/results",
		MonitorHref:        quizPath(quizID) + "/monitor",
		AddQuestionHref:    quizPath(quizID) + "/questions/new",
		RandomizeQuestions: row.RandomizeQuestions,
		RandomizeAnswers:   row.RandomizeOptions,
		Settings: viewmodel.QuizSettings{
			MaxAttempts:        row.MaxAttempts,
			ReviewMode:         viewmodel.ReviewMode(row.ReviewMode),
			ShowScore:          row.ShowScore,
			RandomizeQuestions: row.RandomizeQuestions,
			RandomizeAnswers:   row.RandomizeOptions,
			AllowRetake:        row.AllowRetake,
		},
		Empty: viewmodel.EmptyState{
			Title:  "Belum ada soal",
			Body:   "Tambahkan soal pertama. Kuis dapat dibuka setelah memiliki minimal satu soal.",
			Icon:   "quiz",
			Action: actionPtr(viewmodel.Primary("Tambah soal", quizPath(quizID)+"/questions/new")),
		},
	}
	detail.CanClose = detail.State == viewmodel.QuizOpen

	if detail.Editable {
		detail.CanPublish = true
		result := EvaluatePublish(PublishInput{
			Quiz: PublishQuiz{
				Visibility:         row.Visibility,
				Code:               row.Code,
				InviteToken:        row.InviteToken,
				TargetGrade:        row.TargetGrade,
				TargetMajorID:      row.TargetMajorID,
				MajorLive:          majorLive,
				AllowRetake:        row.AllowRetake,
				MaxAttempts:        row.MaxAttempts,
				ShowScore:          row.ShowScore,
				ReviewMode:         row.ReviewMode,
				RandomizeQuestions: row.RandomizeQuestions,
				RandomizeOptions:   row.RandomizeOptions,
			},
			Questions:  toPublishQuestions(questions),
			UploadsDir: uploadsDir,
		})
		if result.Err != nil {
			return viewmodel.QuizDetail{}, internal(result.Err)
		}
		if !result.OK {
			detail.CanPublish = false
			detail.PublishBlockedReason = ReasonText(result.Reason)
		}

		publish := viewmodel.Primary("Buka kuis", quizPath(quizID)+"/publish")
		detail.PublishAction = &publish
		remove := viewmodel.Danger("Hapus kuis", quizPath(quizID)+"/delete")
		detail.DeleteAction = &remove
	}
	if detail.CanClose {
		closeAction := viewmodel.Outline("Tutup kuis", quizPath(quizID)+"/close")
		detail.CloseAction = &closeAction
	}
	return detail, nil
}

// NewQuizForm builds GET /teacher/quizzes/new with the live major list in the
// same snapshot (AUT-003).
func (s *Service) NewQuizForm(ctx context.Context) (viewmodel.QuizForm, error) {
	var out viewmodel.QuizForm
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		majors, err := readMajorOptions(ctx, tx)
		if err != nil {
			return err
		}
		out = viewmodel.QuizForm{
			Visibility: viewmodel.VisibilityPublic,
			Majors:     majors,
			Action:     listPath,
			CancelHref: listPath,
		}
		return nil
	})
	if err != nil {
		return viewmodel.QuizForm{}, err
	}
	return out, nil
}

// readMajorOptions returns live majors by name then id for the create select.
func readMajorOptions(ctx context.Context, q store.Querier) ([]viewmodel.MajorRef, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT id, name FROM majors WHERE deleted_at = ? ORDER BY name ASC, id ASC", store.LiveAt)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()

	var out []viewmodel.MajorRef
	for rows.Next() {
		var item viewmodel.MajorRef
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			return nil, internal(err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return out, nil
}

// actionPtr is the local pointer helper for optional empty-state actions.
func actionPtr(a viewmodel.CardAction) *viewmodel.CardAction { return &a }

// int64OrZero unwraps a nullable id for map lookups.
func int64OrZero(v sql.NullInt64) int64 {
	if !v.Valid {
		return 0
	}
	return v.Int64
}
