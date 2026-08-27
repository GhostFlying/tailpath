# Observer protocol

`api/openapi.yaml` owns JSON shapes. This document owns timing and meaning.

## Capabilities

An embedded v0.4 exporter first reads the authenticated
`GET /api/v1/capabilities` endpoint. It requires observer protocol version 1,
`multi-observer`, and `observer-withdrawal` before sending any observer state.
A missing or malformed capability response is an
incompatible server, not a reason to fall back to weaker lifecycle semantics.
Existing collectors do not require this preflight and remain compatible with a
v0.4 server.

The public `exporter` package owns the handwritten application contracts for
snapshots, reports, receipts, capabilities, and HTTP delivery. Generated
OpenAPI types remain server/client schema artifacts and are not the embedded
application API. Upstream Tailscale types are confined to source adapters.

## Messages

- `observer_hello`: identity, complete current peer-view inventory, counter
  baseline, and inventory generation after startup, reconnect, or requested
  resync.
- `inventory_update`: a complete replacement snapshot after peer membership or
  identity metadata changes.
- `traffic_sample`: only peers with a non-control RX/TX delta in the latest
  local sample interval.
- `observer_heartbeat`: liveness and inventory hash only; never refreshes edge
  activity.
- `observer_withdrawal`: one or more observer identities and inventory hashes,
  without peers. The current owner releases those runtimes immediately.
- `relay_session_update`: traffic-bearing Peer Relay sessions observed between
  two Tailnet endpoints. Zero-delta session lifecycle messages are not activity.

Node identities may include optional reported `os` display metadata. Known
LocalAPI values are normalized to `linux`, `macos`, `windows`, `ios`, or
`android`; unknown values are preserved. OS never contributes to canonical
identity, aliases, or merge decisions, and protocol-v1 reports that omit it
remain valid.

Every envelope has a UUID report ID, reporter instance UUID, monotonic sequence,
and collection timestamp. Normal messages may contain several observer peer
views. A relay update instead contains one or more sessions with an explicit
relay identity and two scoped relay clients; the two forms cannot be mixed.

## Sparse reporting

The normal local sample interval is fixed at two seconds. The server's idle
heartbeat interval is the single public freshness control and defaults to one
minute. It can be configured from ten seconds through ten minutes; every
receipt returns it so collectors adopt the same policy. Edge, observer, and
node freshness are derived from this value and cannot be configured separately.
Current path evidence, recent edges, and observer liveness expire after two
heartbeat intervals. Nodes without observer reports or traffic evidence are
omitted from the live topology after four intervals. Business traffic is active
for ten seconds.

The hello response returns the server's control StableNodeID. A collector must
exclude all control IDs before deciding whether traffic changed. Control
traffic cannot trigger another report.

Traffic samples carry cumulative counters, local deltas, and the local sample
duration. Hello and inventory baselines use a zero duration because they do not
represent an interval; traffic-bearing peer and relay samples require a positive
duration. The server must not derive a real-time rate from time between sparse
HTTP reports.

Relay sessions additionally carry a session ID, unsigned 24-bit VNI,
directional counters, and directional deltas. Each endpoint has a non-empty
`sessionClientId` plus optional full identity, short disco hint, and underlay
endpoint. The two scoped client IDs must differ. Only a full identity is global
canonical evidence. A scoped client ID remains stable for that client during
the relay/VNI session lifetime. The native adapter derives it from the trimmed
short disco hint even when full identity evidence later appears or the underlay
endpoint moves. Without a short hint, an endpoint change conservatively creates
a new client baseline because Tailscale v1.102.2 exposes no other stable
per-client value. If both clients in one session have the same short hint, the
native adapter discards identity enrichment derived from that ambiguous hint
and uses distinct endpoints to keep the two scoped IDs separate. If the
endpoints are also indistinguishable, it omits that session while retaining
other readable sessions. An endpoint change in the collision case establishes
a new baseline. The client ID, short disco hint, and endpoint never become
global aliases. The relay is the provenance observer and must include its
StableNodeID so the path can retain the canonical relay node. Relay traffic is
fallback evidence and is never added to duplicate endpoint evidence. Reporter
transport identity is authenticated with WhoIs independently of the trusted
observer identity described in a report.

Relay client resolution is `resolved`, `partial`, `anonymous`, or `conflict`.
Topology and History may expose that status together with sanitized session ID
and VNI provenance. Underlay endpoints are removed before durable storage and
never appear in API responses or logs. The raw journal stores only whether a
short disco hint was present, not its value, so restart replay preserves
`partial` status without turning the hint into durable identity evidence.

