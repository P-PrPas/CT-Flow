"""App-wide constants.

MODE decides which filesystem the directory picker walks:
  local -> the server runs on your own PC, so browsing the server IS browsing
           your PC. Roots = all drives.
  vm    -> the server runs on a shared VM. Only VM_DATA_ROOT is browsable, and
           your datasets have to live there (a mounted share, usually).
Set with the LABEL_TOOL_MODE env var.
"""
import os
import string
from pathlib import Path

BACKEND_ROOT = Path(__file__).resolve().parent          # label_tool/backend
REPO_ROOT = BACKEND_ROOT.parent.parent                   # poc-visual-prompt
# In Docker the weight is baked in at /models; outside it lives next to the POC.
MODEL_PATH = os.getenv("MODEL_PATH", str(REPO_ROOT / "poc" / "yoloe-11s-seg.pt"))

MODE = os.getenv("LABEL_TOOL_MODE", "local").lower()
VM_DATA_ROOT = Path(os.getenv("LABEL_TOOL_VM_ROOT", "/data"))


def browse_roots() -> list[str]:
    if MODE == "vm":
        return [str(VM_DATA_ROOT)]
    if os.name == "nt":
        return [f"{d}:\\" for d in string.ascii_uppercase if Path(f"{d}:\\").exists()]
    return ["/"]


def path_allowed(p: Path) -> bool:
    """vm mode is a trust boundary: the browser can send any path string, so
    confine it to VM_DATA_ROOT. local mode is single-user on your own PC."""
    if MODE != "vm":
        return True
    try:
        p.resolve().relative_to(VM_DATA_ROOT.resolve())
        return True
    except ValueError:
        return False


LABEL_COLORS = [
    "#7dd8ff", "#f97316", "#34d399", "#a78bfa",
    "#fbbf24", "#fb7185", "#22d3ee", "#84cc16",
]

DEFAULT_CONF = 0.25
IMAGE_EXTS = {".jpg", ".jpeg", ".png", ".bmp"}
