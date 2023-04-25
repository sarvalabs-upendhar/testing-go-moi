.PHONY: lint build
lint:
	golangci-lint run -E whitespace -E wsl -E wastedassign -E unconvert -E tparallel -E thelper -E stylecheck -E prealloc \
	-E predeclared -E nlreturn -E misspell -E makezero -E lll -E importas -E ifshort -E gosec -E gofumpt \
	-E goconst -E forcetypeassert -E dogsled -E dupl -E errname -E errorlint -E nolintlint

build:
	go build -o ./build/ ./cmd/moichain

install:
	go install ./cmd/moichain