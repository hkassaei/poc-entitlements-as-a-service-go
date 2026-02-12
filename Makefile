.PHONY: build build-ecs build-mock-hss test lint vet fmt clean docker-build docker-up docker-down

# Go binary output directory
BIN := bin

## Build
build: build-ecs build-mock-hss

build-ecs:
	go build -ldflags="-s -w" -o $(BIN)/ecs ./cmd/ecs

build-mock-hss:
	go build -ldflags="-s -w" -o $(BIN)/mock-hss ./cmd/mock-hss

## Test
test:
	go test ./... -count=1

test-race:
	go test ./... -race -count=1

test-cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

## Code quality
lint:
	golangci-lint run ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Files not formatted:" && gofmt -l . && exit 1)

## Docker (Go implementation)
docker-build:
	docker compose -f docker-compose.go.yml build

docker-up:
	docker compose -f docker-compose.go.yml up -d

docker-down:
	docker compose -f docker-compose.go.yml down

docker-logs:
	docker compose -f docker-compose.go.yml logs -f

## Housekeeping
clean:
	rm -rf $(BIN) coverage.out coverage.html
	go clean -cache -testcache
