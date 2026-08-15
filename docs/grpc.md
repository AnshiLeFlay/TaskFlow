# gRPC task-event stream

The backend listens on port `50051` and implements the checked-in proto contract
in `backend/proto/taskflowv1/task_events.proto`:

```proto
service TaskEvents {
  rpc Subscribe(SubscribeRequest) returns (stream TaskEvent);
}
```

`SubscribeRequest.project_id` is required and limits the live stream to one
project. The caller must send `authorization: Bearer <Keycloak access token>`
metadata and be a member of that project. Every message carries its event
ID/type, project/board/task/actor IDs, old and new
status IDs when relevant, a UTC protobuf timestamp and the event-specific JSON
payload. The same logical event ID and fields are sent to browser WebSocket
subscribers.

Example with `grpcurl`:

```sh
grpcurl -plaintext \
  -H 'authorization: Bearer <token>' \
  -d '{"project_id":"f09cbaf2-a53b-4890-8688-a888f01842ce"}' \
  localhost:50051 taskflow.v1.TaskEvents/Subscribe
```

This MVP stream is process-local, at-most-once, and intended to demonstrate an
external consumption interface. It does not replay historical messages. A
production deployment should put the event behind an authenticated/mTLS gateway
and a durable outbox/broker. Consumers should deduplicate by event ID.
