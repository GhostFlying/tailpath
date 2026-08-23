# Observer protocol

`api/openapi.yaml` owns JSON shapes. This document owns timing and meaning.

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
- `relay_session_update`: traffic-bearing Peer Relay sessions observed between
  two Tailnet endpoints. Zero-delta session lifecycle messages are not activity.

Every envelope has a UUID report ID, reporter instance UUID, monotonic sequence,
and collection timestamp. Normal messages may contain several observer peer
views. A relay update instead contains one or more sessions with explicit relay,
source, and target identities; the two forms cannot be mixed.

## Sparse reporting

The normal local sample interval is fixed at two seconds. The server's idle
heartbeat interval is the single public freshness control and defaults to one
minute; every receipt returns it so collectors adopt the same policy.
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

Relay sessions additionally carry a session ID, VNI, optional source/target
network endpoints, directional counters, and directional deltas. The relay is
the provenance observer and must include its StableNodeID so the path can retain
the canonical relay node, while source and target form the logical edge and may
be reconciled best effort from other strong identities. Relay traffic is
fallback evidence and is never added to duplicate endpoint evidence. Reporter
transport identity is authenticated with WhoIs independently of the trusted
observer identity described in a report.

Collectors keep only the newest unsent state during an outage. Reconnect sends
a fresh hello and baseline; a long outage delta is not presented as current
traffic.

## Inventory generations

An inventory generation is a normalized content hash over the observer and its
current peer identities. Counters, path, online state, and last-seen timestamps
are excluded. Hello and inventory update messages replace that observer's
previous membership for the reporter instance. A removed peer withdraws only
that observer's current provenance; it never deletes the canonical node or
history globally.

Traffic and heartbeat messages reference the current generation. Relay session
updates do not participate in LocalAPI peer inventory generations. A sample with
an unknown generation is still accepted because reporters are trusted, and the
receipt requests a full resync. A heartbeat with an unknown generation also
requests resync.

## Ordering and time

The server accepts the next sequence for a reporter instance, treats an exact
report ID as idempotent, ignores stale sequence values, and requests resync when
it sees a sequence gap.

When a new reporter instance claims a canonical observer, ownership of that
observer's sequence/inventory state moves from the previous reporter. Other
observers carried by the previous reporter remain intact. Normal collectors
reconnect with a fresh hello, so process restarts do not cause reporter-state
growth without bound while persisted inventory remains independent of raw
report retention.

The newest accepted traffic observation by server receive time is the primary
path evidence. Path specificity only breaks exact receive-time ties; conflicting
fresh observations retain provenance. Server receive time owns current-state
ordering, lifecycle expiry, and retention. Collector time remains provenance;
the topology marks observer and observation clock skew.
