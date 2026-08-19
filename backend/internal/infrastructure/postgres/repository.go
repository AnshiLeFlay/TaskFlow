package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/example/taskflow/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Repository, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	return &Repository{pool: pool}, nil
}

func NewWithPool(pool *pgxpool.Pool) *Repository     { return &Repository{pool: pool} }
func (r *Repository) Close()                         { r.pool.Close() }
func (r *Repository) Ping(ctx context.Context) error { return r.pool.Ping(ctx) }

func (r *Repository) CreateProject(ctx context.Context, project *domain.Project, owner domain.Member) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return mapError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO projects (id,name,description,owner_id,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6)`, project.ID, project.Name, project.Description, project.OwnerID, project.CreatedAt, project.UpdatedAt)
	if err != nil {
		return mapError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO project_members (project_id,user_id,role,created_at) VALUES ($1,$2,$3,$4)`, owner.ProjectID, owner.UserID, owner.Role, owner.CreatedAt)
	if err != nil {
		return mapError(err)
	}
	return mapError(tx.Commit(ctx))
}

func (r *Repository) ListProjects(ctx context.Context, userID string) ([]domain.Project, error) {
	rows, err := r.pool.Query(ctx, `SELECT p.id,p.name,p.description,p.owner_id,m.role,p.created_at,p.updated_at FROM projects p JOIN project_members m ON m.project_id=p.id WHERE m.user_id=$1 ORDER BY p.created_at DESC`, userID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	projects := make([]domain.Project, 0)
	for rows.Next() {
		var p domain.Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.Role, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, mapError(err)
		}
		projects = append(projects, p)
	}
	return projects, mapError(rows.Err())
}

// ListAllProjects backs superadmin access. There is no project_members join,
// so the role column cannot come from the database; superadmins act as project
// admins by definition, which is what domain.RoleAdmin records here.
func (r *Repository) ListAllProjects(ctx context.Context) ([]domain.Project, error) {
	rows, err := r.pool.Query(ctx, `SELECT p.id,p.name,p.description,p.owner_id,p.created_at,p.updated_at FROM projects p ORDER BY p.created_at DESC`)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	projects := make([]domain.Project, 0)
	for rows.Next() {
		var p domain.Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, mapError(err)
		}
		p.Role = domain.RoleAdmin
		projects = append(projects, p)
	}
	return projects, mapError(rows.Err())
}

func (r *Repository) GetProject(ctx context.Context, id string) (domain.Project, error) {
	var p domain.Project
	err := r.pool.QueryRow(ctx, `SELECT id,name,description,owner_id,created_at,updated_at FROM projects WHERE id=$1`, id).Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedAt, &p.UpdatedAt)
	return p, mapError(err)
}

func (r *Repository) GetMembership(ctx context.Context, projectID, userID string) (domain.Member, error) {
	var m domain.Member
	err := r.pool.QueryRow(ctx, `SELECT project_id,user_id,role,created_at FROM project_members WHERE project_id=$1 AND user_id=$2`, projectID, userID).Scan(&m.ProjectID, &m.UserID, &m.Role, &m.CreatedAt)
	return m, mapError(err)
}

