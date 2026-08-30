# ADR 0008: Keep the optional device directory separate from runtime state

Status: Accepted

## Context

Runtime observers answer which peers are communicating and which Tailscale path
they use, but their peer views are intentionally incomplete. The Tailscale
Devices API can provide a useful control-plane catalog and better display
metadata. It does not observe data-plane traffic, its result is limited to the
devices visible to one credential, and its control connection state is not the
same as Tailpath runtime observability.

Combining these sources without an explicit authority boundary would let a
directory-only device appear online, create an inferred edge, or overwrite a
strong runtime identity with a legacy API identifier.

## Decision

Tailpath may use one separately configured OAuth client with exactly
`devices:core:read`. API keys, write scopes, posture attributes, ACLs, users,
and per-device expansion requests are out of scope. The secret is read from a
file and is never placed in environment variables, SQLite, checkpoints, logs,
or dogfood evidence.

The Devices API is a directory source, not an observer. Observer protocol
version 1 remains unchanged. A successful full response is authoritative only
for the credential-visible current directory:

- API `nodeId` is the directory StableNodeID; legacy numeric `id` is ignored.
- NodeKey may attach a placeholder that has no StableNodeID. It must not merge
  different StableNodeIDs.
- Directory addresses are presentation values, not global runtime IP aliases.
- Directory MagicDNS, hostname, OS, display IPs, tags, control connection state,
  and last-seen time remain in a separate presentation layer.
- Directory presentation wins while present, but runtime evidence remains
  authoritative for edges, paths, traffic, freshness, observability, and
  online state.
- Comparable metadata disagreement is exposed as a conflict with both values
  and collection times instead of being silently resolved.

The server refreshes immediately at startup and every five minutes after a
success. A failure retains the last successful snapshot and marks it stale. A
successful snapshot removes devices that are absent from that response, while
canonical identity, redirects, and History remain durable. Starting with the
feature disabled clears the current directory layer without deleting identity
or History.

Validated normalized directory state is persisted as an optional field in the
existing runtime checkpoint. Applying a directory snapshot follows the same
candidate-state and atomic storage boundary as observer reports: memory and SSE
advance only after the checkpoint and History metadata commit succeeds.

## Consequences

Live remains a passive runtime data-plane view and its counts do not change for
directory-only devices. The separate Devices workspace can show all currently
visible directory devices and clearly separates control connection state from
runtime observation state.

The directory is useful but never a completeness promise. Shared or otherwise
hidden devices may be absent because of upstream visibility behavior. OS
version is not included in v0.5 because it requires the additional
`devices:posture_attributes:read` scope and per-device requests.
