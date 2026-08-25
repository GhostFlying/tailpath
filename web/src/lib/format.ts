import type { PathObservation, TopologyNode } from "../api/types";

export function formatRate(bytesPerSecond: number): string {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond <= 0) return "0 B/s";
  const units = ["B/s", "KB/s", "MB/s", "GB/s"];
  let value = bytesPerSecond;
  let unit = 0;
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000;
    unit += 1;
  }
  const precision = value >= 100 || unit === 0 ? 0 : value >= 10 ? 1 : 2;
  return `${value.toFixed(precision)} ${units[unit]}`;
}

export function formatCompactRate(bytesPerSecond: number): string {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond <= 0) return "0 B/s";
  const units = ["B/s", "KB/s", "MB/s", "GB/s"];
  let value = bytesPerSecond;
  let unit = 0;
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000;
    unit += 1;
  }
  const precision = value >= 10 || unit === 0 ? 0 : 1;
  return `${value.toFixed(precision)} ${units[unit]}`;
}

export function formatBytes(bytes: number): string {
  return formatRate(bytes).replace("/s", "");
}

export function formatAgo(timestamp: string, now = Date.now()): string {
  const seconds = Math.max(
    0,
    Math.floor((now - new Date(timestamp).getTime()) / 1000),
  );
  if (seconds < 5) return "now";
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  return `${Math.floor(minutes / 60)}h ago`;
}

export function runtimeReportingLabel(
  reportingRuntimes: number,
  staleRuntimes: number,
): string {
  if (staleRuntimes > 0) {
    return `${reportingRuntimes} reporting · ${staleRuntimes} stale`;
  }
  const runtimeLabel = reportingRuntimes === 1 ? "runtime" : "runtimes";
  return `${reportingRuntimes} ${runtimeLabel} reporting`;
}

export function pathLabel(path: PathObservation): string {
  switch (path.kind) {
    case "direct":
      return "Direct";
    case "derp":
      return path.derpRegion ? `DERP ${path.derpRegion}` : "DERP";
    case "peer_relay":
      return "Peer Relay";
    default:
      return "Unknown";
  }
}

export function nodeLabel(node: TopologyNode): string {
  const dnsName = node.dnsName?.replace(/\.$/, "");
  return dnsName?.split(".", 1)[0] || node.hostname || node.id;
}
