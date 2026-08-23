import { describe, expect, it } from "vitest";
import { createSingleFlight } from "./singleFlight";

describe("createSingleFlight", () => {
  it("runs at most one task and merges a burst into one follow-up", async () => {
    const releases: Array<() => void> = [];
    let calls = 0;
    let active = 0;
    let maximumActive = 0;
    let markFollowUpStarted = () => {};
    const followUpStarted = new Promise<void>((resolve) => {
      markFollowUpStarted = resolve;
    });
    const runner = createSingleFlight(async () => {
      calls += 1;
      if (calls === 2) markFollowUpStarted();
      active += 1;
      maximumActive = Math.max(maximumActive, active);
      await new Promise<void>((resolve) => releases.push(resolve));
      active -= 1;
    });

    const complete = runner.request();
    void runner.request();
    void runner.request();
    expect(calls).toBe(1);
    expect(maximumActive).toBe(1);
    releases.shift()?.();
    await followUpStarted;
    expect(calls).toBe(2);
    releases.shift()?.();
    await complete;
    expect(calls).toBe(2);
    expect(maximumActive).toBe(1);
  });

  it("does not run a pending follow-up after stop", async () => {
    let release = () => {};
    let calls = 0;
    const runner = createSingleFlight(async () => {
      calls += 1;
      await new Promise<void>((resolve) => {
        release = resolve;
      });
    });
    const complete = runner.request();
    void runner.request();
    runner.stop();
    release();
    await complete;
    await runner.request();
    expect(calls).toBe(1);
  });
});
