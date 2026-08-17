# Build context is label_tool/ (this repo's root), see docker-compose.yml.
FROM python:3.12-slim

# opencv needs these; slim has neither
RUN apt-get update \
    && apt-get install -y --no-install-recommends libgl1 libglib2.0-0 curl \
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

# CUDA build of torch. cu126 needs a host driver that supports CUDA 12.6 or
# newer -- check with `nvidia-smi`, the "CUDA Version" it prints is the
# ceiling your driver supports, and a driver only runs wheels built for that
# version or older (a cu128/cu130 wheel on a driver capped at 12.7 fails with
# "CUDA driver version is insufficient"). No NVIDIA GPU on the build/run
# host? Override at build time:
#   docker compose build --build-arg TORCH_INDEX_URL=https://download.pytorch.org/whl/cpu api
ARG TORCH_INDEX_URL=https://download.pytorch.org/whl/cu126
COPY backend/requirements.txt backend/requirements.txt
RUN pip install --no-cache-dir --extra-index-url ${TORCH_INDEX_URL} \
    -r backend/requirements.txt

# backend/ stays a real package (app.py, routers/, services/) so imports
# inside it (`from .routers import ...`, `from ..services.bank import Bank`)
# resolve the same way they do outside Docker.
COPY backend/ backend/

ENV MODELS_DIR=/models \
    LABEL_TOOL_MODE=vm \
    LABEL_TOOL_VM_ROOT=/data \
    YOLO_CONFIG_DIR=/tmp \
    MPLCONFIGDIR=/tmp \
    PYTHONUNBUFFERED=1

# NFR-07 -- not root. The UID has to match whoever owns the host folder you
# bind-mount at /data, or the container cannot write labels into it:
#   docker compose build --build-arg APP_UID=$(id -u) api
# Docker Desktop on Windows/macOS ignores mount ownership, so the default is
# only a real decision on Linux hosts.
ARG APP_UID=1000
# /models is a named volume (docker-compose.yml) so a checkpoint survives a
# restart instead of re-downloading. A named volume's first mount inherits
# whatever owns this directory in the image at that moment -- must happen
# before USER, or the volume comes up root-owned and `app` can't write into it.
RUN mkdir -p /models \
    && useradd --create-home --uid ${APP_UID} app \
    && chown -R app /app /models

# Bake the smallest/default and the two newest-generation checkpoints into the
# image so a *fresh* named volume auto-seeds from them on first mount (Docker
# copies a mount point's image content into an empty volume the first time it
# attaches) -- this is what makes them survive `docker compose down -v`,
# unlike a checkpoint that only ever landed in the volume via a runtime
# download or a manual `docker cp`. The rest of the catalog still
# auto-downloads into the volume on first use, same as before.
# ponytail: these three .pt files have to already exist under label_tool/models/
# on whoever's machine runs this build (see README.md#model-selection for the
# curl commands) -- COPY can't skip a source file that isn't there.
COPY models/yoloe-11s-seg.pt models/yoloe-26s-seg.pt models/yoloe-26x-seg.pt /models/
RUN chown app /models/*.pt
USER app

EXPOSE 8000
HEALTHCHECK --interval=15s --timeout=3s --retries=10 \
    CMD curl -fs http://localhost:8000/api/config || exit 1
CMD ["uvicorn", "backend.app:app", "--host", "0.0.0.0", "--port", "8000"]