Collectors keep only the newest unsent state during an outage. Reconnect sends
a fresh hello and baseline; a long outage delta is not presented as current
traffic. LocalAPI and report failures use jittered exponential retry from two
seconds through a sixty-second base delay. An accepted complete hello resets
the retry state.

Embedded SnapshotSink sources are sampled concurrently with a fixed
fifteen-second call timeout. The process batches same-kind observer reports for
100 milliseconds, with no more than 64 observers or 1 MiB of encoded JSON per
request, while one loop owns the reporter sequence. Invalid, rejected, timed
out, or oversized source state is isolated from sibling runtimes. A rejected
multi-observer request is split to locate the failing source. The production
timing and size limits are not public configuration knobs.

Peer Relay sampling is capability-detected independently from normal status.
Unsupported, disabled, or transiently failing relay telemetry cannot stop
ordinary peer collection. A new session, the first healthy sample after any
gap or reconnect, a counter reset, and a removed session that later returns all
establish baselines. The collector reports only positive deltas between
consecutive healthy snapshots and keeps no relay catch-up queue. A relay report
transport failure uses the normal reconnect path; the accepted observer hello
also resets every relay baseline.

The optional peer-disco-key lookup is identity enrichment, not session
telemetry capability. A forbidden, unavailable, malformed, or failed lookup
keeps readable sessions and counters as partial or anonymous observations. The
collector logs bounded degraded/recovered enrichment transitions without
resetting the relay counter baseline or reconstructing catch-up traffic.

Tailscale v1.102.2 exposes no explicit session-removal lifecycle event. A
counter reset or changed client key creates a new baseline, but extreme VNI
reuse inside the server freshness window can retain an older scoped pair until
it expires. This is a known upstream-observability limit rather than evidence
for guessing a new identity.

## Inventory generations

An inventory generation is a normalized content hash over the observer and its
current peer identities. Counters, path, online state, and last-seen timestamps
are excluded. Inventory generation and membership belong to the canonical
observer, not to the reporter process carrying the report. Hello and inventory
update messages replace that observer's previous membership. A removed peer
withdraws only that observer's current provenance; it never deletes the
canonical node or history globally.

Traffic and heartbeat messages reference the current generation. Relay session
updates do not participate in LocalAPI peer inventory generations. A sample with
an unknown generation is still accepted because reporters are trusted, and the
receipt requests a full resync. A heartbeat with an unknown generation also
requests resync.

## Ordering and time

The server accepts the next sequence for a reporter instance, treats an exact
report ID as idempotent, ignores stale sequence values, and requests resync when
it sees a sequence gap.

Reporter sequence and report-ID deduplication are scoped to one reporter process
run. A complete hello establishes or transfers the right to update each named
canonical observer. Inventory generation and membership remain on the observer
while only its owner reporter instance changes. Other observers carried by the
previous reporter remain intact. Inventory, traffic, and heartbeat messages from
a non-owner reporter are rejected with a resync request; they cannot refresh
observer liveness or edge evidence. Normal collectors respond with a fresh
hello, so process restarts do not cause reporter-state growth without bound
while persisted observer inventory remains independent of raw report retention.

An observer withdrawal is accepted in the same reporter sequence but changes
state only when that reporter still owns the canonical observer. An unknown
observer, repeated withdrawal, or withdrawal from an owner superseded by a
newer hello is an idempotent no-op. A successful withdrawal makes the observer
offline immediately and excludes its current provenance from Live rates and
path reconciliation. The logical edge remains recent until its normal evidence
deadline, and persisted traffic, path events, inventory, and canonical identity
remain unchanged. A later complete hello claims the observer and establishes a
new counter baseline; it cannot reactivate old traffic.

SnapshotSink retains a pending withdrawal only in process memory and retries
transient transport failure. Identity replacement sends withdrawal for the old
reported identity before hello for the replacement. Cancellation or process
failure cannot guarantee a final message, so server freshness remains the
fallback lifecycle; no durable withdrawal or traffic queue exists.

Peer Relay session updates do not use LocalAPI inventory or hello messages. A
relay update continues to associate its trusted reporter with the relay observer
for the current extension contract; a dedicated relay exporter handshake can
supersede this when relay telemetry is implemented.

The newest accepted traffic observation by server receive time is the primary
path evidence. Path specificity only breaks exact receive-time ties; conflicting
fresh observations retain provenance. Server receive time owns current-state
ordering, lifecycle expiry, and retention. Collector time remains provenance;
the topology marks observer and observation clock skew.
