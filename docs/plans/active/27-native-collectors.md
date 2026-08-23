# Native collector packaging implementation plan

Status: active
Issue: https://github.com/GhostFlying/tailpath/issues/27
Parent: https://github.com/GhostFlying/tailpath/issues/18
Last updated: 2026-08-24

## Context

GoReleaser already cross-compiles one static `tailpath` binary for Linux,
macOS, and Windows on amd64 and arm64. The archives do not yet contain service
definitions, configuration, or install/uninstall commands. v0.2 requires a
usable Linux native collector, a real-Mac-validated macOS alpha collector, and
an explicitly unverified Windows preview package.

This issue packages collectors only. The supported server delivery remains the
Linux Compose image. It does not add package-manager repositories, MSI/pkg/deb
artifacts, signing, notarization, tags, or GitHub Releases.

## Archive contract

- Every archive contains the platform binary, `README.md`, `LICENSE`, and that
  platform's install/uninstall entrypoints at archive root.
- Linux and macOS remain `tar.gz`; Windows remains `zip`.
- GoReleaser builds `linux/darwin/windows` on `amd64/arm64`. CI creates a
  snapshot and verifies all six archives and their exact platform scripts.
- Installers locate the binary relative to their own extracted directory and
  reject missing/wrong-platform prerequisites before changing host state.
- Configuration contains no auth key. It contains only server URL and optional
  LocalAPI socket override; Tailpath ingest authentication remains Tailnet
  identity plus server WhoIs.

## Linux contract

- `install.sh` requires root, installs `/usr/local/bin/tailpath`, installs
  `/etc/systemd/system/tailpath-collector.service`, and creates
  `/etc/default/tailpath-collector` only when absent.
- The service runs the collector as root because LocalAPI socket group IDs vary
  by host. It uses `Restart=on-failure`, systemd hardening, and no writable
  application state directory.
- `--server-url` and optional `--socket` initialize config on first install.
  Reinstall never overwrites an operator-edited config. The installer reloads,
  enables, and starts the service unless a test-only service skip is set.
- `uninstall.sh` stops/disables the unit and removes the unit and binary. It
  preserves `/etc/default/tailpath-collector`; `--purge` removes it.

## macOS contract

- The installer is user-scoped and rejects root/sudo execution. It installs to
  `~/Library/Application Support/Tailpath`, with `collector.env`, binary, and a
  small runner there; logs use `~/Library/Logs/Tailpath`.
- `~/Library/LaunchAgents/com.tailpath.collector.plist` invokes the runner with
  `RunAtLoad` and `KeepAlive`. The runner reads only the known server/socket
  keys and `exec`s the collector.
- Installation bootstraps the agent into `gui/$UID`; reinstall bootouts the old
  label first. A failed passive `collector --check` produces a warning rather
  than mutating Tailscale or blocking file installation.
- Uninstall removes the LaunchAgent, runner, and binary while retaining config
  and logs. `--purge` removes the remaining Application Support and Logs data.

## Windows preview contract

- `install.ps1` and `uninstall.ps1` require an elevated process. Files live in
  `%ProgramFiles%\Tailpath`; machine config is `collector.env` in that directory.
- A startup Scheduled Task named `Tailpath Collector` runs as `SYSTEM` and
  invokes `run-collector.ps1`. The installer replaces the task atomically and
  starts it after registration.
- The runner accepts only known config keys, merges native stdout/stderr, and
  rotates `collector.log` at 5 MiB with five retained files while the process
  runs. It returns the collector exit code so Task Scheduler records failure.
- Uninstall stops/removes the task and program files while preserving config
  and logs by default; `-Purge` removes the directory. No MSI, signing, or real
  Windows support claim is made in v0.2.

## CI and verification

- Add Linux/macOS/Windows jobs that run `go test ./...` and build the native
  binary on each hosted runner.
- Windows CI parses every PowerShell file with the PowerShell AST parser.
  Ubuntu CI syntax-checks POSIX shell files.
- A Linux snapshot job runs GoReleaser v2, verifies checksums, and validates
  exact archive layout for six OS/architecture artifacts. It uploads the
  snapshot archives for inspection.
- Installer fixture tests use explicit temporary roots/service skips and never
  mutate the CI host's real service manager, `/usr/local`, `/etc`, or user
  LaunchAgents.
- Real arm64 Mac verification is a release gate owned by #28: GUI safesocket,
  `collector --check`, LaunchAgent background start, Direct traffic, server
  outage, and recovery. Until recorded, macOS remains alpha and Windows preview.

## Steps

- [x] Add Linux install/uninstall, default config, and hardened systemd unit.
- [x] Add user LaunchAgent install/uninstall, runner, config, and logs for macOS.
- [x] Add elevated Windows install/uninstall, Scheduled Task runner, config,
  and bounded log rotation.
- [x] Include only the matching platform assets in each GoReleaser archive.
- [x] Add safe installer fixture checks and six-archive layout validation.
- [x] Add hosted OS Go test/build matrix and PowerShell/shell syntax checks.
- [x] Update English/Chinese deployment documentation and support labels.
- [x] Run GoReleaser snapshot, inspect every archive, and complete `make check`.

## Risks

- A static Go binary can compile cross-platform while an installer is invalid
  on its target shell. Hosted runner parsing and archive tests are both needed.
- Reinstall can erase a working server URL. Config creation is create-only;
  upgrades preserve it unless the operator edits it explicitly.
- LaunchAgent domains differ between interactive GUI users and root. The
  installer rejects root and consistently uses `gui/$UID`.
- Scheduled Task stdout is otherwise discarded or unbounded. The Windows
  runner owns line streaming and size-based rotation rather than relying on
  Task Scheduler redirection.
- GoReleaser free supports templated archive file paths but not templated file
  contents. All packaged files are complete source-controlled platform files.

## Verification record

- Linux and macOS isolated-root fixtures cover first install, create-only
  configuration, reinstall, default uninstall, and purge. The macOS fixture
  also covers a user home containing spaces.
- The Windows scripts parse with PowerShell 5.1 syntax. A real runner fixture
  preserves exit code 7 while streaming about 27 MiB and retaining between two
  and five bounded log files.
- GoReleaser v2.17.1 produced Linux, Darwin, and Windows amd64/arm64 snapshot
  archives. The layout gate inspected all six, confirmed platform-only files
  and executable bits, and verified every checksum.
- Hosted Ubuntu, macOS, and Windows jobs passed `go test ./...`, native build,
  and their platform packaging checks. The repository `check` and image jobs
  also passed on the published PR head before the fixture-readiness follow-up.
- Local Go generation, formatting, vet, tests, selected race tests, and build
  pass with Go 1.26.6. Web generation, type/format checks, 44 unit tests, and
  production build pass. Playwright against the real fixture API passes 10
  tests with six intentional project/viewport skips.
- A readiness race discovered by the browser gate was fixed by synchronously
  seeding fixture history before the HTTP server becomes healthy; a Go
  regression test now enforces that contract.

## Current state

All #27 implementation and local acceptance work is complete in Draft PR #37.
Real arm64 Mac Tailnet behavior remains deliberately unclaimed and is a #28
release gate. The next step is the final hosted CI run and review, followed by
the v0.2 performance and dogfood work in #28.
