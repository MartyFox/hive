# Hive — Build and Release Pipeline

## Local Development Build

```bash
go build -o hive .
```

Produces a binary with default version metadata (`dev`, `unknown`, `unknown`). All embedded Containerfiles are included via `//go:embed images` in `internal/imgfs/imgfs.go`.

Run tests:

```bash
go test ./...
```

No Podman daemon is required to run the test suite — all Podman calls are injected via seam variables or skipped.

---

## Version Metadata

Version information is injected at link time using `-ldflags -X`. Three variables in `internal/version/version.go` receive values:

| Variable | ldflag target | Release value | Local default |
|---|---|---|---|
| `Version` | `github.com/MartyFox/hive/internal/version.Version` | Git tag (e.g. `v1.2.3`) | `dev` |
| `Commit` | `github.com/MartyFox/hive/internal/version.Commit` | First 7 chars of `GITHUB_SHA` | `unknown` |
| `BuildDate` | `github.com/MartyFox/hive/internal/version.BuildDate` | UTC timestamp (`2006-01-02T15:04:05Z` format) | `unknown` |

Full ldflag string used in the release workflow:

```
-ldflags "-s -w \
  -X github.com/MartyFox/hive/internal/version.Version=${RELEASE_TAG} \
  -X github.com/MartyFox/hive/internal/version.Commit=${GITHUB_SHA::7} \
  -X github.com/MartyFox/hive/internal/version.BuildDate=${build_date}"
```

`-s -w` strips the symbol table and DWARF debug info, reducing binary size. `-trimpath` removes local filesystem paths from the binary.

---

## Workflow: `build-images.yml`

**Trigger:** Push to `main`, or manual `workflow_dispatch`.

**Purpose:** Build all hive container images and push them to GHCR.

**Permissions required:** `contents: read`, `packages: write`

### Build order

1. Build `hive-base` from `internal/imgfs/images/base/Containerfile` using `internal/imgfs/images/base/` as the build context.
2. For each agent (`claude`, `copilot`, `gemini`, `codex`): build `hive-<agent>` from its Containerfile using `internal/imgfs/images/<agent>/` as the build context.

### Tags pushed

Each image receives two tags:

- `ghcr.io/<owner>/hive-<image>:latest`
- `ghcr.io/<owner>/hive-<image>:<full-commit-sha>`

The owner is lowercased before use (GHCR requires lowercase).

### Notes

- The workflow uses Docker Buildx (`docker/setup-buildx-action`) — not `podman build`. The resulting images are OCI-compatible and run under Podman.
- Agent images depend on a locally tagged `hive-base` (tagged without registry prefix) in the same job. The base image is built and tagged locally first.
- `GITHUB_TOKEN` is used for GHCR authentication — no additional secrets required.
- Build args (`HIVE_BEADS`, `HIVE_BEADS_VERSION`) are **not** passed during the CI image build. GHCR images are built without Beads. Users who need Beads must build locally with `hive build`.

---

## Workflow: `release-binary.yml`

**Trigger:** Push of a `v*` tag, or manual `workflow_dispatch` with a `tag` input.

**Purpose:** Cross-compile static binaries for four platforms and create a GitHub Release.

**Permissions required:** `contents: write`

### Matrix

| `GOOS` | `GOARCH` | Binary name |
|---|---|---|
| `linux` | `amd64` | `hive_linux_amd64` |
| `linux` | `arm64` | `hive_linux_arm64` |
| `darwin` | `amd64` | `hive_darwin_amd64` |
| `darwin` | `arm64` | `hive_darwin_arm64` |

All targets build in parallel (`fail-fast: false`).

### Build flags

```bash
CGO_ENABLED=0 GOOS=<goos> GOARCH=<goarch> \
  go build -trimpath \
    -ldflags "-s -w -X ..." \
    -o dist/hive_<goos>_<goarch> .
```

`CGO_ENABLED=0` produces a fully static binary with no libc dependency.

### Release job

After all matrix builds complete, the `release` job:

1. Downloads all binary artifacts
2. Creates a GitHub Release via `softprops/action-gh-release`
3. Attaches binaries plus `LICENSE`, `NOTICE`, and `THIRD_PARTY_NOTICES.md`
4. Uses `generate_release_notes: true` — GitHub auto-generates a changelog from PRs merged since the last release

### Triggering a release

**Via tag push:**

```bash
git tag v1.2.3
git push origin v1.2.3
```

**Via manual dispatch:**

Use the GitHub Actions UI and supply the tag name. The workflow creates the release under that tag name pointing at the current `HEAD`.

---

## Embedded Containerfiles

The `internal/imgfs/images/` tree is compiled into the binary via:

```go
//go:embed images
var FS embed.FS
```

**Adding or modifying a Containerfile** does not require any code change — `go build` picks up the updated file automatically. The constraint is that the directory structure must match what `extractBuildContextFromFS` and `buildTarget` expect:

```
images/
  base/Containerfile
  claude/Containerfile
  copilot/Containerfile
  gemini/Containerfile
  codex/Containerfile
```

Agent names in the image tree must match the agent registry slice in `internal/podman/podman.go`.

---

## OCI Labels

All Containerfiles include standard OCI image labels:

```dockerfile
LABEL org.opencontainers.image.source="https://github.com/MartyFox/hive" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.description="..."
```

These make images discoverable on GHCR and link them back to the source repo.
