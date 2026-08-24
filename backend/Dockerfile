# The Go API service. Build context is the repo root (it needs certs/), see
# docker-compose.yml. The inference sidecar has its own: backend/inference/Dockerfile.

FROM golang:1.26-alpine AS build
WORKDIR /src

# Any .crt dropped in certs/ becomes trusted at build time, same as the Python
# image: a corporate TLS proxy intercepts the module download too.
COPY certs/ /usr/local/share/ca-certificates/
RUN apk add --no-cache ca-certificates && update-ca-certificates

# Module files first so a source-only change reuses the download layer.
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/cmd/ cmd/
COPY backend/internal/ internal/
# CGO off so the result runs on a base image with no libc of its own.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.22
# ca-certificates for outbound TLS; busybox already provides the wget the
# healthcheck uses, which is why this is alpine rather than distroless -- a
# scratch image has nothing that can perform its own healthcheck.
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/api /app/api

# Two data files shared with the Python sidecar rather than compiled in, so
# that adding a checkpoint or changing the schema is one edit and not two that
# can disagree. They are data, not code, hence copied rather than embedded.
COPY backend/models.json /app/models.json
COPY backend/db/schema.sql /app/db/schema.sql

ENV MODELS_DIR=/models \
    MODELS_CATALOG=/app/models.json \
    SCHEMA_PATH=/app/db/schema.sql \
    LABEL_TOOL_MODE=vm \
    LABEL_TOOL_VM_ROOT=/opt/mount/project \
    PORT=8000

# NFR-07 -- not root. Must match the UID that owns DATA_DIR on a Linux host and
# the sidecar's UID, since both write under it:
#   docker compose build --build-arg APP_UID=$(id -u)
ARG APP_UID=1000
RUN adduser -D -u ${APP_UID} app && chown -R app /app
USER app

EXPOSE 8000
HEALTHCHECK --interval=15s --timeout=3s --retries=10 \
    CMD wget -q -O- http://localhost:8000/api/config >/dev/null || exit 1
CMD ["/app/api"]
