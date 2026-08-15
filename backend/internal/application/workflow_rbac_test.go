package application

import (
	"context"
	"testing"

	"github.com/example/taskflow/backend/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestOnlyProjectAdminsCanMutateStatusesAndRules(t *testing.T) {
	ctx := context.Background()
	actor := domain.User{ID: "member"}
	repo := newFakeRepository()
	repo.projects["project"] = domain.Project{ID: "project", OwnerID: "owner"}
	repo.members[memberKey("project", actor.ID)] = domain.Member{ProjectID: "project", UserID: actor.ID, Role: domain.RoleMember}
	repo.boards["board"] = domain.Board{ID: "board", ProjectID: "project"}
	repo.statuses["todo"] = domain.Status{ID: "todo", BoardID: "board", Name: "To Do"}
	repo.statuses["done"] = domain.Status{ID: "done", BoardID: "board", Name: "Done"}
	repo.rules["rule"] = domain.TransitionRule{ID: "rule", BoardID: "board", FromStatusID: "todo", ToStatusID: "done"}
	service := NewService(repo, nil)
	name := "Renamed"
	conditions := domain.RuleConditions{RequiresComment: true}

	tests := []struct {
		name string
		call func() error
	}{
		{"create status", func() error {
			_, err := service.CreateStatus(ctx, actor, "board", CreateStatusCommand{Name: "Review", Position: 2})
			return err
		}},
		{"update status", func() error {
			_, err := service.UpdateStatus(ctx, actor, "todo", UpdateStatusCommand{Name: &name})
			return err
		}},
		{"delete status", func() error { return service.DeleteStatus(ctx, actor, "todo") }},
		{"create rule", func() error {
			_, err := service.CreateRule(ctx, actor, "board", CreateRuleCommand{FromStatusID: "todo", ToStatusID: "done"})
			return err
		}},
		{"update rule", func() error {
			_, err := service.UpdateRule(ctx, actor, "rule", UpdateRuleCommand{Conditions: &conditions})
			return err
		}},
		{"delete rule", func() error { return service.DeleteRule(ctx, actor, "rule") }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.ErrorIs(t, test.call(), domain.ErrForbidden)
		})
	}
}
