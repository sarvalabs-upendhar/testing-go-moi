FROM golang@sha256:ab5685692564e027aa84e2980855775b2e48f8fc82c1590c0e1e8cbc2e716542 AS build # 1.18.10-alpine3.17
RUN apk add --no-cache git gcc g++ ca-certificates tzdata && update-ca-certificates
RUN go env -w GOPRIVATE=github.com/sarvalabs/go-polo
ARG ACCESS_TOKEN
RUN git config --global url."https://golang:${ACCESS_TOKEN}@github.com".insteadOf "https://github.com"
ENV GO111MODULE=auto
ENV CGO_ENABLED=1

WORKDIR /go/src/github/sarvalabs/moichain
COPY go.mod /go/src/github/sarvalabs/moichain
COPY go.sum /go/src/github/sarvalabs/moichain

RUN --mount=type=cache,id=go-build-linux-amd64,target=/root/.cache/go-build --mount=type=cache,id=go-pkg-linux-amd64,target=/go/pkg go mod download -x
COPY . /go/src/github/sarvalabs/moichain
RUN --mount=type=cache,id=go-build-linux-amd64,target=/root/.cache/go-build --mount=type=cache,id=go-pkg-linux-amd64,target=/go/pkg go build -trimpath -o /bin/moichain ./cmd/moichain/

FROM alpine@sha256:c0669ef34cdc14332c0f1ab0c2c01acb91d96014b172f1a76f3a39e63d1f0bda # 3.18.0
VOLUME /data
ARG GIT_COMMIT
ENV GIT_COMMIT="{$GIT_COMMIT}"

LABEL org.opencontainers.image.title="moichain"
LABEL org.opencontainers.image.source="https://github.com/sarvalabs/moichain"
LABEL org.opencontainers.image.revision="${GIT_COMMIT}"

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /bin/moichain /bin/moichain

ENTRYPOINT [ "/bin/moichain" ]

# TODO: Build w/ AppArmour and Seccomp
# TODO: Check SBOM & Supply Chain
