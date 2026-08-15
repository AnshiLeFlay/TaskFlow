package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/example/taskflow/backend/internal/application"
	"github.com/example/taskflow/backend/internal/domain"
	"github.com/example/taskflow/backend/internal/infrastructure/auth"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type userContextKey struct{}

type API struct {
	service   *application.Service
	validator application.TokenValidator
	logger    *slog.Logger
}

// Request DTOs are named so the annotation-generated Swagger document remains
// useful to clients as well as the hand-reviewed OpenAPI 3 contract.
type CreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type AddMemberRequest struct {
	UserID string             `json:"user_id"`
	Role   domain.ProjectRole `json:"role"`
}

// MemberSummary is the response shape for the project members list: the
// minimal identity a client needs to build an assignee picker.
type MemberSummary struct {
	UserID string             `json:"user_id"`
	Role   domain.ProjectRole `json:"role"`
}

type StatusRequest struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
}

type CreateBoardRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Statuses    []StatusRequest `json:"statuses,omitempty"`
}

type UpdateStatusRequest struct {
	Name     *string `json:"name,omitempty"`
	Position *int    `json:"position,omitempty"`
}

type CreateRuleRequest struct {
	FromStatusID string                `json:"from_status_id"`
	ToStatusID   string                `json:"to_status_id"`
	Conditions   domain.RuleConditions `json:"conditions"`
}

type UpdateRuleRequest struct {
	FromStatusID *string                `json:"from_status_id,omitempty"`
	ToStatusID   *string                `json:"to_status_id,omitempty"`
	Conditions   *domain.RuleConditions `json:"conditions,omitempty"`
}

type CreateTaskRequest struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	StatusID    string     `json:"status_id,omitempty"`
	AssigneeID  *string    `json:"assignee_id,omitempty"`
	Deadline    *time.Time `json:"deadline,omitempty"`
}

type UpdateTaskRequest struct {
	Title       *string    `json:"title,omitempty"`
	Description *string    `json:"description,omitempty"`
	AssigneeID  *string    `json:"assignee_id,omitempty"`
	Deadline    *time.Time `json:"deadline,omitempty"`
}

type CreateCommentRequest struct {
	Body string `json:"body"`
}

type TransitionTaskRequest struct {
	TargetStatusID string `json:"target_status_id"`
}

func NewRouter(service *application.Service, validator application.TokenValidator, wsHandler http.Handler, swaggerDir string, allowedOrigins []string, logger *slog.Logger) http.Handler {
	api := &API{service: service, validator: validator, logger: logger}
	router := mux.NewRouter()
	router.Use(api.recoverPanic)
	router.Use(api.accessLog)

	router.HandleFunc("/healthz", api.health).Methods(http.MethodGet)
	router.HandleFunc("/readyz", api.ready).Methods(http.MethodGet)
	router.Handle("/ws", wsHandler).Methods(http.MethodGet)
	router.HandleFunc("/swagger", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger/", http.StatusTemporaryRedirect)
	}).Methods(http.MethodGet)
	router.PathPrefix("/swagger/").Handler(http.StripPrefix("/swagger/", http.FileServer(http.Dir(swaggerDir))))

	v1 := router.PathPrefix("/api/v1").Subrouter()
	v1.Use(api.authenticate)
	v1.HandleFunc("/me", api.me).Methods(http.MethodGet)
	v1.HandleFunc("/projects", api.listProjects).Methods(http.MethodGet)
	v1.HandleFunc("/projects", api.createProject).Methods(http.MethodPost)
	v1.HandleFunc("/projects/{projectId}/members", api.listMembers).Methods(http.MethodGet)
	v1.HandleFunc("/projects/{projectId}/members", api.addMember).Methods(http.MethodPost)
	v1.HandleFunc("/projects/{projectId}/boards", api.listBoards).Methods(http.MethodGet)
	v1.HandleFunc("/projects/{projectId}/boards", api.createBoard).Methods(http.MethodPost)
	v1.HandleFunc("/boards/{boardId}", api.getBoard).Methods(http.MethodGet)
	v1.HandleFunc("/boards/{boardId}/statuses", api.createStatus).Methods(http.MethodPost)
	v1.HandleFunc("/statuses/{statusId}", api.updateStatus).Methods(http.MethodPatch)
	v1.HandleFunc("/statuses/{statusId}", api.deleteStatus).Methods(http.MethodDelete)
	v1.HandleFunc("/boards/{boardId}/rules", api.createRule).Methods(http.MethodPost)
	v1.HandleFunc("/rules/{ruleId}", api.updateRule).Methods(http.MethodPatch)
	v1.HandleFunc("/rules/{ruleId}", api.deleteRule).Methods(http.MethodDelete)
	v1.HandleFunc("/boards/{boardId}/tasks", api.listTasks).Methods(http.MethodGet)
	v1.HandleFunc("/boards/{boardId}/tasks", api.createTask).Methods(http.MethodPost)
	v1.HandleFunc("/tasks/{taskId}", api.updateTask).Methods(http.MethodPatch)
	v1.HandleFunc("/tasks/{taskId}/comments", api.createComment).Methods(http.MethodPost)
	v1.HandleFunc("/tasks/{taskId}/transition", api.transitionTask).Methods(http.MethodPost)

	router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeError(w, domain.ErrNotFound) })
	router.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusMethodNotAllowed, errorEnvelope{Error: apiError{Code: "method_not_allowed", Message: "method not allowed"}})
	})
	return cors(allowedOrigins)(router)
}

