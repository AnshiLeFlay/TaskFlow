.PHONY: help env up down logs build fmt vet test test-backend test-backend-docker test-integration test-frontend test-all e2e e2e-headed e2e-full swagger swagger-check proto clean

help:
	@echo "TaskFlow targets: env, up, down, logs, build, fmt, vet, test, test-backend, test-backend-docker, test-integration, test-frontend, test-all, e2e, e2e-headed, e2e-full, swagger, swagger-check, proto"
	@echo "  test        - backend (local go) + frontend tests; requires a local Go toolchain"
	@echo "  test-all    - backend (dockerized) + frontend + integration tests; no local Go required"

env:
	test -f .env || cp .env.example .env

up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f backend frontend keycloak

build:
	docker compose build

# fmt/vet run inside the Dockerfile's "base" stage so there is no dependency
# on a local Go toolchain.
fmt:
	docker build --target base -t taskflow-backend-base ./backend
	docker run --rm taskflow-backend-base sh -c 'out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "$$out"; echo "gofmt check failed"; exit 1; fi'

vet:
	docker build --target base -t taskflow-backend-base ./backend
	docker run --rm taskflow-backend-base go vet ./...

test: test-backend test-frontend

test-backend:
	cd backend && go test -race -coverprofile=coverage.out ./...

# Builds the Dockerfile's "test" stage, which runs `go test ./...` as part of
# the image build (no local Go toolchain required).
test-backend-docker:
	docker build --target test ./backend

test-integration:
	docker compose up -d postgres
	cd backend && TEST_DATABASE_URL='postgres://taskflow:taskflow@localhost:5432/taskflow?sslmode=disable' go test -tags=integration -count=1 ./tests/integration/...

test-frontend:
	cd frontend && npm ci && npm run typecheck && npm run test --if-present && npm run build

# Uses the dockerized backend test target (not `test`, which needs a local Go
# toolchain that this project assumes absent) so `make test-all` works out of
# the box in a Go-less environment.
test-all: test-backend-docker test-frontend test-integration

e2e:
	cd e2e && npm ci && npx playwright install --with-deps chromium && npm test

e2e-headed:
	cd e2e && npm ci && npx playwright install chromium && npm run test:headed

# Brings up the full stack (built fresh, waiting for healthchecks) and then
# runs the Playwright suite headless against it, in one command.
e2e-full:
	docker compose up -d --build --wait
	cd e2e && npm ci && npx playwright install --with-deps chromium && npm test

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
