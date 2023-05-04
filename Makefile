.PHONY: lint build
lint:
	golangci-lint run ./...

build:
	go build -o ./build/ ./cmd/moichain

install:
	go install ./cmd/moichain

test:
	go test ./... -v -race -short -count=1

test-e2e:
	go test ./... -v -race -count=1