# herdr-top

`herdr-top` is a compact, local macOS process monitor for coding agents running in Herdr. It installs as one Go executable; it does not start or stop Herdr, send agent prompts, or kill processes.

## Prerequisites and install

- macOS on Apple silicon or Intel
- Go 1.24 or newer
- `herdr` installed and available on `PATH`; sessions must already be running

From a checkout:

```sh
go install ./cmd/herdr-top
# or
make install GOBIN="$HOME/.local/bin"
```

The installed executable is `herdr-top`. `go install` uses Go's configured install directory; ensure it is on `PATH`. The explicit `GOBIN` above selects an exact destination even when your environment already sets `GOBIN`. `make uninstall` uses the same `GOBIN`/`PREFIX` selection.

## Use

```sh
herdr-top                         # interactive TUI
herdr-top --list --sort cpu       # one human-readable snapshot
herdr-top --json --sample 750ms   # one scriptable snapshot
herdr-top --help
```

Interactive keys: arrows or `j`/`k` move; `m`, `c`, `i`, and `o` sort by memory, CPU, cumulative I/O, and age; `r` refreshes; Enter focuses the exact mapped Herdr agent pane; `q` quits. Focus waits for and displays server failures. It does not raise a macOS terminal window.

## Metric definitions

- **FOOT** is Darwin `ri_phys_footprint`, summed across the agent process tree. It is physical footprint, not RSS.
- **RSS** is current resident size, shown separately and summed across the tree.
- **CPU%** is `(change in cumulative user + system CPU time) / the process's matching observation interval × 100`, summed across the tree. One fully busy core is approximately 100%; multi-process trees can exceed 100%. It is never a lifetime average.
- **DISKΣ** is cumulative bytes read plus written from Darwin's `DiskReadBytes` and `DiskWriteBytes`, summed across the tree. It is not a current transfer rate and does not use gopsutil's generic byte fields.
- **AGE** is time since process start.

A dash (`—`) means unavailable. Permission errors, process races, and incomplete tree metrics remain unavailable with diagnostics rather than becoming zero. JSON includes a `valid` flag and error text.

## Session mapping, privacy, and limitations

The tool enumerates every running **local** Herdr session and explicitly invokes every query with `--session <name>`, including `default`. Caller session/socket routing variables are removed from subprocesses. A row is rooted at the launcher PID identified by that exact session and pane's `pane process-info` foreground process group, so generic launchers such as Pi's `node` are included without turning helper processes into duplicate agents. Workspace IDs alone are never used. Ambiguous and stale mappings are rejected with diagnostics.

Only local processes and local Herdr sessions are supported. No remote-server inventory is claimed. Recognizable outside-Herdr launchers such as `claude.exe` can be shown, but an unmapped generic `node` process is deliberately omitted because it cannot safely be identified as Pi. Full process command lines and environments are not collected or printed. JSON can contain local workspace labels and working directories returned by Herdr, so treat captured output as private machine data.

Protected processes can deny metrics. Process exit/start races can make a snapshot partial. The physical-footprint adapter uses Apple's shipped but private libproc interface, which Apple may change. Interactive focus changes server-side Herdr focus only; OS window raising is intentionally out of scope.

## Implementation dependencies

Core process data uses [`gopsutil/v4` v4.26.7](https://github.com/shirou/gopsutil). A Darwin-only, typed `rusage_info_v2` adapter uses [`purego` v0.10.2](https://github.com/ebitengine/purego) to read physical footprint without CGO. The terminal UI uses [`bubbletea` v1.3.10](https://github.com/charmbracelet/bubbletea). Versions are pinned in `go.mod`; see `THIRD_PARTY_NOTICES.md`.

## Issue tracking

Use Beads (`bd`) for project tickets. See [the issue-tracking guide](.beads/README.md) for commands and public-history sync; avoid putting private data in tickets.
