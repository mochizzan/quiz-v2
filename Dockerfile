# syntax=docker/dockerfile:1
# Build stage: compile the two commands the deployment needs. The image carries
# both so the one-shot migrate service and the long-running app share a build
# (schema spec §9.3: migrations are a command, never a boot hook).
FROM golang:1.26-alpine3.22 AS build
WORKDIR /src
# The image codecs are cgo (libwebp for WebP, libheif for HEIC), so the
# build stage needs a C toolchain and both libraries' headers (AVT-104).
RUN apk add --no-cache build-base pkgconf libwebp-dev libheif-dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /out/server ./cmd/server \
 && CGO_ENABLED=1 go build -o /out/migrate ./cmd/migrate

# Runtime stage: minimal alpine with a non-root user and baked-in templates.
# The only package installs are libwebp and libheif, the shared libraries the
# cgo codecs link against (AVT-104); alpine's busybox wget, which the compose
# healthcheck relies on, ships with the base image.
FROM alpine:3.22
RUN apk add --no-cache libwebp libheif
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
