"""Login/logout/who-am-I. Always mounted, even with auth switched off --
the UI asks /api/auth/me on load, and "auth is off, you are anonymous" is a
useful answer, not an error."""
from fastapi import APIRouter, HTTPException, Request, Response

from ..services import auth

router = APIRouter(prefix="/api/auth", tags=["auth"])


@router.get("/me")
def me(request: Request):
    """Always reachable, signed in or not (see app.py's PUBLIC allowlist) --
    the UI calls this on load to decide whether to draw a login screen at
    all. `enabled: false` means the server has no `LABEL_TOOL_USERS`
    configured, i.e. every other endpoint is open to anyone who can reach it."""
    return {"enabled": auth.enabled(),
            "user": auth.identify(request.cookies.get(auth.COOKIE))}


@router.post("/login")
def login(req: dict, response: Response, request: Request):
    """Sets an httponly session cookie on success. `400` if the server has no
    `LABEL_TOOL_USERS` configured (nothing to log into); `401` for a wrong
    username *or* password -- deliberately the same message for both, so a
    failed attempt can't be used to enumerate valid usernames."""
    if not auth.enabled():
        raise HTTPException(400, "auth is not configured on this server")
    username = req["username"]
    if not auth.check(username, req["password"]):
        # One message for both wrong-user and wrong-password: which of the two
        # it was is exactly what someone probing usernames wants to learn.
        raise HTTPException(401, "wrong username or password")
    response.set_cookie(
        auth.COOKIE, auth.issue(username),
        max_age=auth.TTL_SECONDS, httponly=True, samesite="lax",
        # Only when the browser already reached us over TLS -- a hard-coded
        # secure=True would silently drop the cookie on a plain-http LAN
        # deployment, which is how this gets run today (NFR-08).
        secure=request.url.scheme == "https",
    )
    return {"enabled": True, "user": username}


@router.post("/logout")
def logout(response: Response):
    response.delete_cookie(auth.COOKIE)
    return {"enabled": auth.enabled(), "user": None}
