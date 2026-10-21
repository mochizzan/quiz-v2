# syntax=docker/dockerfile:1
# Build stage: compile a static server binary from the quiz-v2 module.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

# Runtime stage: minimal alpine with a non-root user and baked-in templates.
# No package installs: alpine ships busybox wget, which the compose
# healthcheck relies on.
FROM alpine:3.20
RUN adduser -D -u 10001 app
COPY --from=build --chown=app:app /out/server /app/server
COPY --chown=app:app templates /app/templates
COPY --chown=app:app static /app/static
USER app
WORKDIR /app
EXPOSE 8080
ENTRYPOINT ["./server"]
