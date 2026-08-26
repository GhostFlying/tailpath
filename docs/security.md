# Security and trust

[中文版](security.zh-CN.md)

Tailpath relies on the Tailnet for network reachability and connection identity.
Every `/api/v1/*` request, including topology, history, SSE, and report ingest,
requires a successful Tailscale WhoIs lookup. The server applies no permissive
CORS. Reporters are trusted components controlled by the operator.

Tailscaled mode binds only a local Tailscale IP by default. Binding a wildcard,
LAN, or other non-Tailscale address requires the explicit
`--unsafe-allow-non-tailnet-listen` flag. That flag changes only the listener;
it never disables API WhoIs.

The Compose collector runs as root only to handle host-specific LocalAPI socket
group IDs. Its socket mount is read-only and operators must not add writable
host mounts to that service. The server runs unprivileged.

The server uses a dedicated Tailnet identity by default. Sharing a tailscaled
identity with another service is an explicit degraded mode because aggregate
peer counters cannot separate Tailpath traffic from business traffic.

Tailpath never logs auth keys or complete report bodies. Public endpoints from
ordinary direct-path observations, node identifiers, and traffic history remain
in the local SQLite database. Peer Relay underlay endpoints are more narrowly
scoped: they may be used during current report processing but are stripped
before the report journal, checkpoint, path events, History, API output, and
logs. Short disco hints are replaced by a constant presence marker in the raw
report journal, preserving `partial` replay semantics without retaining the
value; they are omitted from checkpoints and History. There is no outbound
product telemetry.

Container deployments should pass reusable auth keys through
`TS_AUTHKEY=file:/run/secrets/...`, then zero the secret after enrollment. The
environment and Compose model contain only the file path, not the credential.

Runtime code cannot actively probe peers, capture packets, alter policy, or
change Tailscale preferences. Devices API enrichment is optional, read-only,
and uses a separately scoped credential.
