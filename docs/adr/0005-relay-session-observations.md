# ADR 0005: Relay session observations

Status: accepted
Date: 2026-08-23

## Context

A Peer Relay observes a session between two Tailnet endpoints. Modeling that
session as an ordinary peer observation would create relay-to-endpoint edges or
falsely attribute the observation to one endpoint. The v0.1 observer contract
must remain extensible even though the Peer Relay exporter ships in v0.3.

## Decision

`relay_session_update` envelopes contain `relaySessions` instead of ordinary
observer peer views. Every traffic-bearing session names the relay observer,
source endpoint, target endpoint, session ID, VNI, optional network endpoints,
cumulative directional counters, directional deltas, sample duration, and
last-active time. The relay observer must include StableNodeID; endpoint
identities may use the normal best-effort alias reconciliation.

The aggregator creates one logical source-target edge with a Peer Relay path.
The relay canonical node is the observation provenance. Relay rates and traffic
history are fallback evidence when endpoint observations are unavailable; they
are never added to duplicate endpoint evidence.

Reporter transport identity still comes from Tailnet WhoIs. Because reporters
are operator-controlled and trusted, the relay identity in the session is an
explicit observer claim and may differ from the transport identity, matching
the existing multi-tsnet reporter model.

## Consequence

The API, aggregator, and storage can preserve three-party observations before a
relay-specific collector exists. v0.3 adds exporter integration, disco/VNI
correlation, and richer session presentation without changing the envelope or
logical edge model. Zero-delta relay session lifecycle messages are rejected so
reporting alone cannot create or refresh an active traffic edge.
