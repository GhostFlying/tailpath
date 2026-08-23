export const layoutCacheKey = "tailpath.graph-layout.v1";
export const layoutCacheVersion = 1;
export const layoutCacheMaxEntries = 2_000;
export const layoutCacheMaxBytes = 512 * 1024;
export const layoutCacheMaxAgeMs = 30 * 24 * 60 * 60 * 1000;

export interface LayoutPosition {
  x: number;
  y: number;
  lastSeen: number;
}

interface StoredNode extends LayoutPosition {
  id: string;
}

interface StoredLayout {
  version: number;
  nodes: StoredNode[];
}

export function readLayoutCache(
  storage: Storage | undefined,
  now = Date.now(),
): Map<string, LayoutPosition> {
  if (!storage) return new Map();
  try {
    const raw = storage.getItem(layoutCacheKey);
    if (!raw || raw.length > layoutCacheMaxBytes) return new Map();
    const parsed: unknown = JSON.parse(raw);
    if (!isStoredLayout(parsed)) return new Map();
    if (parsed.nodes.length > layoutCacheMaxEntries) return new Map();
    const cutoff = now - layoutCacheMaxAgeMs;
    const result = new Map<string, LayoutPosition>();
    for (const node of parsed.nodes) {
      if (!validStoredNode(node) || node.lastSeen < cutoff) continue;
      result.set(node.id, {
        x: node.x,
        y: node.y,
        lastSeen: node.lastSeen,
      });
    }
    return result;
  } catch {
    return new Map();
  }
}

export function writeLayoutCache(
  storage: Storage | undefined,
  positions: ReadonlyMap<string, LayoutPosition>,
  seenNodeIDs: Iterable<string>,
  now = Date.now(),
): void {
  if (!storage) return;
  try {
    const merged = readLayoutCache(storage, now);
    for (const [id, position] of positions) {
      if (validStoredNode({ id, ...position })) merged.set(id, position);
    }
    for (const id of seenNodeIDs) {
      const position = merged.get(id);
      if (position) merged.set(id, { ...position, lastSeen: now });
    }
    const cutoff = now - layoutCacheMaxAgeMs;
    const nodes = [...merged]
      .map(([id, position]) => ({ id, ...position }))
      .filter((node) => validStoredNode(node) && node.lastSeen >= cutoff)
      .sort((left, right) => right.lastSeen - left.lastSeen)
      .slice(0, layoutCacheMaxEntries);
    const payload: StoredLayout = { version: layoutCacheVersion, nodes };
    const encoded = JSON.stringify(payload);
    if (encoded.length <= layoutCacheMaxBytes) {
      storage.setItem(layoutCacheKey, encoded);
    }
  } catch {
    // Layout persistence is optional when storage is blocked or full.
  }
}

export function clearLayoutCache(storage: Storage | undefined): void {
  if (!storage) return;
  try {
    storage.removeItem(layoutCacheKey);
  } catch {
    // Relayout still works when storage cannot be cleared.
  }
}

function isStoredLayout(value: unknown): value is StoredLayout {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Partial<StoredLayout>;
  return (
    candidate.version === layoutCacheVersion && Array.isArray(candidate.nodes)
  );
}

function validStoredNode(node: StoredNode): boolean {
  return (
    typeof node.id === "string" &&
    node.id.length > 0 &&
    node.id.length <= 512 &&
    finiteCoordinate(node.x) &&
    finiteCoordinate(node.y) &&
    Number.isFinite(node.lastSeen) &&
    node.lastSeen >= 0
  );
}

function finiteCoordinate(value: number): boolean {
  return Number.isFinite(value) && Math.abs(value) <= 10_000_000;
}