func (r *Repository) UpsertMember(ctx context.Context, member domain.Member) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO project_members (project_id,user_id,role,created_at) VALUES ($1,$2,$3,$4) ON CONFLICT (project_id,user_id) DO UPDATE SET role=EXCLUDED.role`, member.ProjectID, member.UserID, member.Role, member.CreatedAt)
	return mapError(err)
}

func (r *Repository) ListMembers(ctx context.Context, projectID string) ([]domain.Member, error) {
	rows, err := r.pool.Query(ctx, `SELECT project_id,user_id,role,created_at FROM project_members WHERE project_id=$1 ORDER BY role,user_id`, projectID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	members := make([]domain.Member, 0)
	for rows.Next() {
		var m domain.Member
		if err := rows.Scan(&m.ProjectID, &m.UserID, &m.Role, &m.CreatedAt); err != nil {
			return nil, mapError(err)
		}
		members = append(members, m)
	}
	return members, mapError(rows.Err())
}

func (r *Repository) CreateBoard(ctx context.Context, board *domain.Board, statuses []domain.Status, rules []domain.TransitionRule) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return mapError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO boards (id,project_id,name,description,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6)`, board.ID, board.ProjectID, board.Name, board.Description, board.CreatedAt, board.UpdatedAt)
	if err != nil {
		return mapError(err)
	}
	for _, status := range statuses {
		_, err = tx.Exec(ctx, `INSERT INTO statuses (id,board_id,name,position,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6)`, status.ID, status.BoardID, status.Name, status.Position, status.CreatedAt, status.UpdatedAt)
		if err != nil {
			return mapError(err)
		}
	}
	for _, rule := range rules {
		conditions, marshalErr := json.Marshal(rule.Conditions)
		if marshalErr != nil {
			return fmt.Errorf("encode rule conditions: %w", marshalErr)
		}
		_, err = tx.Exec(ctx, `INSERT INTO transition_rules (id,board_id,from_status_id,to_status_id,conditions,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, rule.ID, rule.BoardID, rule.FromStatusID, rule.ToStatusID, conditions, rule.CreatedAt, rule.UpdatedAt)
		if err != nil {
			return mapError(err)
		}
	}
	return mapError(tx.Commit(ctx))
}

func (r *Repository) ListBoards(ctx context.Context, projectID string) ([]domain.Board, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,project_id,name,description,created_at,updated_at FROM boards WHERE project_id=$1 ORDER BY created_at`, projectID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	boards := make([]domain.Board, 0)
	for rows.Next() {
		var b domain.Board
		if err := rows.Scan(&b.ID, &b.ProjectID, &b.Name, &b.Description, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, mapError(err)
		}
		boards = append(boards, b)
	}
	return boards, mapError(rows.Err())
}

func (r *Repository) GetBoard(ctx context.Context, id string) (domain.Board, error) {
	var b domain.Board
	err := r.pool.QueryRow(ctx, `SELECT id,project_id,name,description,created_at,updated_at FROM boards WHERE id=$1`, id).Scan(&b.ID, &b.ProjectID, &b.Name, &b.Description, &b.CreatedAt, &b.UpdatedAt)
	return b, mapError(err)
}

func (r *Repository) GetBoardAggregate(ctx context.Context, id string) (domain.BoardAggregate, error) {
	b, err := r.GetBoard(ctx, id)
	if err != nil {
		return domain.BoardAggregate{}, err
	}
	statuses, err := r.listStatuses(ctx, id)
	if err != nil {
		return domain.BoardAggregate{}, err
	}
	tasks, err := r.ListTasks(ctx, id)
	if err != nil {
		return domain.BoardAggregate{}, err
	}
	rules, err := r.listRules(ctx, id)
	if err != nil {
		return domain.BoardAggregate{}, err
	}
	return domain.BoardAggregate{Board: b, Statuses: statuses, Tasks: tasks, Rules: rules}, nil
}

func (r *Repository) listStatuses(ctx context.Context, boardID string) ([]domain.Status, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,board_id,name,position,created_at,updated_at FROM statuses WHERE board_id=$1 ORDER BY position,id`, boardID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	statuses := make([]domain.Status, 0)
	for rows.Next() {
		var status domain.Status
		if err := rows.Scan(&status.ID, &status.BoardID, &status.Name, &status.Position, &status.CreatedAt, &status.UpdatedAt); err != nil {
			return nil, mapError(err)
		}
		statuses = append(statuses, status)
	}
	return statuses, mapError(rows.Err())
}

func (r *Repository) CreateStatus(ctx context.Context, status *domain.Status) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO statuses (id,board_id,name,position,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6)`, status.ID, status.BoardID, status.Name, status.Position, status.CreatedAt, status.UpdatedAt)
	return mapError(err)
}

