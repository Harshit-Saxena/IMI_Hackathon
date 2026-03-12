.PHONY: run build test migrate lint docker-up docker-down docker-logs clean

# ── Build ─────────────────────────────────────────────────────────────────────
build:
	go build -o bin/server ./cmd/server

run:
	go run ./cmd/server

# ── Database ──────────────────────────────────────────────────────────────────
migrate:
	go run ./cmd/server   # server auto-runs migrations on startup

# ── Tests ─────────────────────────────────────────────────────────────────────
test:
	go test ./... -v -count=1

test-unit:
	go test ./... -v -count=1 -short

test-integration:
	go test ./... -v -count=1 -run Integration

bench:
	go test ./... -bench=. -benchmem -run='^$$'

# ── Code quality ──────────────────────────────────────────────────────────────
lint:
	golangci-lint run ./...

vet:
	go vet ./...

tidy:
	go mod tidy

# ── Docker ────────────────────────────────────────────────────────────────────
docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

docker-build:
	docker build -t search-system:latest .

# ── Cleanup ───────────────────────────────────────────────────────────────────
clean:
	rm -rf bin/
	go clean ./...
