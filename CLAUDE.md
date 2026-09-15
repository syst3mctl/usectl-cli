# CLAUDE.md

Cobra CLI. Released by GoReleaser from `v*` tags.

## This repo is one part of a larger workspace

It is cloned as a sibling inside **`manager-usectl-setup`**, which holds the K8s
manifests, the shared docs and the canonical guidance.

**Read the workspace CLAUDE.md first — it is the authoritative one:**

    ~/Documents/Projects/manager-usectl-setup/CLAUDE.md

It covers the architecture, the deploy pipeline, the addon system, machine
groups, the migration rules and the build/deploy commands. Do not duplicate any
of that here — edit it there.

## Platform knowledge lives in the Obsidian vault

    ~/vault/usectl/         (entry point: usectl.md)
    git@github.com:j3dyy/obisidan-vault.git    (private)

69 linked notes on infrastructure, network isolation, data, security,
observability, runbooks and the recurring failure patterns. **That vault is the
source of truth for how the platform actually behaves.** `knowledge-for-gem/` in
the workspace is older and has been wrong more than once.

Start at `usectl.md`, or `00 Maps/Triage.md` if something is broken.

## Local commands

    make build
    make install
    go build -o usectl .

## Worth knowing in this repo

**`usectl apps` does not exist** — removed in v2.0.0. It is `usectl machines pods ...`, and pods are addressed by **name, not UUID**. `machines create` takes no `--repo/--domain/--branch/--type/--port`; those are `pods create` flags. Verify commands against `cmd/*.go`, never against `knowledge-for-gem/`.

## Conventions

- **No test suite anywhere in this project.** Do not invent test commands, and do
  not write `_test.go` files unless asked.
- Commit messages are plain — no attribution trailers.
- Prefer leaving changes uncommitted and saying what to run.
