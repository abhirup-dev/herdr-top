# Agent guide

- Scope is a single local macOS executable in `cmd/herdr-top`.
- Never start/stop Herdr servers, send prompts, kill processes, or mutate live topology in tests.
- Every session-scoped Herdr invocation must include `--session`, including `default`, and subprocesses must isolate inherited routing selectors.
- Map processes only through session-specific `pane process-info`; pane and workspace IDs are server-scoped.
- Keep physical footprint, RSS, cumulative disk bytes, and sampled CPU semantically distinct. An unavailable value is not zero.
- Do not inspect process environments or construct shell commands from process data. Use direct argv execution.
- Keep Darwin ABI code isolated in `internal/footprint`, with named fields matching Apple headers and no numeric field indexing.
- Use synthetic fixtures and run formatting, tests, race tests, vet, and build before handoff.
