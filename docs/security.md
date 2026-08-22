# Security and trust

[中文版](security.zh-CN.md)

Tailpath relies on the Tailnet for network reachability and connection identity.
The server uses WhoIs for reporter provenance and applies no permissive CORS.
Reporters are trusted components controlled by the operator.

The server uses a dedicated Tailnet identity by default. Sharing a tailscaled
identity with another service is an explicit degraded mode because aggregate
peer counters cannot separate Tailpath traffic from business traffic.

Tailpath never logs auth keys or complete report bodies. Public endpoints,
node identifiers, and traffic history remain in the local SQLite database.
There is no outbound product telemetry.

Runtime code cannot actively probe peers, capture packets, alter policy, or
change Tailscale preferences. Devices API enrichment is optional, read-only,
and uses a separately scoped credential.
