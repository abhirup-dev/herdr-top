# Contributing

Use Go 1.24 or newer on macOS. Keep the runtime a single Go binary and do not add shell or Python helpers.

Before submitting a change, run:

```sh
gofmt -w ./cmd ./internal
go test ./...
go test -race ./...
go vet ./...
make build
```

Use synthetic test fixtures. Never add process/environment dumps, session archives, host identifiers, credentials, or private workspace names. Tests must not mutate live Herdr focus or topology; use fake command runners. Preserve unavailable metric states and explicit per-session routing.
