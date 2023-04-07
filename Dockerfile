FROM golang:1.18-alpine as build
RUN --mount=type=cache,target=/tmp/apkcache \
  apk add --cache-dir=/tmp/apkcache build-base git
RUN go env -w GOPRIVATE=github.com/sarvalabs/go-polo
ARG ACCESS_TOKEN
RUN git config --global url."https://golang:${ACCESS_TOKEN}@github.com".insteadOf "https://github.com"
ENV GO111MODULE=auto
ENV CGO_ENABLED=1
WORKDIR /src
RUN --mount=type=bind,source=.,rw \
  --mount=type=cache,target=/root/.cache \
  --mount=type=cache,target=/go/pkg/mod \
  go build -o /bin/moichain ./cmd/moichain

FROM alpine:latest
VOLUME /data 
WORKDIR /local
COPY --from=build /bin/moichain /local/moichain
LABEL org.opencontainers.image.source https://github.com/sarvalabs/moichain
ENTRYPOINT ["/local/moichain"]