package grpcapi

import (
	"context"
	"testing"
	"time"

	"github.com/example/taskflow/backend/internal/domain"
	taskflowv1 "github.com/example/taskflow/backend/proto/taskflowv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testSubscribeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s testSubscribeStream) Context() context.Context { return s.ctx }

func (testSubscribeStream) Send(*taskflowv1.TaskEvent) error { return nil }

func TestSubscribeRejectsMissingProjectID(t *testing.T) {
	server := NewServer(nil, nil, nil)
	err := server.Subscribe(&taskflowv1.SubscribeRequest{}, testSubscribeStream{ctx: context.Background()})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestSubscribeRequiresAuthorizationMetadata(t *testing.T) {
	server := NewServer(nil, nil, nil)
	err := server.Subscribe(&taskflowv1.SubscribeRequest{ProjectId: "project"}, testSubscribeStream{ctx: context.Background()})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestToProtoMapsDomainEvent(t *testing.T) {
	occurredAt := time.Date(2026, 8, 15, 10, 11, 12, 13, time.UTC)
	event := domain.TaskEvent{
		ID: "event", Type: domain.EventTaskTransitioned, ProjectID: "project",
		BoardID: "board", TaskID: "task", ActorID: "actor",
		FromStatusID: "todo", ToStatusID: "done", OccurredAt: occurredAt,
		Payload: map[string]any{"revision": float64(2)},
	}

	got := toProto(event)

	assert.Equal(t, event.ID, got.GetId())
	assert.Equal(t, string(event.Type), got.GetType())
	assert.Equal(t, event.ProjectID, got.GetProjectId())
	assert.Equal(t, event.BoardID, got.GetBoardId())
	assert.Equal(t, event.TaskID, got.GetTaskId())
	assert.Equal(t, event.ActorID, got.GetActorId())
	assert.Equal(t, event.FromStatusID, got.GetFromStatusId())
	assert.Equal(t, event.ToStatusID, got.GetToStatusId())
	assert.True(t, got.GetOccurredAt().AsTime().Equal(occurredAt))
	assert.JSONEq(t, `{"revision":2}`, got.GetPayloadJson())
}