func (a *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := auth.BearerToken(r.Header.Get("Authorization"))
		if err != nil {
			writeUnauthorized(w)
			return
		}
		user, err := a.validator.Verify(r.Context(), raw)
		if err != nil {
			writeUnauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
	})
}

func CurrentUser(ctx context.Context) (domain.User, bool) {
	user, ok := ctx.Value(userContextKey{}).(domain.User)
	return user, ok
}

// me godoc
// @Summary Return the authenticated Keycloak user
// @Tags identity
// @Security BearerAuth
// @Success 200 {object} domain.User
// @Router /api/v1/me [get]
func (a *API) me(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, mustUser(r)) }

// listProjects godoc
// @Summary List projects visible to the current user
// @Tags projects
// @Security BearerAuth
// @Success 200 {object} map[string][]domain.Project
// @Router /api/v1/projects [get]
func (a *API) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := a.service.ListProjects(r.Context(), mustUser(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

// createProject godoc
// @Summary Create a project
// @Tags projects
// @Security BearerAuth
// @Param request body CreateProjectRequest true "Project fields"
// @Success 201 {object} domain.Project
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Router /api/v1/projects [post]
func (a *API) createProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !decode(w, r, &req) {
		return
	}
	project, err := a.service.CreateProject(r.Context(), mustUser(r), application.CreateProjectCommand{Name: req.Name, Description: req.Description})
	respond(w, http.StatusCreated, project, err)
}

// listMembers godoc
// @Summary List project members
// @Tags projects
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Success 200 {object} map[string][]MemberSummary
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Router /api/v1/projects/{projectId}/members [get]
func (a *API) listMembers(w http.ResponseWriter, r *http.Request) {
	members, err := a.service.ListMembers(r.Context(), mustUser(r), mux.Vars(r)["projectId"])
	if err != nil {
		writeError(w, err)
		return
	}
	summaries := make([]MemberSummary, 0, len(members))
	for _, m := range members {
		summaries = append(summaries, MemberSummary{UserID: m.UserID, Role: m.Role})
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": summaries})
}

// addMember godoc
// @Summary Add or update a project member
// @Tags projects
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param request body AddMemberRequest true "Member and project role"
// @Success 201 {object} domain.Member
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Router /api/v1/projects/{projectId}/members [post]
func (a *API) addMember(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string             `json:"user_id"`
		Role   domain.ProjectRole `json:"role"`
	}
	if !decode(w, r, &req) {
		return
	}
	member, err := a.service.AddMember(r.Context(), mustUser(r), mux.Vars(r)["projectId"], application.AddMemberCommand{UserID: req.UserID, Role: req.Role})
	respond(w, http.StatusCreated, member, err)
}

// listBoards godoc
// @Summary List project boards
// @Tags boards
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Success 200 {object} map[string][]domain.Board
// @Router /api/v1/projects/{projectId}/boards [get]
func (a *API) listBoards(w http.ResponseWriter, r *http.Request) {
	boards, err := a.service.ListBoards(r.Context(), mustUser(r), mux.Vars(r)["projectId"])
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"boards": boards})
}

