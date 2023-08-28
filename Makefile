.PHONY: lint build test
lint:
	golangci-lint run ./...

build:
	go build -o ./build/ ./cmd/moichain

install: moipod logiclab mcutils

moipod:
	go install ./cmd/moipod

logiclab:
	go install ./cmd/logiclab

mcutils:
	go install ./cmd/mcutils

test:
	go test ./... -v -race -short -count=1

test-e2e:
	go test ./... -v -race -count=1