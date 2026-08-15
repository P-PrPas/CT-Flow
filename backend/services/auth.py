"""Password login (T-12 / FR-30) and who-labeled-what (FR-31).

**Off unless you configure users.** With no users the whole app behaves
exactly as before -- that is the "one person, own PC" case the tool started
as, and adding a login screen to it would be pure friction. Configure users
and every endpoint except /api/config and /api/auth/* needs a session cookie.

    python -m backend.services.auth alice hunter2

prints a `name:hash` pair for LABEL_TOOL_USERS (comma-separated for several).
The plaintext password is never stored or logged anywhere.

ponytail: pbkdf2 + an HMAC-signed cookie, both stdlib -- no new dependency,
no user database, no password reset. This is a lock on the door, not an
identity system; move to real SSO the day someone asks for it.
"""
import hashlib
import hmac
import os
import secrets
import time

COOKIE = "labeltool_session"
TTL_SECONDS = 12 * 3600
ITERATIONS = 240_000

# ponytail: a generated secret means restarting the API signs everyone out.
# That is the right default (no secret sitting in a repo); set
# LABEL_TOOL_SECRET when logging out on every deploy starts to annoy people.
_SECRET = (os.getenv("LABEL_TOOL_SECRET") or secrets.token_hex(32)).encode()


def hash_password(password: str, salt: str | None = None) -> str:
    salt = salt or secrets.token_hex(16)
    dk = hashlib.pbkdf2_hmac("sha256", password.encode(), bytes.fromhex(salt), ITERATIONS)
    return f"pbkdf2${ITERATIONS}${salt}${dk.hex()}"


def verify_password(password: str, stored: str) -> bool:
    try:
        algo, iters, salt, want = stored.split("$")
        if algo != "pbkdf2":
            return False
        dk = hashlib.pbkdf2_hmac("sha256", password.encode(), bytes.fromhex(salt), int(iters))
    except (ValueError, TypeError):
        return False
    return hmac.compare_digest(dk.hex(), want)


def users() -> dict[str, str]:
    """LABEL_TOOL_USERS="alice:pbkdf2$...,bob:pbkdf2$..." -> {name: hash}.
    Read per call, not cached, so `docker compose up -d` picks up a new user
    without a code change."""
    out = {}
    for entry in os.getenv("LABEL_TOOL_USERS", "").split(","):
        name, sep, stored = entry.strip().partition(":")
        if sep and name and stored:
            out[name] = stored
    return out


def enabled() -> bool:
    return bool(users())


def check(username: str, password: str) -> bool:
    stored = users().get(username)
    # Hash even when the user does not exist, so a wrong username and a wrong
    # password take the same time to reject.
    return verify_password(password, stored or hash_password("no-such-user"))


def _sign(body: str) -> str:
    return hmac.new(_SECRET, body.encode(), hashlib.sha256).hexdigest()


def issue(user: str) -> str:
    body = f"{user}|{int(time.time()) + TTL_SECONDS}"
    return f"{body}|{_sign(body)}"


def identify(token: str | None) -> str | None:
    """The user this cookie proves, or None. None covers every failure --
    absent, tampered, expired, malformed -- because the caller's response to
    all of them is the same 401."""
    if not token:
        return None
    # rsplit, not split: the signature and expiry are the last two fields, so
    # a user name containing "|" can never shift them.
    parts = token.rsplit("|", 2)
    if len(parts) != 3:
        return None
    user, exp, sig = parts
    if not hmac.compare_digest(sig, _sign(f"{user}|{exp}")):
        return None
    try:
        return user if int(exp) > time.time() else None
    except ValueError:
        return None


def demo():
    h = hash_password("hunter2")
    assert verify_password("hunter2", h)
    assert not verify_password("hunter3", h)
    assert not verify_password("hunter2", "garbage")

    os.environ["LABEL_TOOL_USERS"] = f"alice:{h}"
    assert enabled() and check("alice", "hunter2")
    assert not check("alice", "wrong") and not check("mallory", "hunter2")

    tok = issue("alice")
    assert identify(tok) == "alice"
    assert identify(tok[:-1] + ("0" if tok[-1] != "0" else "1")) is None  # tampered signature
    assert identify("alice|9999999999|deadbeef") is None                  # forged
    assert identify(f"alice|1|{_sign('alice|1')}") is None                # correctly signed, expired
    assert identify(None) is None and identify("nonsense") is None

    # A user name containing the separator must still round-trip, not become
    # a way to shift the expiry field.
    assert identify(issue("a|b")) == "a|b"

    del os.environ["LABEL_TOOL_USERS"]
    assert not enabled()
    print("auth self-check OK")


if __name__ == "__main__":
    import sys

    if len(sys.argv) == 3:
        print(f"{sys.argv[1]}:{hash_password(sys.argv[2])}")
    else:
        demo()
        print("usage: python -m backend.services.auth <username> <password>")
