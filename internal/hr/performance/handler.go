package performance

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// HandlerService is the subset of *Service used by the HTTP handler.
type HandlerService interface {
	CreateCycle(ctx context.Context, companyID int64, in CycleInput) (Cycle, error)
	ListCycles(ctx context.Context, companyID int64) ([]Cycle, error)
	GetCycle(ctx context.Context, companyID, cycleID int64) (Cycle, error)
	ActivateCycle(ctx context.Context, companyID, cycleID, actorID int64) error
	CloseCycle(ctx context.Context, companyID, cycleID, actorID int64) error
	AddParticipants(ctx context.Context, companyID, cycleID int64, employeeIDs []int64, actorID int64) (int, error)
	ListParticipants(ctx context.Context, companyID, cycleID int64) ([]Participant, error)
	GetParticipant(ctx context.Context, companyID, participantID int64) (Participant, error)
	ListGoals(ctx context.Context, companyID, participantID int64) ([]Goal, error)
	AddGoal(ctx context.Context, companyID, participantID, actorID int64, isAdmin bool, in GoalInput) (Goal, error)
	UpdateGoal(ctx context.Context, companyID, goalID, actorID int64, isAdmin bool, in GoalInput) error
	DeleteGoal(ctx context.Context, companyID, goalID, actorID int64, isAdmin bool) error
	SubmitSelfReview(ctx context.Context, companyID, participantID, actorID int64, in ReviewInput) error
	SubmitManagerReview(ctx context.Context, companyID, participantID, actorID int64, isAdmin bool, in ReviewInput) error
	Complete(ctx context.Context, companyID, participantID, actorID int64, finalRating int) (int64, error)
	MyReviews(ctx context.Context, companyID, userID int64) ([]Participant, error)
	TeamReviews(ctx context.Context, companyID, userID int64) ([]Participant, error)
}

// People lists the company's employees for the participant picker.
// *employees.Service satisfies it.
type People interface {
	List(ctx context.Context, companyID int64, filter ...employees.EmployeeFilter) ([]employees.Employee, error)
}

var (
	_ HandlerService = (*Service)(nil)
	_ People         = (*employees.Service)(nil)
)

// Handler serves /hr/performance.
type Handler struct {
	logger    *slog.Logger
	service   HandlerService
	people    People
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
	now       func() time.Time
}

// NewHandler builds a Handler. people may be nil (no participant picker
// then).
func NewHandler(l *slog.Logger, s HandlerService, p People, t *view.Engine, c *shared.CSRFManager, r rbac.Middleware) *Handler {
	if l == nil {
		l = slog.Default()
	}
	return &Handler{logger: l, service: s, people: p, templates: t, csrf: c, rbac: r, now: time.Now}
}

// MountRoutes mounts /hr/performance. Viewers reach their own and their
// team's reviews; the per-review checks (participant, reviewer, or admin)
// happen in the handlers. Cycle management and completion need
// hr.performance.admin.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHRPerformanceView, shared.PermHRPerformanceAdmin))
		r.Get("/", h.index)
		r.Get("/reviews/{pid}", h.review)
		r.Post("/reviews/{pid}/goals", h.addGoal)
		r.Post("/reviews/{pid}/goals/{gid}/update", h.updateGoal)
		r.Post("/reviews/{pid}/goals/{gid}/delete", h.deleteGoal)
		r.Post("/reviews/{pid}/self", h.submitSelf)
		r.Post("/reviews/{pid}/manager", h.submitManager)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHRPerformanceAdmin))
		r.Post("/cycles", h.createCycle)
		r.Get("/cycles/{id}", h.cycle)
		r.Post("/cycles/{id}/activate", h.activateCycle)
		r.Post("/cycles/{id}/close", h.closeCycle)
		r.Post("/cycles/{id}/participants", h.addParticipants)
		r.Post("/reviews/{pid}/complete", h.complete)
	})
}

// isAdmin reports whether the user holds hr.performance.admin.
func (h *Handler) isAdmin(r *http.Request, userID int64) bool {
	if h.rbac.Service == nil {
		return false
	}
	perms, err := h.rbac.Service.EffectivePermissions(r.Context(), userID)
	if err != nil {
		h.logger.Error("hr performance permissions", slog.Any("error", err))
		return false
	}
	for _, p := range perms {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "admin", "*", shared.PermHRPerformanceAdmin:
			return true
		}
	}
	return false
}

