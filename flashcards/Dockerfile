# Multi-stage build: a static Go binary in a distroless image.
#
# The result is ~19 MB with no shell and no package manager. That is the
# production-grade shape, and it has a real trade-off worth knowing before an
# interview asks: with no shell, `kubectl exec -it ... -- sh` will not work.
# Use `kubectl debug` and an ephemeral container instead.

FROM golang:1.26-alpine AS build
WORKDIR /src

# Copy manifests first so dependency download caches independently of source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 makes the binary static so it runs on distroless/static.
# -trimpath and -ldflags="-s -w" drop local paths and debug info.
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags="-s -w" \
      -o /out/flashcards ./cmd/flashcards

# Run the tests inside the build so a broken deck can't ship.
RUN go test ./...

FROM gcr.io/distroless/static-debian12:nonroot

# 65532 is distroless' "nonroot" user. Running as non-root is what lets the
# Pod spec set runAsNonRoot: true in M3.
USER 65532:65532
WORKDIR /app

COPY --from=build /out/flashcards /app/flashcards

ENV PORT=8080 DATA_DIR=/data
EXPOSE 8080

# No HEALTHCHECK on purpose: Kubernetes probes replace it, and a Docker
# HEALTHCHECK would need a shell this image does not have.
ENTRYPOINT ["/app/flashcards"]
