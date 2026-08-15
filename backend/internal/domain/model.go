package domain

import "time"

type ProjectRole string

const (
	RoleAdmin  ProjectRole = "admin"
	RoleMember ProjectRole = "member"
	RoleViewer ProjectRole = "viewer"
)

func (r ProjectRole) Valid() bool {
	switch r {
	case RoleAdmin, RoleMember, RoleViewer:
		return true
	default:
		return false
	}
}

func (r ProjectRole) CanManageWorkflow() bool { return r == RoleAdmin }
func (r ProjectRole) CanManageMembers() bool  { return r == RoleAdmin }
func (r ProjectRole) CanManageTasks() bool {
	return r == RoleAdmin || r == RoleMember
}

type Project struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	OwnerID     string      `json:"owner_id"`
	Role        ProjectRole `json:"role,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type Member struct {
	ProjectID string      `json:"project_id"`
	UserID    string      `json:"user_id"`
	Role      ProjectRole `json:"role"`
	CreatedAt time.Time   `json:"created_at"`
}

type Board struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type BoardAggregate struct {
	Board
	Statuses []Status         `json:"statuses"`
	Tasks    []Task           `json:"tasks"`
	Rules    []TransitionRule `json:"rules"`
}

type Status struct {
	ID        string    `json:"id"`
	BoardID   string    `json:"board_id"`
	Name      string    `json:"name"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type RuleConditions struct {
	AuthorOnly       bool          `json:"author_only"`
	AssigneeOnly     bool          `json:"assignee_only"`
	RequiresComment  bool          `json:"requires_comment"`
	ProjectOwnerOnly bool          `json:"project_owner_only"`
	AllowedRoles     []ProjectRole `json:"allowed_roles,omitempty"`
}

type TransitionRule struct {
	ID           string         `json:"id"`
	BoardID      string         `json:"board_id"`
	FromStatusID string         `json:"from_status_id"`
	ToStatusID   string         `json:"to_status_id"`
	Conditions   RuleConditions `json:"conditions"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type Task struct {
	ID          string     `json:"id"`
	BoardID     string     `json:"board_id"`
	StatusID    string     `json:"status_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	AuthorID    string     `json:"author_id"`
	AssigneeID  *string    `json:"assignee_id"`
	Deadline    *time.Time `json:"deadline"`
	Comments    []Comment  `json:"comments,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Comment struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	AuthorID  string    `json:"author_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type User struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Name     string   `json:"name,omitempty"`
	Roles    []string `json:"roles"`
}
