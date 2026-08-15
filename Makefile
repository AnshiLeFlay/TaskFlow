.PHONY: help up down logs build test test-backend test-integration test-frontend e2e e2e-headed swagger swagger-check proto clean

help:
	@echo "TaskFlow targets: up, down, logs, build, test, test-backend, test-integration, test-frontend, e2e, e2e-headed, swagger, swagger-check, proto"

up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f backend frontend keycloak

build:
	docker compose build

test: test-backend test-frontend

test-backend:
	cd backend && go test -race -coverprofile=coverage.out ./...

test-integration:
	docker compose up -d postgres
	cd backend && TEST_DATABASE_URL='postgres://taskflow:taskflow@localhost:5432/taskflow?sslmode=disable' go test -tags=integration -count=1 ./tests/integration/...

test-frontend:
	cd frontend && npm ci && npm run typecheck && npm run test --if-present && npm run build

e2e:
	cd e2e && npm ci && npx playwright install --with-deps chromium && npm test

e2e-headed:
	cd e2e && npm ci && npx playwright install chromium && npm run test:headed

# Generate the Swagger document served by Swagger UI from Go annotations. The
# reviewed OpenAPI 3 contract remains available alongside it.
swagger:
	cd backend && go run github.com/swaggo/swag/cmd/swag@v1.16.4 init -g cmd/server/main.go -o swagger/generated --outputTypes json,yaml
	mkdir -p swagger/generated
	cp backend/swagger/generated/swagger.yaml swagger/generated/swagger.yaml
	cp backend/swagger/generated/swagger.json swagger/generated/swagger.json

swagger-check:
	docker run --rm -v "$(CURDIR)/swagger:/api" swaggerapi/swagger-cli:v4 validate /api/openapi.yaml
	docker run --rm -v "$(CURDIR)/swagger:/api" swaggerapi/swagger-cli:v4 validate /api/generated/swagger.json

proto:
	docker run --rm -v "$(CURDIR)/backend:/workspace" -w /workspace namely/protoc-all:1.51 -f proto/taskflowv1/task_events.proto -l go

clean:
	docker compose down --remove-orphans
