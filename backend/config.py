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
PKG_ROOT = BACKEND_ROOT.parent                            # label_tool -- this repo's root
REPO_ROOT = PKG_ROOT.parent                                # poc-visual-prompt -- shared data/ lives here, see _experiment_conf.py
# Where YOLOE checkpoints live -- ultralytics auto-downloads a missing one by
# filename into here the first time it's selected (see services/models.py),
# so nothing has to be pre-baked or pre-fetched. Repo-local, unlike data/:
# unlike datasets there's no reason to share weights with the POC checkout.
# In Docker this is a named volume so a restart doesn't re-download.
MODELS_DIR = os.getenv("MODELS_DIR", str(PKG_ROOT / "models"))

MODE = os.getenv("LABEL_TOOL_MODE", "local").lower()
VM_DATA_ROOT = Path(os.getenv("LABEL_TOOL_VM_ROOT", "/opt/mount/project"))


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
