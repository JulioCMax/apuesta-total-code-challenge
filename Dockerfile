# syntax=docker/dockerfile:1

# --- build stage -----------------------------------------------------------
# golang:1.27-alpine matches the local toolchain (go.mod: go 1.27.0).
FROM golang:1.27-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

# CGO_ENABLED=0 produces a fully static binary so it can run on the
# distroless base below (no libc, no shell).
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/seed ./cmd/seed

# --- api runtime image -------------------------------------------------------
# distroless static-debian12:nonroot — alpine-with-shell was rejected
# (design.md D-adjacent note in Local vs Lambda): there is nothing to debug
# inside a static Go binary, and a smaller/safer image is strictly better.
FROM gcr.io/distroless/static-debian12:nonroot AS api

COPY --from=builder /out/api /api

EXPOSE 8080
ENTRYPOINT ["/api"]

# --- seed runtime image ------------------------------------------------------
# Same distroless base; a separate final stage so docker-compose can target
# each binary by image stage without a second Dockerfile.
FROM gcr.io/distroless/static-debian12:nonroot AS seed

COPY --from=builder /out/seed /seed

ENTRYPOINT ["/seed"]
