# Registry — maintainer guide

This document covers publishing hive images to a container registry so end users
can pull pre-built images instead of building locally.

## How image resolution works

```
hive run copilot
  ├─ local image hive-copilot exists?         → use it
  ├─ pull <HIVE_REGISTRY>/hive-copilot:latest → tag + use it
  └─ pull failed                              → build from embedded Containerfiles
```

`HIVE_REGISTRY` defaults to `ghcr.io/martinf`. End users can override it in
`~/.hive/config` to point at a team or private registry.

The registry is a convenience layer only. `hive build` always works offline and
is the primary path. The registry lets users on a fresh machine skip the build.

## Setting up ghcr.io (GitHub Container Registry)

**Prerequisites:**
- GitHub account with write access to the `martinf/hive` repository (or a fork)
- A Personal Access Token (PAT) with `write:packages` scope

**1. Create a PAT:**

GitHub → Settings → Developer settings → Personal access tokens → Tokens (classic)
→ New token → select `write:packages` (automatically includes `read:packages`).

**2. Authenticate podman to ghcr.io:**

```bash
export CR_PAT=ghp_your_token_here
echo "$CR_PAT" | podman login ghcr.io -u YOUR_GITHUB_USERNAME --password-stdin
```

Credentials are stored in `~/.config/containers/auth.json` (or the system keychain
on macOS) and reused for all subsequent pulls and pushes.

**3. Build and push images:**

```bash
# Build all images locally first
hive build

# Tag and push each agent image
for agent in claude copilot gemini codex; do
  podman tag hive-$agent ghcr.io/martinf/hive-$agent:latest
  podman push ghcr.io/martinf/hive-$agent:latest
done

# Also push the base image (required for agent Containerfiles that reference hive-base)
podman tag hive-base ghcr.io/martinf/hive-base:latest
podman push ghcr.io/martinf/hive-base:latest
```

**4. Make images public (optional — allows anonymous pull):**

GitHub → your package → Package settings → Change visibility → Public.

Without this, users must authenticate to ghcr.io before `hive run` can pull.

## Automating builds with GitHub Actions

Create `.github/workflows/build-images.yml` to rebuild and push on every push to `main`:

```yaml
name: Build and push hive images

on:
  push:
    branches: [main]
  workflow_dispatch:

jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write

    steps:
      - uses: actions/checkout@v4

      - name: Log in to ghcr.io
        run: echo "${{ secrets.GITHUB_TOKEN }}" | podman login ghcr.io -u ${{ github.actor }} --password-stdin

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Build hive binary
        run: go build -o hive .

      - name: Build all images
        run: ./hive build

      - name: Push images
        run: |
          for agent in base claude copilot gemini codex; do
            podman tag hive-$agent ghcr.io/${{ github.repository_owner }}/hive-$agent:latest
            podman push ghcr.io/${{ github.repository_owner }}/hive-$agent:latest
          done
```

## Using a team or private registry

Set `HIVE_REGISTRY` in `~/.hive/config`:

```ini
HIVE_REGISTRY=ghcr.io/my-org
```

hive will pull `ghcr.io/my-org/hive-<agent>:latest` instead of the default.
Authenticate once with `podman login ghcr.io -u ... --password-stdin` and hive
uses the stored credentials automatically.

Team members with the same `HIVE_REGISTRY` setting get consistent pre-built images
without needing the Go toolchain or a full local build.
