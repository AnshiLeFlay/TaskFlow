package domain

import (
	"errors"
	"fmt"
	"time"
)

type TransitionContext struct {
	ActorID      string
	ProjectOwner string
	ProjectRole  ProjectRole
	CommentCount int
}

func ValidateTransition(task Task, rule *TransitionRule, targetStatusID string, ctx TransitionContext) error {
	if targetStatusID == "" {
		return &ValidationError{Field: "target_status_id", Message: "is required"}
	}
	if task.StatusID == targetStatusID {
		return fmt.Errorf("%w: task is already in target status", ErrTransitionNotAllowed)
	}
	if rule == nil || rule.FromStatusID != task.StatusID || rule.ToStatusID != targetStatusID {
		return fmt.Errorf("%w: no workflow rule for this status pair", ErrTransitionNotAllowed)
	}
	c := rule.Conditions
	if c.AuthorOnly && task.AuthorID != ctx.ActorID {
		return fmt.Errorf("%w: only the task author may perform this transition", ErrTransitionNotAllowed)
	}
	if c.AssigneeOnly && (task.AssigneeID == nil || *task.AssigneeID != ctx.ActorID) {
		return fmt.Errorf("%w: only the task assignee may perform this transition", ErrTransitionNotAllowed)
	}
	if c.RequiresComment && ctx.CommentCount == 0 {
		return fmt.Errorf("%w: at least one comment is required", ErrTransitionNotAllowed)
	}
	if c.ProjectOwnerOnly && ctx.ProjectOwner != ctx.ActorID {
		return fmt.Errorf("%w: only the project owner may perform this transition", ErrTransitionNotAllowed)
	}
	if len(c.AllowedRoles) > 0 {
		allowed := false
		for _, role := range c.AllowedRoles {
			if role == ctx.ProjectRole {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("%w: project role %q is not allowed", ErrTransitionNotAllowed, ctx.ProjectRole)
		}
	}
	return nil
}

type EventType string

const (
	EventTaskTransitioned EventType = "task.transitioned"
	EventTaskUpdated      EventType = "task.updated"
	EventCommentCreated   EventType = "comment.created"
)

type TaskEvent struct {
	ID           string         `json:"id"`
	Type         EventType      `json:"type"`
	ProjectID    string         `json:"project_id"`
	BoardID      string         `json:"board_id"`
	TaskID       string         `json:"task_id"`
	ActorID      string         `json:"actor_id"`
	FromStatusID string         `json:"from_status_id,omitempty"`
	ToStatusID   string         `json:"to_status_id,omitempty"`
	OccurredAt   time.Time      `json:"occurred_at"`
	Payload      map[string]any `json:"payload,omitempty"`
}

type EventPublisher interface {
	Publish(TaskEvent)
}

type NopPublisher struct{}

func (NopPublisher) Publish(TaskEvent) {}

func IsBusinessError(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, ErrForbidden) || errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalid) || errors.Is(err, ErrTransitionNotAllowed)
}
