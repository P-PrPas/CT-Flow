# The API service. Build context is label_tool/ (this repo's root), see
# docker-compose.yml.
#
# No torch here any more: inference and the prompt bank live in the sidecar
# (Dockerfile.vpe). That is what makes this image small enough to rebuild
# casually, and it is also the split the Go port needs -- see
# docs/REFACTOR_PLAN.md.
FROM python:3.12-slim

# libglib2.0-0 for opencv-headless (libgl1 is only needed by the GUI build,
# which this image no longer installs); curl for the healthcheck.
RUN apt-get update \
    && apt-get install -y --no-install-recommends libglib2.0-0 curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Any .crt dropped in certs/ becomes trusted at build time. Needed when
# something intercepts TLS between the build and pypi -- a corporate proxy,
# or antivirus HTTPS scanning (AVG/Avast/Kaspersky all do this).
# Empty folder = no-op.
COPY certs/ /usr/local/share/ca-certificates/
RUN update-ca-certificates
ENV PIP_CERT=/etc/ssl/certs/ca-certificates.crt \
    REQUESTS_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt \
    SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt

COPY backend/requirements.txt backend/requirements.txt
RUN pip install --no-cache-dir -r backend/requirements.txt

# backend/ stays a real package (app.py, routers/, services/) so imports
# inside it (`from .routers import ...`, `from ..services.images import ...`)
# resolve the same way they do outside Docker.
COPY backend/ backend/

ENV MODELS_DIR=/models \
    LABEL_TOOL_MODE=vm \
    LABEL_TOOL_VM_ROOT=/opt/mount/project \
    VPE_URL=http://vpe:8001 \
    PYTHONUNBUFFERED=1

# NFR-07 -- not root. The UID has to match whoever owns the host folder you
# bind-mount at /opt/mount/project, or the container cannot write uploads into
# it -- and it has to match the vpe container's, since both write under there:
#   docker compose build --build-arg APP_UID=$(id -u)
# Docker Desktop on Windows/macOS ignores mount ownership, so the default is
# only a real decision on Linux hosts.
ARG APP_UID=1000
RUN useradd --create-home --uid ${APP_UID} app && chown -R app /app
USER app

EXPOSE 8000
HEALTHCHECK --interval=15s --timeout=3s --retries=10 \
    CMD curl -fs http://localhost:8000/api/config || exit 1
CMD ["uvicorn", "backend.app:app", "--host", "0.0.0.0", "--port", "8000"]