func (h *Handler) today() time.Time {
	if h.now == nil {
		return dateOnly(time.Now())
	}
	return dateOnly(h.now())
}

// flashError shows "hr: ..." domain errors to the user and hides the rest.
func flashError(err error) string {
	msg := err.Error()
	if strings.HasPrefix(msg, "hr: ") {
		if trimmed := strings.TrimSpace(strings.TrimPrefix(msg, "hr: ")); trimmed != "" {
			return strings.ToUpper(trimmed[:1]) + trimmed[1:]
		}
	}
	return shared.UserSafeMessage(err)
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request, to string, err error, success string) {
	if s := shared.SessionFromContext(r.Context()); s != nil {
		if err != nil {
			s.AddFlash(shared.FlashMessage{Kind: "error", Message: flashError(err)})
		} else {
			s.AddFlash(shared.FlashMessage{Kind: "success", Message: success})
		}
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, page, title string, data map[string]any) {
	var token string
	var flash *shared.FlashMessage
	if s := shared.SessionFromContext(r.Context()); s != nil {
		token, _ = h.csrf.EnsureToken(r.Context(), s)
		flash = s.PopFlash()
	}
	if err := h.templates.Render(w, page, view.TemplateData{
		Title:       title,
		CurrentPath: r.URL.Path,
		CSRFToken:   token,
		Flash:       flash,
		Data:        data,
	}); err != nil {
		h.logger.Error("render performance page", slog.String("page", page), slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

func pathID(r *http.Request, key string) int64 {
	id, err := strconv.ParseInt(chi.URLParam(r, key), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func reviewURL(id int64) string { return "/hr/performance/reviews/" + strconv.FormatInt(id, 10) }
func cycleURL(id int64) string  { return "/hr/performance/cycles/" + strconv.FormatInt(id, 10) }

const cyclesTabURL = "/hr/performance?tab=cycles"

// ReviewRow is a list row. FinalRating is set only when the viewer is
// entitled to it (reviewer or admin, or the participant once completed);
// list rows never carry self or manager ratings or comments.
type ReviewRow struct {
	ParticipantID, CycleID          int64
	CycleName, CycleStatus          string
	EmployeeName, ReviewerName      string
	Status                          string
	SelfReviewDue, ManagerReviewDue time.Time
	FinalRating                     *int
}

func toRow(p Participant, showFinal bool) ReviewRow {
	row := ReviewRow{
		ParticipantID: p.ID, CycleID: p.CycleID, CycleName: p.CycleName, CycleStatus: p.CycleStatus,
		EmployeeName: p.EmployeeName, ReviewerName: p.ReviewerName, Status: p.Status,
		SelfReviewDue: p.SelfReviewDue, ManagerReviewDue: p.ManagerReviewDue,
	}
	if showFinal {
		row.FinalRating = p.FinalRating
	}
	return row
}

// EmployeeOption is an employee in the participant picker (no sensitive
// fields).
type EmployeeOption struct {
	ID             int64
	EmployeeNumber string
	Name           string
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	isAdmin := h.isAdmin(r, ident.UserID)
	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	switch {
	case tab == "team":
	case tab == "cycles" && isAdmin:
	default:
		tab = "my"
	}

	var loadErr error
	mine, err := h.service.MyReviews(r.Context(), ident.CompanyID, ident.UserID)
	if err != nil {
		h.logger.Error("my performance reviews", slog.Any("error", err))
		loadErr = errors.Join(loadErr, err)
	}
	myRows := make([]ReviewRow, 0, len(mine))
	for _, p := range mine {
		myRows = append(myRows, toRow(p, p.Status == StageCompleted))
	}
	team, err := h.service.TeamReviews(r.Context(), ident.CompanyID, ident.UserID)
	if err != nil {
		h.logger.Error("team performance reviews", slog.Any("error", err))
		loadErr = errors.Join(loadErr, err)
	}
	teamRows := make([]ReviewRow, 0, len(team))
	for _, p := range team {
		teamRows = append(teamRows, toRow(p, true))
	}
	data := map[string]any{
		"Tab":         tab,
		"IsAdmin":     isAdmin,
		"MyReviews":   myRows,
		"TeamReviews": teamRows,
	}
	if isAdmin {
		cycles, cerr := h.service.ListCycles(r.Context(), ident.CompanyID)
		if cerr != nil {
			h.logger.Error("list performance cycles", slog.Any("error", cerr))
			loadErr = errors.Join(loadErr, cerr)
		}
		data["Cycles"] = cycles
	}
	data["Error"] = loadErr
	h.render(w, r, "pages/hr/performance.html", "Performance", data)
}

func (h *Handler) cycle(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id := pathID(r, "id")
	c, err := h.service.GetCycle(r.Context(), ident.CompanyID, id)
	if errors.Is(err, ErrNotFound) {
		shared.WriteHTTPError(w, http.StatusNotFound, "performance cycle not found")
		return
	}
	if err != nil {
		h.logger.Error("get performance cycle", slog.Int64("cycle_id", id), slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
		return
	}
	var loadErr error
	participants, err := h.service.ListParticipants(r.Context(), ident.CompanyID, c.ID)
	if err != nil {
		h.logger.Error("list performance participants", slog.Int64("cycle_id", c.ID), slog.Any("error", err))
		loadErr = err
	}
	rows := make([]ReviewRow, 0, len(participants))
	enrolled := make(map[int64]bool, len(participants))
	for _, p := range participants {
		rows = append(rows, toRow(p, true))
		enrolled[p.EmployeeID] = true
	}
	var options []EmployeeOption
	if h.people != nil && c.Status != CycleClosed {
		list, perr := h.people.List(r.Context(), ident.CompanyID, employees.EmployeeFilter{Status: "ACTIVE"})
		if perr != nil {
			h.logger.Error("performance employee options", slog.Any("error", perr))
			loadErr = errors.Join(loadErr, perr)
		}
		for _, e := range list {
			if !enrolled[e.ID] {
				options = append(options, EmployeeOption{ID: e.ID, EmployeeNumber: e.EmployeeNumber, Name: e.Name})
			}
		}
	}
	h.render(w, r, "pages/hr/performance_cycle.html", "Performance cycle - "+c.Name, map[string]any{
		"Cycle":        c,
		"Participants": rows,
		"Employees":    options,
		"CanActivate":  c.Status == CycleDraft,
		"CanClose":     c.Status == CycleActive,
		"CanEnroll":    c.Status != CycleClosed,
		"IsDraft":      c.Status == CycleDraft,
		"Error":        loadErr,
	})
}

// access describes how the current user relates to a participant.
type access struct {
	own, reviewer, admin bool
}

func (a access) any() bool { return a.own || a.reviewer || a.admin }

// canSeeManager reports whether the viewer may see manager ratings, scores,
// and comments.
func (a access) canSeeManager() bool { return a.reviewer || a.admin }

func accessFor(p Participant, userID int64, isAdmin bool) access {
	return access{own: sameUser(p.EmployeeUserID, userID), reviewer: sameUser(p.ReviewerUserID, userID), admin: isAdmin}
}

// loadParticipant resolves {pid} for the current user. It writes 404 when
// the participant does not exist in the company, or when the viewer is not
// the participant, the reviewer, or an admin (or the cycle is still a draft
// and the viewer is not an admin), and returns ok=false.
func (h *Handler) loadParticipant(w http.ResponseWriter, r *http.Request, ident shared.RequestIdentity) (Participant, access, bool) {
	pid := pathID(r, "pid")
	p, err := h.service.GetParticipant(r.Context(), ident.CompanyID, pid)
	if errors.Is(err, ErrNotFound) {
		shared.WriteHTTPError(w, http.StatusNotFound, "performance review not found")
		return Participant{}, access{}, false
	}
	if err != nil {
		h.logger.Error("get performance participant", slog.Int64("participant_id", pid), slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
		return Participant{}, access{}, false
	}
	a := accessFor(p, ident.UserID, h.isAdmin(r, ident.UserID))
	if !a.any() || (!a.admin && p.CycleStatus == CycleDraft) {
		shared.WriteHTTPError(w, http.StatusNotFound, "performance review not found")
		return Participant{}, access{}, false
	}
	return p, a, true
}

// GoalView is a goal with the scores the viewer is entitled to.
type GoalView struct {
	ID                              int64
	Seq                             int
	Title, Description, KPITarget   string
	Weight                          string
	SelfScore, ManagerScore         *int
	ShowSelfScore, ShowManagerScore bool
}

// ReviewView is a participant with only the fields the viewer is entitled
// to: self data for the participant (and for the reviewer/admin once
// submitted), manager data for the reviewer/admin, and the final rating for
// the reviewer/admin or the participant once completed.
type ReviewView struct {
	ID, CycleID                     int64
	CycleName, CycleStatus          string
	PeriodStart, PeriodEnd          time.Time
	SelfReviewDue, ManagerReviewDue time.Time
	EmployeeName, ReviewerName      string
	Status                          string

	ShowSelf        bool
	SelfRating      *int
	SelfComments    string
	SelfSubmittedAt *time.Time

	ShowManager        bool
	ManagerRating      *int
	ManagerComments    string
	ManagerSubmittedAt *time.Time

	ShowFinal   bool
	FinalRating *int
	CompletedAt *time.Time
}

func buildReviewView(p Participant, a access) ReviewView {
	v := ReviewView{
		ID: p.ID, CycleID: p.CycleID, CycleName: p.CycleName, CycleStatus: p.CycleStatus,
		PeriodStart: p.PeriodStart, PeriodEnd: p.PeriodEnd, SelfReviewDue: p.SelfReviewDue, ManagerReviewDue: p.ManagerReviewDue,
		EmployeeName: p.EmployeeName, ReviewerName: p.ReviewerName, Status: p.Status,
	}
	v.ShowSelf = a.own || (a.canSeeManager() && p.Status != StageEnrolled)
	if v.ShowSelf {
		v.SelfRating, v.SelfComments, v.SelfSubmittedAt = p.SelfRating, p.SelfComments, p.SelfSubmittedAt
	}
	v.ShowManager = a.canSeeManager()
	if v.ShowManager {
		v.ManagerRating, v.ManagerComments, v.ManagerSubmittedAt = p.ManagerRating, p.ManagerComments, p.ManagerSubmittedAt
	}
	v.ShowFinal = a.canSeeManager() || (a.own && p.Status == StageCompleted)
	if v.ShowFinal {
		v.FinalRating, v.CompletedAt = p.FinalRating, p.CompletedAt
	}
	return v
}

func buildGoalViews(goals []Goal, v ReviewView) []GoalView {
	out := make([]GoalView, 0, len(goals))
	for _, g := range goals {
		gv := GoalView{ID: g.ID, Seq: g.Seq, Title: g.Title, Description: g.Description, KPITarget: g.KPITarget, Weight: g.Weight,
			ShowSelfScore: v.ShowSelf, ShowManagerScore: v.ShowManager}
		if v.ShowSelf {
			gv.SelfScore = g.SelfScore
		}
		if v.ShowManager {
			gv.ManagerScore = g.ManagerScore
		}
		out = append(out, gv)
	}
	return out
}

func (h *Handler) review(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	p, a, ok := h.loadParticipant(w, r, ident)
	if !ok {
		return
	}
	goals, gerr := h.service.ListGoals(r.Context(), ident.CompanyID, p.ID)
	if gerr != nil {
		h.logger.Error("list performance goals", slog.Int64("participant_id", p.ID), slog.Any("error", gerr))
	}
	v := buildReviewView(p, a)
	active := p.CycleStatus == CycleActive
	selfOverdue := h.today().After(dateOnly(p.SelfReviewDue))
	defaultFinal := 0
	if v.ShowManager && v.ManagerRating != nil {
		defaultFinal = *v.ManagerRating
	}
	canManager := a.canSeeManager() && active &&
		(p.Status == StageSelfSubmitted || (p.Status == StageEnrolled && selfOverdue))
	h.render(w, r, "pages/hr/performance_review.html", "Performance review - "+p.EmployeeName, map[string]any{
		"Review":             v,
		"Goals":              buildGoalViews(goals, v),
		"IsOwn":              a.own,
		"IsReviewer":         a.reviewer,
		"IsAdmin":            a.admin,
		"CanEditGoals":       (a.own || a.admin) && active && p.Status != StageCompleted,
		"CanSelf":            a.own && active && p.Status == StageEnrolled,
		"CanManager":         canManager,
		"SelfPending":        a.canSeeManager() && active && p.Status == StageEnrolled && !selfOverdue,
		"CanComplete":        a.admin && p.CycleStatus != CycleDraft && p.Status == StageManagerSubmitted,
		"Ratings":            []int{1, 2, 3, 4, 5},
		"DefaultFinalRating": defaultFinal,
		"Error":              gerr,
	})
}

// requireParticipantAction loads {pid} and writes 403 unless allowed(access)
// holds. Missing participants are 404.
func (h *Handler) requireParticipantAction(w http.ResponseWriter, r *http.Request, ident shared.RequestIdentity, allowed func(access) bool) (Participant, access, bool) {
	pid := pathID(r, "pid")
	p, err := h.service.GetParticipant(r.Context(), ident.CompanyID, pid)
	if errors.Is(err, ErrNotFound) {
		shared.WriteHTTPError(w, http.StatusNotFound, "performance review not found")
		return Participant{}, access{}, false
	}
	if err != nil {
		h.logger.Error("get performance participant", slog.Int64("participant_id", pid), slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
		return Participant{}, access{}, false
	}
	a := accessFor(p, ident.UserID, h.isAdmin(r, ident.UserID))
	if !allowed(a) {
		shared.WriteHTTPError(w, http.StatusForbidden, "")
		return Participant{}, access{}, false
	}
	return p, a, true
}

func goalEditor(a access) bool { return a.own || a.admin }

func goalInput(r *http.Request) GoalInput {
	return GoalInput{
		Title:       r.FormValue("title"),
		Description: r.FormValue("description"),
		KPITarget:   r.FormValue("kpi_target"),
		Weight:      r.FormValue("weight"),
	}
}

func (h *Handler) addGoal(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	p, a, ok := h.requireParticipantAction(w, r, ident, goalEditor)
	if !ok {
		return
	}
	_, err := h.service.AddGoal(r.Context(), ident.CompanyID, p.ID, ident.UserID, a.admin, goalInput(r))
	h.redirect(w, r, reviewURL(p.ID), err, "Goal added")
}

func (h *Handler) updateGoal(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	p, a, ok := h.requireParticipantAction(w, r, ident, goalEditor)
	if !ok {
		return
	}
	err := h.service.UpdateGoal(r.Context(), ident.CompanyID, pathID(r, "gid"), ident.UserID, a.admin, goalInput(r))
	h.redirect(w, r, reviewURL(p.ID), err, "Goal updated")
}

func (h *Handler) deleteGoal(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	p, a, ok := h.requireParticipantAction(w, r, ident, goalEditor)
	if !ok {
		return
	}
	err := h.service.DeleteGoal(r.Context(), ident.CompanyID, pathID(r, "gid"), ident.UserID, a.admin)
	h.redirect(w, r, reviewURL(p.ID), err, "Goal deleted")
}

// reviewInput parses the overall rating, comments, and goal_score_<id>
// fields (empty scores are skipped).
func reviewInput(r *http.Request) (ReviewInput, error) {
	if err := r.ParseForm(); err != nil {
		return ReviewInput{}, ErrInvalidRating
	}
	rating, err := strconv.Atoi(strings.TrimSpace(r.PostForm.Get("rating")))
	if err != nil {
		return ReviewInput{}, ErrInvalidRating
	}
	in := ReviewInput{Rating: rating, Comments: r.PostForm.Get("comments"), GoalScores: map[int64]int{}}
	for key, values := range r.PostForm {
		rest, found := strings.CutPrefix(key, "goal_score_")
		if !found || len(values) == 0 || strings.TrimSpace(values[0]) == "" {
			continue
		}
		goalID, err := strconv.ParseInt(rest, 10, 64)
		if err != nil || goalID <= 0 {
			return ReviewInput{}, ErrGoalNotFound
		}
		score, err := strconv.Atoi(strings.TrimSpace(values[0]))
		if err != nil {
			return ReviewInput{}, ErrInvalidRating
		}
		in.GoalScores[goalID] = score
	}
	return in, nil
}

func (h *Handler) submitSelf(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	p, _, ok := h.requireParticipantAction(w, r, ident, func(a access) bool { return a.own })
	if !ok {
		return
	}
	in, err := reviewInput(r)
	if err == nil {
		err = h.service.SubmitSelfReview(r.Context(), ident.CompanyID, p.ID, ident.UserID, in)
	}
	h.redirect(w, r, reviewURL(p.ID), err, "Self review submitted")
}

func (h *Handler) submitManager(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	p, a, ok := h.requireParticipantAction(w, r, ident, access.canSeeManager)
	if !ok {
		return
	}
	in, err := reviewInput(r)
	if err == nil {
		err = h.service.SubmitManagerReview(r.Context(), ident.CompanyID, p.ID, ident.UserID, a.admin, in)
	}
	h.redirect(w, r, reviewURL(p.ID), err, "Manager review submitted")
}

func (h *Handler) complete(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	// The route group already requires hr.performance.admin; Complete itself
	// has no isAdmin parameter, so check again here defensively.
	if !h.isAdmin(r, ident.UserID) {
		shared.WriteHTTPError(w, http.StatusForbidden, "")
		return
	}
	pid := pathID(r, "pid")
	rating, err := strconv.Atoi(strings.TrimSpace(r.FormValue("final_rating")))
	if err != nil {
		err = ErrInvalidRating
	} else {
		_, err = h.service.Complete(r.Context(), ident.CompanyID, pid, ident.UserID, rating)
	}
	h.redirect(w, r, reviewURL(pid), err, "Review completed")
}

func parseDate(v string) (time.Time, bool) {
	d, err := time.Parse("2006-01-02", strings.TrimSpace(v))
	return d, err == nil
}

func (h *Handler) createCycle(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	in := CycleInput{Name: r.FormValue("name"), ActorID: ident.UserID}
	var ok1, ok2, ok3, ok4 bool
	in.PeriodStart, ok1 = parseDate(r.FormValue("period_start"))
	in.PeriodEnd, ok2 = parseDate(r.FormValue("period_end"))
	in.SelfReviewDue, ok3 = parseDate(r.FormValue("self_review_due"))
	in.ManagerReviewDue, ok4 = parseDate(r.FormValue("manager_review_due"))
	if !ok1 || !ok2 || !ok3 || !ok4 {
		h.redirect(w, r, cyclesTabURL, ErrInvalidCycle, "")
		return
	}
	c, err := h.service.CreateCycle(r.Context(), ident.CompanyID, in)
	if err != nil {
		h.redirect(w, r, cyclesTabURL, err, "")
		return
	}
	h.redirect(w, r, cycleURL(c.ID), nil, "Performance cycle created as a draft; add participants, then activate it")
}

func (h *Handler) activateCycle(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id := pathID(r, "id")
	err := h.service.ActivateCycle(r.Context(), ident.CompanyID, id, ident.UserID)
	h.redirect(w, r, cycleURL(id), err, "Performance cycle activated")
}

func (h *Handler) closeCycle(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id := pathID(r, "id")
	err := h.service.CloseCycle(r.Context(), ident.CompanyID, id, ident.UserID)
	h.redirect(w, r, cycleURL(id), err, "Performance cycle closed")
}

func (h *Handler) addParticipants(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id := pathID(r, "id")
	if err := r.ParseForm(); err != nil {
		h.redirect(w, r, cycleURL(id), ErrNoEmployees, "")
		return
	}
	var ids []int64
	for _, v := range r.PostForm["employee_ids"] {
		eid, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil || eid <= 0 {
			h.redirect(w, r, cycleURL(id), ErrEmployeeNotFound, "")
			return
		}
		ids = append(ids, eid)
	}
	added, err := h.service.AddParticipants(r.Context(), ident.CompanyID, id, ids, ident.UserID)
	h.redirect(w, r, cycleURL(id), err, strconv.Itoa(added)+" participant(s) added")
}
