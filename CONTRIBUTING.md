# Contributing

## Development

Run `make` to see available targets:

```bash
make              # list targets
make build        # build the binary
make check        # run all checks (lint + tests)
make test         # unit tests
make test-integration  # integration tests (require jj)
```

Build with a specific version (for releases):

```bash
make build VERSION=0.2.0
```

## Integration test tips

```bash
# Verbose output (shows jj log, resolved DAGs with parent info)
go test -tags integration -v ./...

# Keep test jj repos for manual inspection
JIP_KEEP_REPO=1 go test -tags integration -v -run TestIntegration_OverlappingRevsets ./internal/jj/
# The repo path is printed in the test output, e.g.:
#   repo: /tmp/jip-integration-1234567890
# You can then inspect it:
#   jj -R /tmp/jip-integration-1234567890 log -r ::
```

## Releasing

Releases are automated with [GoReleaser](https://goreleaser.com/) via GitHub
Actions. Tag a version and push:

```bash
jj tag set v0.1.0 -r main
git push --tags
```

This runs the full check suite, then builds binaries for Linux, macOS, and
Windows (amd64 + arm64) and publishes a GitHub Release.