// createBoard godoc
// @Summary Create a board and its initial statuses
// @Tags boards
// @Security BearerAuth
// @Param projectId path string true "Project ID"
// @Param request body CreateBoardRequest true "Board fields and optional initial statuses"
// @Success 201 {object} domain.BoardAggregate
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Router /api/v1/projects/{projectId}/boards [post]
func (a *API) createBoard(w http.ResponseWriter, r *http.Request) {
	type statusRequest struct {
		Name     string `json:"name"`
		Position *int   `json:"position"`
	}
	var req struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Statuses    []statusRequest `json:"statuses"`
	}
	if !decode(w, r, &req) {
		return
	}
	statuses := make([]application.StatusInput, 0, len(req.Statuses))
	for i, status := range req.Statuses {
		if status.Position == nil {
			writeError(w, &domain.ValidationError{Field: fmt.Sprintf("statuses[%d].position", i), Message: "is required"})
			return
		}
		statuses = append(statuses, application.StatusInput{Name: status.Name, Position: *status.Position})
	}
	board, err := a.service.CreateBoard(r.Context(), mustUser(r), mux.Vars(r)["projectId"], application.CreateBoardCommand{Name: req.Name, Description: req.Description, Statuses: statuses})
	respond(w, http.StatusCreated, board, err)
}

// getBoard godoc
// @Summary Return a board with statuses, rules, and tasks
// @Tags boards
// @Security BearerAuth
// @Param boardId path string true "Board ID"
// @Success 200 {object} domain.BoardAggregate
// @Router /api/v1/boards/{boardId} [get]
func (a *API) getBoard(w http.ResponseWriter, r *http.Request) {
	board, err := a.service.GetBoard(r.Context(), mustUser(r), mux.Vars(r)["boardId"])
	respond(w, http.StatusOK, board, err)
}

