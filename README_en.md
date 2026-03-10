# CCClaw

```text
  o-o   o-o   o-o o
 /     /     /    |
O     O     O     |  oo o   o   o
 \     \     \    | | |  \ / \ /
  o-o   o-o   o-o o o-o-  o   o
```

> A long-running Claude Code orchestration agent focused on reducing token waste

Chinese is the default documentation. See [README.md](README.md) for the primary version.

`CCClaw` can also be shortened to `3Claw`. The name is intentionally open-ended:

- `Claude | Common | Chaos | ...`
- `Code | Continuance | Calm | ...`
- `Claw`

## Overview

`ccclaw` is a Linux-native orchestration tool built around:

- **GitHub Issues** as the task source of truth
- **Claude Code** as the execution engine
- **systemd --user** as the background scheduler
- A dedicated **home knowledge repository** as long-term memory

The current repository state has reached the `phase0.4` milestone: the first installable release flow is already in place.

The point of `ccclaw` is not to make agents look flashy. It is to move repetitive operational work out of expensive model time:

- polling and scheduling
- approval gates
- fixed runtime layout
- long-term memory persistence
- multi-repo routing
- open-source collaboration control

## Key Strengths

- **Token-aware by design**: deterministic work stays in local CLI logic and `systemd`; Claude Code is used when real execution is needed.
- **Issue-driven workflow**: it fits the way open-source teams already work.
- **Linux-first**: no Docker, Kubernetes, or hosted control plane required for the default setup.
- **Program tree and memory tree are separated**: upgrades do not overwrite user memory under `/opt/ccclaw`.
- **Strict config boundaries**: secrets live in `.env`, normal config lives in `.toml`, and runtime does not depend on pre-exported shell variables.
- **Open-source safe gate**: admin-created issues can execute automatically; non-admin issues require `/ccclaw approve`.
- **Release-ready packaging**: releases are built from `src/dist/` and include at least the install package plus `SHA256SUMS`.

## Best Fit

- Solo maintainers running Claude Code across multiple GitHub repositories
- Small teams that want background automation without abandoning Issues as the audit trail
- Long-running engineering workflows that need persistent memory, reports, and operational history
- Teams trying to reduce token burn from repetitive orchestration overhead

## Current Status

- Stage: `phase0.4`
- Release status: first installable release flow is available
- Default platform: `linux/amd64`
- Default app dir: `~/.ccclaw`
- Default home repo: `/opt/ccclaw`
- Home repo modes: `init | remote | local`
- Task repo modes: `none | remote | local`
- Default scheduler: `systemd --user`
- Version format: `yy.mm.dd.HHMM`

## Topology

```text
GitHub Issue
    |
    v
ccclaw ingest/run
    |
    +--> admin gate / approve gate
    +--> target repo route
    +--> Claude Code executor
    +--> docs + reports + kb memory
    |
systemd --user timers
```

Default installed layout:

```text
~/.ccclaw
├── bin/
│   ├── ccclaw
│   └── ccclaude
├── .env
├── log/
├── var/
└── ops/
    ├── config/config.toml
    ├── systemd/
    └── scripts/

/opt/ccclaw
├── kb/
│   ├── designs/
│   ├── assay/
│   ├── journal/
│   └── skills/
│       ├── L1/
│       └── L2/
└── docs/
    ├── reports/
    ├── plans/
    └── rfcs/
```

## Why CCClaw

Many agent projects lead with multi-model routing, dashboards, chat UX, or cloud infrastructure. `ccclaw` deliberately starts with the harder operational basics:

1. GitHub Issues remain the task truth source
2. installation layout is fixed and predictable
3. secrets and normal config are separated
4. memory survives upgrades
5. open-source execution is gated by permission

That makes it a better fit for low-noise, auditable, long-running engineering work.

## Prerequisites

Prepare these before downloading a release.

### Accounts and Credentials

- A working GitHub account
- A GitHub token with access to the target repositories; it will be used by `gh` and later stored as `GH_TOKEN` in `.env`
- One of the following Claude access paths:
  - a valid Claude Code subscription/account and a usable login/auth path
  - or an API proxy URL + token that can be used as the Claude backend

### Network and Region

- Best case: you are in a country/region officially supported by Claude Code
- If not, you need to verify your own network path, account setup, and compliance constraints yourself
- This README documents official install and proxy configuration paths only; it does not explain bypassing region checks

### Host Requirements

- Linux
- `systemd --user` recommended
- `sudo` recommended, because the installer may need to install packages and write into `/opt/ccclaw`
- Recommended packages: `git`, `gh`, `curl`, `wget`, `rg`, `sqlite3`, `golang`

### Recommended Pre-flight

```bash
gh auth login
gh auth status
```

If you plan to use proxy mode, make sure you already have:

- `ANTHROPIC_BASE_URL`
- `ANTHROPIC_AUTH_TOKEN`

## Download and Install

### 1. Download the latest release

Release page:

- <https://github.com/41490/ccclaw/releases>

With `gh`:

```bash
mkdir -p /tmp/ccclaw-release
cd /tmp/ccclaw-release
gh release download --repo 41490/ccclaw --pattern 'ccclaw_*_linux_amd64.tar.gz' --pattern 'SHA256SUMS'
```

### 2. Verify checksums

```bash
sha256sum -c SHA256SUMS
```

### 3. Extract and run the installer

```bash
tar -xzf ccclaw_*_linux_amd64.tar.gz
cd ccclaw_*_linux_amd64
bash install.sh
```

The interactive installer will ask for:

- app dir, default `~/.ccclaw`
- control repo, default `41490/ccclaw`
- home repo mode: `init | remote | local`
- home repo dir, default `/opt/ccclaw`
- task repo mode: `none | remote | local`
- `GH_TOKEN`
- optional Claude proxy settings

### 4. What the installer does

`install.sh` currently performs these real actions:

- probes `claude`, `gh`, `rg`, `sqlite3`, `rtk`, `git`, `node`, `npm`, `uv`
- checks whether the official Claude install channel is reachable
- installs base system dependencies when needed
- installs `rtk` and creates the `ccclaude` wrapper
- initializes or attaches the home repository
- creates the fixed `.env`
- creates the fixed `config.toml`
- installs `~/.ccclaw/bin/ccclaw`
- installs `install.sh` and `upgrade.sh`
- installs `systemd --user` unit files
- binds task repositories and writes `[[targets]]`
- prints a deployment summary and next-step commands

## Recommended Install Paths

### Path A: first interactive install

```bash
bash install.sh
```

### Path B: non-interactive install with a fresh home repo

```bash
bash install.sh \
  --yes \
  --home-repo-mode init \
  --task-repo-mode none
```

### Path C: non-interactive install with a remote home repo

```bash
bash install.sh \
  --yes \
  --home-repo-mode remote \
  --home-repo /opt/ccclaw \
  --home-repo-remote owner/private-home-repo
```

### Path D: non-interactive install with a remote task repo

```bash
bash install.sh \
  --yes \
  --home-repo-mode init \
  --task-repo-mode remote \
  --task-repo-remote owner/work-repo \
  --task-repo owner/work-repo \
  --task-repo-path ~/work-repo
```

### Path E: non-interactive install with a local task repo

```bash
bash install.sh \
  --yes \
  --home-repo-mode init \
  --task-repo-mode local \
  --task-repo-local /abs/path/to/repo \
  --task-repo owner/work-repo
```

## Claude Code Readiness

`ccclaw` currently assumes one of two Claude access paths.

### Path 1: official Claude Code login/auth

- Claude Code CLI is installed
- login/auth has already been completed
- the installer will probe the current local `claude` state

If `claude` is missing, the interactive installer can offer the official installer. In non-interactive mode, automatic Claude installation only happens when `--install-claude` is explicitly passed.

### Path 2: proxy API mode

Make sure `.env` contains at least:

```bash
ANTHROPIC_BASE_URL=
ANTHROPIC_AUTH_TOKEN=
```

If you use proxy mode, every "Claude login" step in this README can be treated as "make sure the proxy URL and token are valid".

## First Steps After Install

```bash
~/.ccclaw/bin/ccclaw doctor \
  --config ~/.ccclaw/ops/config/config.toml \
  --env-file ~/.ccclaw/.env
```

Enable background timers if desired:

```bash
systemctl --user daemon-reload
systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer
```

Check task repo bindings:

```bash
~/.ccclaw/bin/ccclaw target list --config ~/.ccclaw/ops/config/config.toml
```

## Common Commands

```bash
ccclaw
ccclaw -h
ccclaw -V
ccclaw doctor
ccclaw config
ccclaw ingest
ccclaw run
ccclaw status
ccclaw target list
ccclaw target add --repo owner/repo --path /abs/path/to/repo
ccclaw target disable --repo owner/repo
```

CLI behavior:

- no args: help
- `-h | --help`: help
- `-V | --version`: version

## Configuration

### Secrets

Stored at:

```text
~/.ccclaw/.env
```

At minimum, verify:

- `GH_TOKEN`
- `ANTHROPIC_BASE_URL`
- `ANTHROPIC_AUTH_TOKEN`

Optional:

- `ANTHROPIC_API_KEY`
- `GREPTILE_API_KEY`

### Normal config

Stored at:

```text
~/.ccclaw/ops/config/config.toml
```

It contains:

- control repo
- path layout
- executor command
- approval gate
- `targets`

## Daily Workflow

1. bind a work repository
2. create or update an Issue in the control repository
3. let `ccclaw` inspect and execute when the gate allows it
4. review results and continue discussion in the Issue

Open-source gate flow:

1. admin-created Issues are eligible for automatic execution
2. external Issues are inspected and discussed by default
3. an admin comment `/ccclaw approve` moves the Issue into execution

## Upgrade and Release

Program-tree upgrade entrypoint:

```bash
~/.ccclaw/upgrade.sh
```

Upgrade rules:

- upgrades refresh the program tree and managed contract files
- user memory under `/opt/ccclaw` is not overwritten
- `kb/**/CLAUDE.md` files are merged with managed blocks

Developer release flow from `src/Makefile`:

```bash
cd src
make fmt
make test
make build
make package
make checksum
make archive
make release
```

Release rules:

- packages are built from `src/dist/`
- at least the install tarball and `SHA256SUMS` must be included
- local archives go to `/ops/logs/ccclaw/`
- `make release` only runs on a clean git worktree

## Build From Source

```bash
cd src
make build
make package
```

Built binary:

```text
src/dist/bin/ccclaw
```

Release tree:

```text
src/dist/
```

## Main Advantages

- natural fit for open-source maintenance because it stays on GitHub Issues
- natural fit for Linux hosts because it defaults to `systemd --user`
- lower token waste because deterministic orchestration is kept local
- durable project memory through fixed knowledge and report locations
- safer upgrades because the program tree and home repo are separated

## Current Boundaries

- default coverage is still `linux/amd64`
- the runtime is still Claude Code-centered; multi-provider support is not phase0 scope
- GitHub Issues remain the main collaboration entrypoint; Matrix-style realtime collaboration is not in the default install path yet

## License

This project is licensed under the [MIT License](LICENSE).
