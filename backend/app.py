"""FastAPI app for the Next.js labeling UI.

Dev (from label_tool/): .venv\\Scripts\\python.exe -m uvicorn backend.app:app --port 8000 --reload
"""
from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from .routers import auth as auth_router
from .routers import jobs, pool, system, testset, uploads
from .services import auth

TAGS = [
    {"name": "system", "description": "Environment info and the server-side folder picker. "
     "`GET /api/config` is the one endpoint every client call depends on -- it's the only one "
     "reachable without signing in besides /api/auth/*."},
    {"name": "auth", "description": "Login/logout/who-am-I. Opt-in: with `LABEL_TOOL_USERS` unset "
     "on the server, every endpoint below is open to anyone who can reach it and `enabled` is "
     "always `false`. Session is an httponly cookie, not a bearer token -- there's no `Authorization` "
     "header to set from a REST client; log in once and the cookie jar handles the rest."},
    {"name": "pool", "description": "The core labeling loop: open an image folder, save boxes into "
     "the prompt bank, get the model's suggestions for an image, review/fix a generated label. "
     "Everything here operates on one `input_dir` at a time -- the prompt bank, labels, and test-set "
     "manifest all live in a fixed subfolder of it (`.ctflow/`), so that's the only path a client ever "
     "has to send."},
    {"name": "jobs", "description": "Long inference passes over many images (score/evaluate/"
     "autolabel/reembed) run as a background job: the POST returns a `job_id` immediately, poll "
     "`GET /api/jobs/{job_id}` until `finished`. See each POST's docstring for what `result` looks "
     "like once it's done -- it differs per job."},
    {"name": "testset", "description": "Ground truth for the held-out test set that `POST "
     "/api/evaluate` measures against. Test images are pool images flagged in a manifest (no copy) "
     "-- these flags and their labels must never reach the prompt bank."},
    {"name": "upload", "description": "Drag-and-drop image intake for someone without filesystem "
     "access to the server. Refuses to run on a `vm`-mode deployment until auth is turned on."},
]

app = FastAPI(
    title="CT-Flow",
    description=(
        "Human-in-the-loop visual-prompt labeling tool for YOLOE. Draw one box, teach the model, "
        "let it pre-annotate the rest -- see the repo README for the full workflow.\n\n"
        "**Path safety:** any field that names a filesystem path (`input_dir`, an `image` path) is "
        "checked server-side against the deployment's allowed roots (`GET /api/config`'s "
        "`mode`/`roots`) and rejected with `403` if it's outside them -- the browser can send any "
        "string, so every such field is a trust boundary, not just a convenience.\n\n"
        "**Background jobs:** score/evaluate/autolabel/reembed all return `{job_id, total}` "
        "immediately and do the real work after the response -- poll `GET /api/jobs/{job_id}` "
        "(the `jobs` tag) until `finished` is `true`, then read `result` (shape documented on "
        "the endpoint that started the job) or `error`."
    ),
    version="1.0.0",
    openapi_tags=TAGS,
)
app.add_middleware(
    CORSMiddleware, allow_origins=["*"], allow_methods=["*"], allow_headers=["*"]
)

# Reachable without a session: the UI needs both before it can even draw a
# login box. Everything else is gated, so a route added later is protected by
# default rather than by remembering to protect it.
PUBLIC = {"/api/config", "/api/auth/me", "/api/auth/login", "/api/auth/logout"}


@app.middleware("http")
async def require_login(request: Request, call_next):
    """T-12 / FR-30. Inert until LABEL_TOOL_USERS is set -- see services/auth.py."""
    if request.method == "OPTIONS" or request.url.path in PUBLIC or not auth.enabled():
        return await call_next(request)
    if auth.identify(request.cookies.get(auth.COOKIE)) is None:
        return JSONResponse({"detail": "not signed in"}, status_code=401)
    return await call_next(request)


app.include_router(auth_router.router)
app.include_router(system.router)
app.include_router(pool.router)
app.include_router(testset.router)
app.include_router(jobs.router)
app.include_router(uploads.router)