// createStatus godoc
// @Summary Create a board status
// @Tags workflow
// @Security BearerAuth
// @Param boardId path string true "Board ID"
// @Param request body StatusRequest true "Status name and position"
// @Success 201 {object} domain.Status
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Router /api/v1/boards/{boardId}/statuses [post]
func (a *API) createStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Position *int   `json:"position"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Position == nil {
		writeError(w, &domain.ValidationError{Field: "position", Message: "is required"})
		return
	}
	status, err := a.service.CreateStatus(r.Context(), mustUser(r), mux.Vars(r)["boardId"], application.CreateStatusCommand{Name: req.Name, Position: *req.Position})
	respond(w, http.StatusCreated, status, err)
}

// updateStatus godoc
// @Summary Update a status
// @Tags workflow
// @Security BearerAuth
// @Param statusId path string true "Status ID"
// @Param request body UpdateStatusRequest true "Fields to change"
// @Success 200 {object} domain.Status
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Router /api/v1/statuses/{statusId} [patch]
func (a *API) updateStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     *string `json:"name"`
		Position *int    `json:"position"`
	}
	if !decode(w, r, &req) {
		return
	}
	status, err := a.service.UpdateStatus(r.Context(), mustUser(r), mux.Vars(r)["statusId"], application.UpdateStatusCommand{Name: req.Name, Position: req.Position})
	respond(w, http.StatusOK, status, err)
}

// deleteStatus godoc
// @Summary Delete an unused status
// @Tags workflow
// @Security BearerAuth
// @Param statusId path string true "Status ID"
// @Success 204
// @Router /api/v1/statuses/{statusId} [delete]
func (a *API) deleteStatus(w http.ResponseWriter, r *http.Request) {
	err := a.service.DeleteStatus(r.Context(), mustUser(r), mux.Vars(r)["statusId"])
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// createRule godoc
// @Summary Create a workflow transition rule
// @Tags workflow
// @Security BearerAuth
// @Param boardId path string true "Board ID"
// @Param request body CreateRuleRequest true "Directed workflow edge and conditions"
// @Success 201 {object} domain.TransitionRule
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Router /api/v1/boards/{boardId}/rules [post]
func (a *API) createRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FromStatusID string                `json:"from_status_id"`
		ToStatusID   string                `json:"to_status_id"`
		Conditions   domain.RuleConditions `json:"conditions"`
	}
	if !decode(w, r, &req) {
		return
	}
	rule, err := a.service.CreateRule(r.Context(), mustUser(r), mux.Vars(r)["boardId"], application.CreateRuleCommand{FromStatusID: req.FromStatusID, ToStatusID: req.ToStatusID, Conditions: req.Conditions})
	respond(w, http.StatusCreated, rule, err)
}

// updateRule godoc
// @Summary Update a workflow transition rule
// @Tags workflow
// @Security BearerAuth
// @Param ruleId path string true "Rule ID"
// @Param request body UpdateRuleRequest true "Rule fields to change"
// @Success 200 {object} domain.TransitionRule
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Router /api/v1/rules/{ruleId} [patch]
func (a *API) updateRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FromStatusID *string                `json:"from_status_id"`
		ToStatusID   *string                `json:"to_status_id"`
		Conditions   *domain.RuleConditions `json:"conditions"`
	}
	if !decode(w, r, &req) {
		return
	}
	rule, err := a.service.UpdateRule(r.Context(), mustUser(r), mux.Vars(r)["ruleId"], application.UpdateRuleCommand{FromStatusID: req.FromStatusID, ToStatusID: req.ToStatusID, Conditions: req.Conditions})
	respond(w, http.StatusOK, rule, err)
}

// deleteRule godoc
// @Summary Delete a workflow transition rule
// @Tags workflow
// @Security BearerAuth
// @Param ruleId path string true "Rule ID"
// @Success 204
// @Router /api/v1/rules/{ruleId} [delete]
func (a *API) deleteRule(w http.ResponseWriter, r *http.Request) {
	err := a.service.DeleteRule(r.Context(), mustUser(r), mux.Vars(r)["ruleId"])
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listTasks godoc
// @Summary List board tasks
// @Tags tasks
// @Security BearerAuth
// @Param boardId path string true "Board ID"
// @Success 200 {object} map[string][]domain.Task
// @Router /api/v1/boards/{boardId}/tasks [get]
func (a *API) listTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := a.service.ListTasks(r.Context(), mustUser(r), mux.Vars(r)["boardId"])
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

// createTask godoc
// @Summary Create a task
// @Tags tasks
// @Security BearerAuth
// @Param boardId path string true "Board ID"
// @Param request body CreateTaskRequest true "Task fields"
// @Success 201 {object} domain.Task
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Router /api/v1/boards/{boardId}/tasks [post]
func (a *API) createTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string     `json:"title"`
		Description string     `json:"description"`
		StatusID    string     `json:"status_id"`
		AssigneeID  *string    `json:"assignee_id"`
		Deadline    *time.Time `json:"deadline"`
	}
	if !decode(w, r, &req) {
		return
	}
	task, err := a.service.CreateTask(r.Context(), mustUser(r), mux.Vars(r)["boardId"], application.CreateTaskCommand{Title: req.Title, Description: req.Description, StatusID: req.StatusID, AssigneeID: req.AssigneeID, Deadline: req.Deadline})
	respond(w, http.StatusCreated, task, err)
}

// updateTask godoc
// @Summary Update task fields except workflow status
// @Tags tasks
// @Security BearerAuth
// @Param taskId path string true "Task ID"
// @Param request body UpdateTaskRequest true "Task fields to change"
// @Success 200 {object} domain.Task
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Router /api/v1/tasks/{taskId} [patch]
func (a *API) updateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       *string        `json:"title"`
		Description *string        `json:"description"`
		AssigneeID  optionalString `json:"assignee_id"`
		Deadline    optionalTime   `json:"deadline"`
	}
	if !decode(w, r, &req) {
		return
	}
	task, err := a.service.UpdateTask(r.Context(), mustUser(r), mux.Vars(r)["taskId"], application.UpdateTaskCommand{Title: req.Title, Description: req.Description, AssigneeID: req.AssigneeID.Value, SetAssignee: req.AssigneeID.Set, Deadline: req.Deadline.Value, SetDeadline: req.Deadline.Set})
	respond(w, http.StatusOK, task, err)
}

// createComment godoc
// @Summary Add a task comment
// @Tags tasks
// @Security BearerAuth
// @Param taskId path string true "Task ID"
// @Param request body CreateCommentRequest true "Comment body"
// @Success 201 {object} domain.Comment
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Router /api/v1/tasks/{taskId}/comments [post]
func (a *API) createComment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &req) {
		return
	}
	comment, err := a.service.CreateComment(r.Context(), mustUser(r), mux.Vars(r)["taskId"], req.Body)
	respond(w, http.StatusCreated, comment, err)
}

// transitionTask godoc
// @Summary Transition a task after evaluating its persisted workflow rule
// @Tags tasks
// @Security BearerAuth
// @Param taskId path string true "Task ID"
// @Param request body TransitionTaskRequest true "Target workflow status"
// @Success 200 {object} domain.Task
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 422 {object} errorEnvelope
// @Router /api/v1/tasks/{taskId}/transition [post]
func (a *API) transitionTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetStatusID string `json:"target_status_id"`
	}
	if !decode(w, r, &req) {
		return
	}
	task, err := a.service.TransitionTask(r.Context(), mustUser(r), mux.Vars(r)["taskId"], req.TargetStatusID)
	respond(w, http.StatusOK, task, err)
}

// health godoc
// @Summary Process liveness
// @Tags operations
// @Success 200 {object} map[string]string
// @Router /healthz [get]
func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ready godoc
// @Summary PostgreSQL-backed readiness
// @Tags operations
// @Success 200 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /readyz [get]
func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.service.Ready(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *API) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				a.logger.Error("panic in HTTP handler", "error", recovered, "method", r.Method, "path", r.URL.Path)
				writeJSON(w, http.StatusInternalServerError, errorEnvelope{Error: apiError{Code: "internal_error", Message: "internal server error"}})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *API) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", requestID)
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		started := time.Now()
		next.ServeHTTP(recorder, r)
		a.logger.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration", time.Since(started),
			"request_id", requestID,
		)
	})
}

// statusRecorder wraps http.ResponseWriter to capture the status code written
// by downstream handlers so the access-log middleware can log it.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rec *statusRecorder) WriteHeader(status int) {
	if !rec.wroteHeader {
		rec.status = status
		rec.wroteHeader = true
	}
	rec.ResponseWriter.WriteHeader(status)
}

func (rec *statusRecorder) Write(b []byte) (int, error) {
	if !rec.wroteHeader {
		rec.status = http.StatusOK
		rec.wroteHeader = true
	}
	return rec.ResponseWriter.Write(b)
}

func cors(allowed []string) mux.MiddlewareFunc {
	set := make(map[string]struct{}, len(allowed))
	for _, origin := range allowed {
		set[origin] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if _, ok := set[origin]; ok || len(set) == 0 {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func mustUser(r *http.Request) domain.User {
	user, ok := CurrentUser(r.Context())
	if !ok {
		panic("authenticated route without user context")
	}
	return user
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, errorEnvelope{Error: apiError{Code: "invalid_json", Message: friendlyJSONError(err)}})
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, errorEnvelope{Error: apiError{Code: "invalid_json", Message: "request body must contain one JSON object"}})
		return false
	}
	return true
}

func friendlyJSONError(err error) string {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return fmt.Sprintf("field %q has an invalid value", typeErr.Field)
	}
	if errors.Is(err, io.EOF) {
		return "request body is required"
	}
	return "invalid request body: " + err.Error()
}

func respond(w http.ResponseWriter, status int, value any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, status, value)
}

type apiError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	message := "internal server error"
	details := map[string]any(nil)
	switch {
	case errors.Is(err, domain.ErrInvalid):
		status, code, message = http.StatusBadRequest, "invalid_input", err.Error()
		var validation *domain.ValidationError
		if errors.As(err, &validation) && validation.Field != "" {
			details = map[string]any{"field": validation.Field}
		}
	case errors.Is(err, domain.ErrForbidden):
		status, code, message = http.StatusForbidden, "forbidden", "you do not have permission to perform this action"
	case errors.Is(err, domain.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", "resource not found"
	case errors.Is(err, domain.ErrConflict):
		status, code, message = http.StatusConflict, "conflict", err.Error()
	case errors.Is(err, domain.ErrTransitionNotAllowed):
		status, code, message = http.StatusUnprocessableEntity, "transition_not_allowed", err.Error()
	}
	writeJSON(w, status, errorEnvelope{Error: apiError{Code: code, Message: message, Details: details}})
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="taskflow"`)
	writeJSON(w, http.StatusUnauthorized, errorEnvelope{Error: apiError{Code: "unauthorized", Message: "valid bearer token required"}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type optionalString struct {
	Set   bool
	Value *string
}

func (o *optionalString) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

type optionalTime struct {
	Set   bool
	Value *time.Time
}

func (o *optionalTime) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var value time.Time
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}
