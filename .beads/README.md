# Issue tracking

This project uses Beads (`bd`) with embedded Dolt storage. Issue IDs use the `herdr-top` prefix. Issue history syncs publicly to this project's GitHub repository through a Dolt remote.

```sh
bd ready
bd create "Describe the work"
bd show <issue-id>
bd update <issue-id> --claim
bd close <issue-id> --reason "Completed"
```

Use `bd dolt pull` before collaborating and `bd dolt push` after issue updates. Do not put secrets or private machine data in tickets: synced issue history is public.

The local database, backups, locks, and credentials are ignored by Git. Only configuration, metadata, and hook scripts are tracked. The local Dolt remote is derived from the checkout's Git origin rather than hard-coding a developer-specific SSH alias in shared configuration.

To initialize a new checkout from the public issue history:

```sh
bd init --prefix herdr-top --skip-agents --non-interactive \
  --remote git+https://github.com/abhirup-dev/herdr-top.git
```

`--skip-agents` preserves the existing `AGENTS.md`. Run `bd prime` for Beads workflow guidance without generating a large agent-instruction section.
