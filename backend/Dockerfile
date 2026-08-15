# Build context is the repo root (the model weight lives in ../../poc from here).
FROM python:3.12-slim

# opencv needs these; slim has neither
RUN apt-get update \
    && apt-get install -y --no-install-recommends libgl1 libglib2.0-0 curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Any .crt dropped in label_tool/certs/ becomes trusted at build time. Needed
# when something intercepts TLS between the build and pypi -- a corporate
# proxy, or antivirus HTTPS scanning (AVG/Avast/Kaspersky all do this).
# Empty folder = no-op.
COPY label_tool/certs/ /usr/local/share/ca-certificates/
RUN update-ca-certificates
ENV PIP_CERT=/etc/ssl/certs/ca-certificates.crt \
    REQUESTS_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt \
    SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt

# CPU-only torch: the full CUDA wheel is ~2.5GB and this is a labeling tool.
# Swap to the default index + an nvidia base image if you ever want GPU.
COPY label_tool/backend/requirements.txt backend/requirements.txt
RUN pip install --no-cache-dir --extra-index-url https://download.pytorch.org/whl/cpu \
    -r backend/requirements.txt

COPY poc/yoloe-11s-seg.pt /models/yoloe-11s-seg.pt
# backend/ stays a real package (app.py, routers/, services/) so imports
# inside it (`from .routers import ...`, `from ..services.bank import Bank`)
# resolve the same way they do outside Docker.
COPY label_tool/backend/ backend/

ENV MODEL_PATH=/models/yoloe-11s-seg.pt \
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
RUN useradd --create-home --uid ${APP_UID} app && chown -R app /app
USER app

EXPOSE 8000
HEALTHCHECK --interval=15s --timeout=3s --retries=10 \
    CMD curl -fs http://localhost:8000/api/config || exit 1
CMD ["uvicorn", "backend.app:app", "--host", "0.0.0.0", "--port", "8000"]
