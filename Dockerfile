# syntax=docker/dockerfile:1
# Build stage: compile the two commands the deployment needs. The image carries
# both so the one-shot migrate service and the long-running app share a build
# (schema spec §9.3: migrations are a command, never a boot hook).
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server \
 && CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate

# Runtime stage: minimal alpine with a non-root user and baked-in templates.
# No package installs: alpine ships busybox wget, which the compose
# healthcheck relies on.
FROM alpine:3.20
RUN adduser -D -u 10001 app
COPY --from=build --chown=app:app /out/server /app/server
COPY --from=build --chown=app:app /out/migrate /app/migrate
COPY --chown=app:app templates /app/templates
COPY --chown=app:app static /app/static
# The upload root must exist and belong to the runtime user before the host
# directory is mounted over it; main proves it is writable and refuses to start
# otherwise.
RUN mkdir -p /app/uploads && chown app:app /app/uploads
USER app
WORKDIR /app
EXPOSE 8080
ENTRYPOINT ["./server"]
