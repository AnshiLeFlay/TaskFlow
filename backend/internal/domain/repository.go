package domain

import (
	"context"
	"time"
)

type Repository interface {
	Ping(context.Context) error

	CreateProject(context.Context, *Project, Member) error
	ListProjects(context.Context, string) ([]Project, error)
	GetProject(context.Context, string) (Project, error)
	GetMembership(context.Context, string, string) (Member, error)
	UpsertMember(context.Context, Member) error

	CreateBoard(context.Context, *Board, []Status) error
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
