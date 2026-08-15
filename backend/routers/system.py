"""Environment/filesystem info: what mode is this deployment in, and what
can the folder picker see."""
from fastapi import APIRouter, HTTPException

from .. import config
from ..deps import checked_path
from ..services.images import list_images

router = APIRouter(prefix="/api", tags=["system"])


@router.get("/config")
def get_config():
    return {"mode": config.MODE, "roots": config.browse_roots(),
            "colors": config.LABEL_COLORS}


@router.get("/browse")
def browse(path: str = ""):
    """Directory picker backing store. Lists subdirectories of `path` plus how
    many images each level holds, so you can tell a dataset folder from a
    container folder."""
    if not path:
        return {"path": "", "parent": None, "dirs": [], "images": 0,
                "roots": config.browse_roots()}
    p = checked_path(path)
    if not p.is_dir():
        raise HTTPException(404, "not a directory")
    dirs = []
    try:
        for child in sorted(p.iterdir()):
            if child.is_dir() and not child.name.startswith("."):
                dirs.append({"name": child.name, "path": str(child)})
    except PermissionError:
        pass
    parent = str(p.parent) if p.parent != p and config.path_allowed(p.parent) else None
    return {"path": str(p), "parent": parent, "dirs": dirs,
            "images": len(list_images(str(p))), "roots": config.browse_roots()}
