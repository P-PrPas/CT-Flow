"""Login/logout/who-am-I. Always mounted, even with auth switched off --
the UI asks /api/auth/me on load, and "auth is off, you are anonymous" is a
useful answer, not an error."""
from fastapi import APIRouter, HTTPException, Request, Response
from pydantic import BaseModel

from ..services import auth

router = APIRouter(prefix="/api/auth", tags=["auth"])


class LoginReq(BaseModel):
    username: str
    password: str


@router.get("/me")
def me(request: Request):
    return {"enabled": auth.enabled(),
            "user": auth.identify(request.cookies.get(auth.COOKIE))}


@router.post("/login")
def login(req: LoginReq, response: Response, request: Request):
    if not auth.enabled():
        raise HTTPException(400, "auth is not configured on this server")
    if not auth.check(req.username, req.password):
        # One message for both wrong-user and wrong-password: which of the two
        # it was is exactly what someone probing usernames wants to learn.
        raise HTTPException(401, "wrong username or password")
    response.set_cookie(
        auth.COOKIE, auth.issue(req.username),
        max_age=auth.TTL_SECONDS, httponly=True, samesite="lax",
        # Only when the browser already reached us over TLS -- a hard-coded
        # secure=True would silently drop the cookie on a plain-http LAN
        # deployment, which is how this gets run today (NFR-08).
        secure=request.url.scheme == "https",
    )
    return {"enabled": True, "user": req.username}


@router.post("/logout")
def logout(response: Response):
    response.delete_cookie(auth.COOKIE)
    return {"enabled": auth.enabled(), "user": None}
