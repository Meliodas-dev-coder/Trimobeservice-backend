.PHONY: run build seed tidy test vet fmt

# Run the API (loads .env automatically)
run:
	go run ./cmd/api

# Compile everything to ./bin/api
build:
	go build -o bin/api ./cmd/api

# Create/promote a super-admin (applies migrations first).
# usage: make seed EMAIL=admin@trimo.dev PASSWORD=secret123 NAME="Super Admin"
seed:
	go run ./cmd/seed -email="$(EMAIL)" -password="$(PASSWORD)" -name="$(NAME)"

tidy:
	go mod tidy

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .
