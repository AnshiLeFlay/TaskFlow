package grpcapi

import (
	"encoding/json"
	"errors"

	"github.com/example/taskflow/backend/internal/application"
	"github.com/example/taskflow/backend/internal/domain"
	"github.com/example/taskflow/backend/internal/infrastructure/auth"
	"github.com/example/taskflow/backend/internal/infrastructure/realtime"
	taskflowv1 "github.com/example/taskflow/backend/proto/taskflowv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	taskflowv1.UnimplementedTaskEventsServer
	service   *application.Service
	validator application.TokenValidator
	broker    *realtime.Broker
}

func NewServer(service *application.Service, validator application.TokenValidator, broker *realtime.Broker) *Server {
	return &Server{service: service, validator: validator, broker: broker}
}

func (s *Server) Subscribe(request *taskflowv1.SubscribeRequest, stream taskflowv1.TaskEvents_SubscribeServer) error {
	if request.GetProjectId() == "" {
		return status.Error(codes.InvalidArgument, "project_id is required")
	}
	md, ok := metadata.FromIncomingContext(stream.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "authorization metadata is required")
	}
	values := md.Get("authorization")
	if len(values) != 1 {
		return status.Error(codes.Unauthenticated, "one bearer token is required")
	}
	rawToken, err := auth.BearerToken(values[0])
	if err != nil {
		return status.Error(codes.Unauthenticated, "valid bearer token is required")
	}
	user, err := s.validator.Verify(stream.Context(), rawToken)
	if err != nil {
		return status.Error(codes.Unauthenticated, "invalid bearer token")
	}
	if err := s.service.AuthorizeProject(stream.Context(), user.ID, request.GetProjectId()); err != nil {
		if errors.Is(err, domain.ErrForbidden) || errors.Is(err, domain.ErrNotFound) {
			return status.Error(codes.PermissionDenied, "project membership is required")
		}
		return status.Error(codes.Internal, "could not authorize subscription")
	}
	events, unsubscribe := s.broker.Subscribe([]string{request.GetProjectId()})
	defer unsubscribe()
	for {
		select {
		case <-stream.Context().Done():
			return nil
		case event, ok := <-events:
			if !ok {
				return nil
			}
			if event.ProjectID != request.GetProjectId() {
				continue
			}
			if err := s.service.AuthorizeProject(stream.Context(), user.ID, event.ProjectID); err != nil {
				if errors.Is(err, domain.ErrForbidden) || errors.Is(err, domain.ErrNotFound) {
					return status.Error(codes.PermissionDenied, "project membership is required")
				}
				return status.Error(codes.Internal, "could not reauthorize subscription")
			}
			if err := stream.Send(toProto(event)); err != nil {
				return err
			}
		}
	}
}

func toProto(event domain.TaskEvent) *taskflowv1.TaskEvent {
	payload := "{}"
	if len(event.Payload) > 0 {
		if encoded, err := json.Marshal(event.Payload); err == nil {
			payload = string(encoded)
		}
	}
	return &taskflowv1.TaskEvent{
		Id: event.ID, Type: string(event.Type), ProjectId: event.ProjectID,
		BoardId: event.BoardID, TaskId: event.TaskID, ActorId: event.ActorID,
		FromStatusId: event.FromStatusID, ToStatusId: event.ToStatusID,
		OccurredAt: timestamppb.New(event.OccurredAt), PayloadJson: payload,
	}
}
