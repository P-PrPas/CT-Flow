"""FastAPI app for the Next.js labeling UI.

Dev (from label_tool/): .venv\\Scripts\\python.exe -m uvicorn backend.app:app --port 8000 --reload
"""
from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from .routers import auth as auth_router
from .routers import jobs, pool, system, testset, uploads
from .services import auth

app = FastAPI(title="CT-Flow")
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
