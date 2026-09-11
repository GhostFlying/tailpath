import { describe, expect, it } from "vitest";
import {
  detailMode,
  SpatialIndex,
  placeLabel,
  centered,
  segmentHitsRect,
  quadratic,
  shorten,
  pointAlong,
} from "./layoutGeometry";

describe("screen-space layout", () => {
  it("keeps detail thresholds stable through small zoom changes", () => {
    expect(detailMode("detail", 0.64)).toBe("overview");
    expect(detailMode("overview", 0.75)).toBe("overview");
    expect(detailMode("detail", 0.75)).toBe("detail");
    expect(detailMode("overview", 0.81)).toBe("detail");
  });
  it("tries another anchor around an unrelated body and preserves it later", () => {
    const index = new SpatialIndex<{
      id: string;
      bounds: ReturnType<typeof centered>;
    }>();
    const body = centered({ x: 100, y: 100 }, 52, 52);
    index.add({ id: "self", bounds: body });
    index.add({ id: "blocker", bounds: centered({ x: 100, y: 155 }, 140, 30) });
    const request = {
      id: "name",
      owner: "self",
      body,
      width: 120,
      height: 22,
      priority: 0,
    };
    const first = placeLabel(request, index, []);
    expect(first?.anchor).toBe(1);
    expect(
      placeLabel({ ...request, previous: first?.anchor }, index, [])?.bounds,
    ).toEqual(first?.bounds);
  });
  it("returns an explicit unresolved label when every candidate is blocked", () => {
    const index = new SpatialIndex<{
      id: string;
      bounds: ReturnType<typeof centered>;
    }>();
    index.add({ id: "wall", bounds: centered({ x: 100, y: 100 }, 1000, 1000) });
    expect(
      placeLabel(
        {
          id: "name",
          owner: "self",
          body: centered({ x: 100, y: 100 }, 52, 52),
          width: 120,
          height: 22,
          priority: 0,
        },
        index,
        [],
      ),
    ).toBeNull();
  });
  it("finds large and negative-coordinate obstacles without duplicate results", () => {
    const index = new SpatialIndex<{
      id: string;
      bounds: ReturnType<typeof centered>;
    }>();
    index.add({ id: "large", bounds: centered({ x: 0, y: 0 }, 10000, 10000) });
    index.add({ id: "small", bounds: centered({ x: -96, y: -96 }, 30, 30) });
    expect(
      index
        .query(centered({ x: -96, y: -96 }, 10, 10))
        .map((x) => x.id)
        .sort(),
    ).toEqual(["large", "small"]);
  });
  it("checks intersections between samples, including tangent and degenerate segments", () => {
    const box = centered({ x: 0, y: 0 }, 10, 10);
    expect(segmentHitsRect({ x: -100, y: 0 }, { x: 100, y: 0 }, box)).toBe(
      true,
    );
    expect(segmentHitsRect({ x: -100, y: 6 }, { x: 100, y: 6 }, box)).toBe(
      false,
    );
    expect(segmentHitsRect({ x: 0, y: 0 }, { x: 0, y: 0 }, box)).toBe(true);
  });
  it("subdivides curved paths and places labels by arc length", () => {
    const p = quadratic({ x: 0, y: 0 }, { x: 100, y: 200 }, { x: 200, y: 0 });
    expect(p.length).toBeGreaterThan(8);
    expect(pointAlong(p, 0.5).x).toBeCloseTo(100);
    expect(pointAlong(p, 0.5).y).toBeCloseTo(100);
  });
  it("keeps meaningful suffixes and Unicode code points within measured budgets", () => {
    const measure = (s: string) => Array.from(s).length * 12;
    expect(shorten("生产环境设备-东京-01", 72, measure)).toBe("生产…-01");
    expect(measure(shorten("😀😀😀😀😀😀😀", 48, measure))).toBeLessThanOrEqual(
      48,
    );
    expect(shorten("smallbox", 160, measure)).toBe("smallbox");
  });
});

it("bounds indexing work for extreme but finite cached positions", () => {
  const index = new SpatialIndex<{
    id: string;
    bounds: ReturnType<typeof centered>;
  }>();
  index.add({ id: "far", bounds: centered({ x: 1e100, y: -1e100 }, 20, 20) });
  index.add({ id: "near", bounds: centered({ x: 0, y: 0 }, 20, 20) });
  expect(
    index.query(centered({ x: 0, y: 0 }, 5, 5)).map((item) => item.id),
  ).toEqual(["near"]);
});