func (r *Repository) GetStatus(ctx context.Context, id string) (domain.Status, error) {
	var status domain.Status
	err := r.pool.QueryRow(ctx, `SELECT id,board_id,name,position,created_at,updated_at FROM statuses WHERE id=$1`, id).Scan(&status.ID, &status.BoardID, &status.Name, &status.Position, &status.CreatedAt, &status.UpdatedAt)
	return status, mapError(err)
}

func (r *Repository) UpdateStatus(ctx context.Context, status domain.Status) error {
	tag, err := r.pool.Exec(ctx, `UPDATE statuses SET name=$2,position=$3,updated_at=$4 WHERE id=$1`, status.ID, status.Name, status.Position, status.UpdatedAt)
	return affectedError(tag.RowsAffected(), err)
}

func (r *Repository) DeleteStatus(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM statuses WHERE id=$1`, id)
	return affectedError(tag.RowsAffected(), err)
}

func (r *Repository) listRules(ctx context.Context, boardID string) ([]domain.TransitionRule, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,board_id,from_status_id,to_status_id,conditions,created_at,updated_at FROM transition_rules WHERE board_id=$1 ORDER BY created_at,id`, boardID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	rules := make([]domain.TransitionRule, 0)
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, mapError(rows.Err())
}

func (r *Repository) CreateRule(ctx context.Context, rule *domain.TransitionRule) error {
	conditions, err := json.Marshal(rule.Conditions)
	if err != nil {
		return fmt.Errorf("encode rule conditions: %w", err)
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO transition_rules (id,board_id,from_status_id,to_status_id,conditions,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, rule.ID, rule.BoardID, rule.FromStatusID, rule.ToStatusID, conditions, rule.CreatedAt, rule.UpdatedAt)
	return mapError(err)
}

func (r *Repository) GetRule(ctx context.Context, id string) (domain.TransitionRule, error) {
	return scanRule(r.pool.QueryRow(ctx, `SELECT id,board_id,from_status_id,to_status_id,conditions,created_at,updated_at FROM transition_rules WHERE id=$1`, id))
}

func (r *Repository) FindRule(ctx context.Context, boardID, fromID, toID string) (domain.TransitionRule, error) {
	return scanRule(r.pool.QueryRow(ctx, `SELECT id,board_id,from_status_id,to_status_id,conditions,created_at,updated_at FROM transition_rules WHERE board_id=$1 AND from_status_id=$2 AND to_status_id=$3`, boardID, fromID, toID))
}

func (r *Repository) UpdateRule(ctx context.Context, rule domain.TransitionRule) error {
	conditions, err := json.Marshal(rule.Conditions)
	if err != nil {
		return fmt.Errorf("encode rule conditions: %w", err)
	}
	tag, err := r.pool.Exec(ctx, `UPDATE transition_rules SET from_status_id=$2,to_status_id=$3,conditions=$4,updated_at=$5 WHERE id=$1`, rule.ID, rule.FromStatusID, rule.ToStatusID, conditions, rule.UpdatedAt)
	return affectedError(tag.RowsAffected(), err)
}

func (r *Repository) DeleteRule(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM transition_rules WHERE id=$1`, id)
	return affectedError(tag.RowsAffected(), err)
}

func (r *Repository) ListTasks(ctx context.Context, boardID string) ([]domain.Task, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,board_id,status_id,title,description,author_id,assignee_id,deadline,created_at,updated_at FROM tasks WHERE board_id=$1 ORDER BY created_at,id`, boardID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	tasks := make([]domain.Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err)
	}
	rows.Close()
	comments, err := r.listCommentsByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		tasks[i].Comments = comments[tasks[i].ID]
	}
	return tasks, nil
}

func (r *Repository) CreateTask(ctx context.Context, task *domain.Task) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO tasks (id,board_id,status_id,title,description,author_id,assignee_id,deadline,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, task.ID, task.BoardID, task.StatusID, task.Title, task.Description, task.AuthorID, task.AssigneeID, task.Deadline, task.CreatedAt, task.UpdatedAt)
	return mapError(err)
}

