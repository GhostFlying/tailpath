# Deployment

[中文版](deployment.zh-CN.md)

The default Compose service runs `tailpath server --network=tsnet`, stores tsnet
state and SQLite under `/var/lib/tailpath`, and joins the Tailnet with
`TS_AUTHKEY` on first start. `TS_AUTHKEY=file:/run/secrets/authkey` reads the
value from a mounted secret file, matching the Tailscale container convention.
The production Compose model mounts the host path in `TAILPATH_AUTHKEY_FILE` as
`/run/secrets/tailscale-authkey`; it never places the credential value in the
model or container environment.

Create the source file in a private directory before first enrollment. The
image runs as a nonroot user, so the file may be world-readable inside a
mode-0700 parent directory:

```sh
install -d -m 0700 secrets
install -m 0600 /dev/null secrets/tailscale-authkey
read -r -s -p 'One-use tsnet auth key: ' tailpath_authkey
printf '\n'
printf '%s\n' "$tailpath_authkey" > secrets/tailscale-authkey
unset tailpath_authkey
chmod 0444 secrets/tailscale-authkey
docker compose up -d server
```

After the server is healthy, clear the value but retain the empty source file
for future Compose recreation:

```sh
chmod u+w secrets/tailscale-authkey
: > secrets/tailscale-authkey
chmod 0444 secrets/tailscale-authkey
```

The single `tailpath-data` volume owns both `tailpath.db` and the `tsnet/`
identity directory. `docker compose down` followed by `docker compose up` must
therefore retain the same enrolled identity. Do not use `down -v` unless the
database and Tailnet identity are intentionally being destroyed together.

## Image channels

Semantic-version tags and `latest` are stable release artifacts. Every fully
successful `main` CI run also publishes an immutable
`edge-<full-commit-sha>` multi-architecture image, then advances the mutable
`edge` tag to the newest successfully published commit in `main` history.
Failed checks and pull requests cannot publish either edge tag. Edge images are
dogfood artifacts: they have no stable, backup, or rollback compatibility
contract and do not include native collector release archives.

Operators may configure a dogfood Compose deployment with `:edge`, but it must
not update itself automatically. Before an explicit update, record the running
image ID and remote edge digest, check whether the range introduces a numbered
database migration, and retain the old image until restart, Live, History, and
collector reconnect checks pass. Production deployments should use a
versioned release tag or digest instead.

Linux collectors can run natively or through the optional `collector`
host-network Compose profile with the tailscaled LocalAPI socket mounted
read-only. The collector reaches the server through its Tailnet hostname or
Tailscale IP; Docker service DNS is not on that path. Set
`TAILPATH_SERVER_URL` when the server is not named `tailpath`. macOS and Windows
collectors run as native binaries. The built-in reporter deliberately bypasses
process HTTP proxy settings so ingest preserves the Tailscale source identity
required by WhoIs.

Native collectors accept `TAILPATH_SERVER_URL` and `TAILPATH_SOCKET`; explicit
`--server` and `--socket` flags take precedence over those environment values.
Peer Relay telemetry defaults to capability-detected `auto`; set
`TAILPATH_RELAY_TELEMETRY=off` or `--relay-telemetry=off` to disable its two
passive debug-status reads. The flag takes precedence over the environment.
Native installer configuration includes the explicit `auto` default and may be
edited to `off` without reinstalling. Run `tailpath collector --check` to read
LocalAPI once and print self identity, runtime platform, peer count, relay
capability, enabled state, and session count as JSON without contacting the
Tailpath server or actively probing any peer. It never prints relay endpoints,
session IDs, scoped client IDs, or disco hints.

## Native collector archives

Each GitHub Release contains Linux and macOS `tar.gz` archives and Windows
`zip` archives for amd64 and arm64. An archive contains only the collector
binary and installer material for its platform, plus the README and license.
Verify the downloaded archive against `checksums.txt` before installation.

On Linux, extract the matching archive and run:

```sh
sudo ./install.sh --server-url http://tailpath.example.ts.net:8080
sudo systemctl status tailpath-collector.service
```

The installer puts the binary in `/usr/local/bin`, creates a hardened systemd
unit, and creates `/etc/default/tailpath-collector` with mode 0600 only when it
does not exist. Use `--socket PATH` for a non-default LocalAPI socket. Reinstall
preserves operator-edited configuration. `sudo ./uninstall.sh` preserves the
configuration; `sudo ./uninstall.sh --purge` removes it.

On macOS, install as the logged-in desktop user, never with `sudo`:

```sh
./install.sh --server-url http://tailpath.example.ts.net:8080
launchctl print "gui/$(id -u)/com.tailpath.collector"
```

The installer uses `~/Library/Application Support/Tailpath`, a user
LaunchAgent, and `~/Library/Logs/Tailpath`. It performs a passive LocalAPI check
and warns, without blocking installation, when the Tailscale GUI safesocket is
not available. `./uninstall.sh` preserves configuration and logs;
`./uninstall.sh --purge` removes them. The macOS package is a preview: CI,
archive-layout, and installer-fixture checks pass, but no supported real-node
contract exists until the qualification tracked in
[#58](https://github.com/GhostFlying/tailpath/issues/58) passes in full.

On Windows, extract the archive and run `install.ps1` from an elevated Windows
PowerShell 5.1 process:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\install.ps1 -ServerUrl http://tailpath.example.ts.net:8080
Get-ScheduledTask -TaskName "Tailpath Collector"
```

The preview installer uses `%ProgramFiles%\Tailpath` and a SYSTEM startup
Scheduled Task. Its runner retains at most five approximately 5 MiB log files.
Reinstall preserves `collector.env`; elevated `.\uninstall.ps1` preserves
configuration and logs, while `.\uninstall.ps1 -Purge` removes them. v0.2 does
not claim real-node Windows support, signing, or MSI delivery.

In tailscaled server mode, an omitted listen host resolves to the first local
Tailscale IP. Explicit wildcard, LAN, and other addresses are rejected unless
`--unsafe-allow-non-tailnet-listen` is supplied; API WhoIs remains mandatory
after that override.

Because socket group IDs vary across Linux hosts, the Compose collector runs as
root solely to open this read-only socket. It has no writable host mount. The
server remains the image's unprivileged `nonroot` user.

The server identity must be dedicated to Tailpath. A tailscaled-mode deployment
that shares an identity must explicitly acknowledge that all traffic to that
node is hidden as system telemetry.

Tailpath v0.2 does not provide a supported backup or restore contract. Copying
only `tailpath.db` is insufficient because the `tsnet/` directory owns the
server identity, and copying a live SQLite file is not documented as safe.
Operators must treat ad hoc volume copies as unsupported. Upgrade one version
at a time; the server runs migrations before readiness.

SQLite uses WAL with `synchronous=NORMAL`. A process or operating-system crash
must leave the database structurally consistent, but sudden power or storage
loss can discard the newest committed observations. Collectors resynchronize
current runtime state after recovery; a resulting history gap is not repaired.
