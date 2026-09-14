# deploy/

Ready-to-copy layout for the VM, per `docs/deploying-a-service.md`. Structure
mirrors that doc's `~/core` / `~/app` split -- no `~/service` folder, nothing
here is shared with another app.

## First time on a fresh VM

```bash
docker network create --driver=bridge --subnet=172.20.0.0/16 bridge0
docker login container3.connectedtech.dev   # watchtower reads this same
                                             # /root/.docker/config.json to
                                             # pull new tags automatically
scp -r deploy/* vm:~/           # or git clone the repo there and symlink
cd ~/core && docker compose up -d
cd ~/app/ctflow && cp .env.example .env && vim .env && docker compose up -d
```

## Redeploying a new version

Push a `v*` tag on GitHub (builds + pushes to both ghcr.io and
container3.connectedtech.dev, both `:<tag>` and `:latest`). Then on the VM
either:

- run with `TAG=latest` (the default) and just wait -- `cr-watchtower` polls
  every minute and restarts anything labeled
  `com.centurylinklabs.watchtower.enable=true` once `:latest`'s digest on the
  registry changes, which is every push, or
- pin `TAG=` in `app/ctflow/.env` to an exact version and
  `docker compose up -d` yourself -- a version tag's digest never changes
  once pushed, so watchtower leaves a pinned deploy alone.
