import { describe, expect, it } from "vitest";
import { pathIdentityKey } from "./pathIdentity";

describe("pathIdentityKey", () => {
  it("tracks complete Peer Relay identity", () => {
    const relay = {
      kind: "peer_relay" as const,
      peerRelayStableNodeId: "relay-hz",
      peerRelayEndpoint: "203.0.113.8:41641",
      peerRelayVni: 8,
      peerRelayResolution: "endpoint_match" as const,
    };

    expect(pathIdentityKey({ ...relay, peerRelayVni: 4293 })).not.toBe(
      pathIdentityKey(relay),
    );
    expect(
      pathIdentityKey({
        ...relay,
        peerRelayEndpoint: "203.0.113.9:41641",
      }),
    ).not.toBe(pathIdentityKey(relay));
    expect(
      pathIdentityKey({ ...relay, peerRelayResolution: "relay_session" }),
    ).not.toBe(pathIdentityKey(relay));
  });

  it("ignores ephemeral direct endpoint ports", () => {
    expect(
      pathIdentityKey({
        kind: "direct",
        directEndpoint: "203.0.113.8:41641",
      }),
    ).toBe(
      pathIdentityKey({
        kind: "direct",
        directEndpoint: "203.0.113.8:53122",
      }),
    );
  });
});
