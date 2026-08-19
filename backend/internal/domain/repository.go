package domain

import (
	"context"
	"time"
)

type Repository interface {
	Ping(context.Context) error

	CreateProject(context.Context, *Project, Member) error
	ListProjects(context.Context, string) ([]Project, error)
	// ListAllProjects returns every project regardless of membership. Only
	// superadmins reach it; ordinary access always goes through ListProjects.
	ListAllProjects(context.Context) ([]Project, error)
	GetProject(context.Context, string) (Project, error)
	GetMembership(context.Context, string, string) (Member, error)
	UpsertMember(context.Context, Member) error
	ListMembers(context.Context, string) ([]Member, error)

	// CreateBoard persists a board with its statuses and transition rules in
	// one transaction. Rules are part of the same write because a board whose
	// statuses exist without them cannot move a single task.
	CreateBoard(context.Context, *Board, []Status, []TransitionRule) error
	ListBoards(context.Context, string) ([]Board, error)
	GetBoard(context.Context, string) (Board, error)
	GetBoardAggregate(context.Context, string) (BoardAggregate, error)

	CreateStatus(context.Context, *Status) error
	GetStatus(context.Context, string) (Status, error)
	UpdateStatus(context.Context, Status) error
	DeleteStatus(context.Context, string) error

	CreateRule(context.Context, *TransitionRule) error
	GetRule(context.Context, string) (TransitionRule, error)
	FindRule(context.Context, string, string, string) (TransitionRule, error)
	UpdateRule(context.Context, TransitionRule) error
	DeleteRule(context.Context, string) error

	ListTasks(context.Context, string) ([]Task, error)
	CreateTask(context.Context, *Task) error
	GetTask(context.Context, string) (Task, error)
	UpdateTask(context.Context, Task) error
	UpdateTaskStatus(context.Context, string, string, time.Time, string) (Task, error)
	CountComments(context.Context, string) (int, error)
	CreateComment(context.Context, *Comment) error
}
