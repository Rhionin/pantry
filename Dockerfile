# syntax=docker/dockerfile:1

ARG COMMIT_HASH=unknown

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
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/pantry-server ./cmd/server

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