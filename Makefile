.PHONY: lint build test
lint:
	golangci-lint run ./...

build:
	go build -o ./build/ ./cmd/moipod

install: moipod mcutils logiclab

moipod:
	go install ./cmd/moipod

mcutils:
	go install ./cmd/mcutils

logiclab:
	go install ./cmd/logiclab

test:
	make install && go test ./... -v -race --shuffle=on -short -count=1

test-e2e:
	go test ./... -v -race -count=1 --shuffle=on