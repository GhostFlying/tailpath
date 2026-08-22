# Observer protocol

`api/openapi.yaml` owns JSON shapes. This document owns timing and meaning.

## Messages

- `observer_hello`: identity, inventory, counter baseline, and inventory hash
  after startup, reconnect, or requested resync.
- `inventory_update`: changed node identity or inventory metadata.
- `traffic_sample`: only peers with a non-control RX/TX delta in the latest
  local sample interval.
- `observer_heartbeat`: liveness and inventory hash only; never refreshes edge
  activity.
- `relay_session_update`: Peer Relay session lifecycle or traffic changes.

Every envelope has a UUID report ID, reporter instance UUID, monotonic sequence,
and collection timestamp. One envelope may contain several observers.

## Sparse reporting

The normal local sample interval is two seconds. The idle heartbeat interval is
configurable and defaults to five minutes. An observer is offline after two
missed heartbeats plus thirty seconds.

The hello response returns the server's control StableNodeID. A collector must
exclude all control IDs before deciding whether traffic changed. Control
traffic cannot trigger another report.

Traffic samples carry cumulative counters, local deltas, and the local sample
duration. The server must not derive a real-time rate from time between sparse
HTTP reports.

Collectors keep only the newest unsent state during an outage. Reconnect sends
a fresh hello and baseline; a long outage delta is not presented as current
traffic.

## Ordering

The server accepts the next sequence for a reporter instance, treats an exact
report ID as idempotent, ignores stale sequence values, and requests resync when
it lacks the inventory generation referenced by a sample.
