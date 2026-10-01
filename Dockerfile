# syntax=docker/dockerfile:1

ARG COMMIT_HASH=unknown
ARG COMMIT_TIME=
ARG COMMIT_SUBJECT=

# Frontend build stage
FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Go build stage
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/frontend/dist/ ./internal/webui/assets/
RUN mkdir -p /data
# Redeclared so this stage inherits the global defaults. $$ leaves the values
# for the shell: a commit subject spliced into this script would break on
# quotes, so the subject is base64-encoded before -ldflags -X.
ARG COMMIT_HASH
ARG COMMIT_TIME
ARG COMMIT_SUBJECT
RUN set -eu; \
    subject_b64=$(printf '%s' "$$COMMIT_SUBJECT" | base64 | tr -d '\n'); \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w -X github.com/Rhionin/pantry/internal/buildinfo.Commit=$$COMMIT_HASH -X github.com/Rhionin/pantry/internal/buildinfo.CommittedAt=$$COMMIT_TIME -X github.com/Rhionin/pantry/internal/buildinfo.subjectStamp=$$subject_b64" \
    -o /out/pantry-server ./cmd/server

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot
ARG COMMIT_HASH
LABEL org.opencontainers.image.revision="${COMMIT_HASH}"
LABEL com.rhionin.pantry.commit="${COMMIT_HASH}"
COPY --from=build /out/pantry-server /usr/local/bin/pantry-server
COPY --from=build --chown=nonroot:nonroot /data /data
ENV ADDR=":8080" DB_PATH="/data/pantry.db"
EXPOSE 8080
VOLUME ["/data"]
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/pantry-server"]