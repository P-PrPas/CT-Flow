"""Shared FastAPI dependencies: every router that accepts a filesystem path
from the browser has to run it through checked_path before touching disk,
and anything that records provenance asks current_user who is doing it."""
from pathlib import Path

from fastapi import HTTPException, Request

from . import config
from .services import auth


def checked_path(path: str) -> Path:
    p = Path(path)
    if not config.path_allowed(p):
        raise HTTPException(403, f"path outside {config.VM_DATA_ROOT} (vm mode)")
    return p


def current_user(request: Request) -> str | None:
    """FR-31. None means "auth is off", not "unauthenticated" -- the
    middleware in app.py already rejected the second case before we get
    here."""
    return auth.identify(request.cookies.get(auth.COOKIE))
