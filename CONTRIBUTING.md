# Contributing

Thanks for improving Hive.

## Development

Requirements:

- Go 1.21 or newer
- Podman for runtime/manual testing

Useful commands:

```bash
go test ./...
go build ./...
```

Before opening a pull request:

- run `go test ./...`;
- run `gofmt` on changed Go files;
- update `README.md` when command flags, config keys, mounts, security behavior,
  or release behavior changes;
- add focused tests for security-sensitive behavior.

## Security-Sensitive Changes

Hive controls host mounts, token handling, and high-autonomy agent execution.
Changes in these areas need tests and clear documentation:

- mount mode or path validation;
- GitHub token transport and cleanup;
- Podman run flags;
- prompt or command execution;
- image build or release workflows.

## Contribution License

Unless you explicitly state otherwise, any contribution intentionally submitted
for inclusion in Hive is submitted under the Apache License, Version 2.0, the
same license as this repository.

## Code of Conduct

By participating, you agree to follow `CODE_OF_CONDUCT.md`.