func (r *Repository) GetTask(ctx context.Context, id string) (domain.Task, error) {
	return scanTask(r.pool.QueryRow(ctx, `SELECT id,board_id,status_id,title,description,author_id,assignee_id,deadline,created_at,updated_at FROM tasks WHERE id=$1`, id))
}

func (r *Repository) UpdateTask(ctx context.Context, task domain.Task) error {
	tag, err := r.pool.Exec(ctx, `UPDATE tasks SET title=$2,description=$3,assignee_id=$4,deadline=$5,updated_at=$6 WHERE id=$1`, task.ID, task.Title, task.Description, task.AssigneeID, task.Deadline, task.UpdatedAt)
	return affectedError(tag.RowsAffected(), err)
}

func (r *Repository) UpdateTaskStatus(ctx context.Context, id, expectedStatusID string, expectedUpdatedAt time.Time, targetStatusID string) (domain.Task, error) {
	task, err := scanTask(r.pool.QueryRow(ctx, `UPDATE tasks SET status_id=$4,updated_at=NOW() WHERE id=$1 AND status_id=$2 AND updated_at=$3 RETURNING id,board_id,status_id,title,description,author_id,assignee_id,deadline,created_at,updated_at`, id, expectedStatusID, expectedUpdatedAt, targetStatusID))
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Task{}, fmt.Errorf("%w: task status changed concurrently", domain.ErrConflict)
	}
	return task, err
}

func (r *Repository) CountComments(ctx context.Context, taskID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM comments WHERE task_id=$1`, taskID).Scan(&count)
	return count, mapError(err)
}

func (r *Repository) CreateComment(ctx context.Context, comment *domain.Comment) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO comments (id,task_id,author_id,body,created_at) VALUES ($1,$2,$3,$4,$5)`, comment.ID, comment.TaskID, comment.AuthorID, comment.Body, comment.CreatedAt)
	return mapError(err)
}

func (r *Repository) listCommentsByBoard(ctx context.Context, boardID string) (map[string][]domain.Comment, error) {
	rows, err := r.pool.Query(ctx, `SELECT c.id,c.task_id,c.author_id,c.body,c.created_at FROM comments c JOIN tasks t ON t.id=c.task_id WHERE t.board_id=$1 ORDER BY c.created_at,c.id`, boardID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	comments := make(map[string][]domain.Comment)
	for rows.Next() {
		var comment domain.Comment
		if err := rows.Scan(&comment.ID, &comment.TaskID, &comment.AuthorID, &comment.Body, &comment.CreatedAt); err != nil {
			return nil, mapError(err)
		}
		comments[comment.TaskID] = append(comments[comment.TaskID], comment)
	}
	return comments, mapError(rows.Err())
}

type scanner interface{ Scan(...any) error }

func scanRule(row scanner) (domain.TransitionRule, error) {
	var rule domain.TransitionRule
	var conditions []byte
	if err := row.Scan(&rule.ID, &rule.BoardID, &rule.FromStatusID, &rule.ToStatusID, &conditions, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
		return domain.TransitionRule{}, mapError(err)
	}
	if err := json.Unmarshal(conditions, &rule.Conditions); err != nil {
		return domain.TransitionRule{}, fmt.Errorf("decode rule conditions: %w", err)
	}
	return rule, nil
}

func scanTask(row scanner) (domain.Task, error) {
	var task domain.Task
	err := row.Scan(&task.ID, &task.BoardID, &task.StatusID, &task.Title, &task.Description, &task.AuthorID, &task.AssigneeID, &task.Deadline, &task.CreatedAt, &task.UpdatedAt)
	return task, mapError(err)
}

func affectedError(rows int64, err error) error {
	if err != nil {
		return mapError(err)
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: resource already exists", domain.ErrConflict)
		case "23503":
			return fmt.Errorf("%w: resource is referenced or related resource does not exist", domain.ErrConflict)
		case "23514", "22P02":
			return fmt.Errorf("%w: database constraint rejected input", domain.ErrInvalid)
		}
	}
	return err
}
