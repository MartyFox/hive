# Security Policy

## Supported Versions

Security fixes target the latest released version and the default branch.

## Reporting a Vulnerability

Do not report suspected vulnerabilities in public issues.

Use GitHub private vulnerability reporting if it is enabled for this repository.
If it is not enabled, contact the project maintainer privately and include:

- affected version or commit;
- host OS and Podman version;
- exact Hive command/config used;
- whether `--writable-config`, `--gh-token`, or extra mounts were enabled;
- reproduction steps and expected impact.

## Security Scope

Hive is a local isolation tool for AI agent CLIs. Reports are especially useful
for:

- host filesystem access outside `/workspace`, configured agent paths, Hive
  state, or explicit extra mounts;
- writable host mounts being enabled without explicit user/config opt-in;
- GitHub token exposure through process args, logs, temp files, or failed
  cleanup;
- container privilege escalation beyond the intended Podman flags;
- prompt/command injection where non-shell prompt paths execute shell content.

Expected behavior:

- `/workspace` is mounted read-write from the current directory.
- agent config and shared skills mount read-only by default.
- `~/.hive/state/<agent>` is writable.
- extra mounts are explicit and must target `/mnt/...`.
- GitHub token injection is off by default.
- `--cmd` intentionally runs through a shell.
- Claude and Copilot images use high-autonomy flags (`--dangerously-skip-permissions`
  and `--yolo`) inside the container, so untrusted prompts or commands can still
  modify anything writable in `/workspace`.

## Disclosure

Please allow time for a fix and release before public disclosure. The project
will credit reporters unless they request otherwise.
