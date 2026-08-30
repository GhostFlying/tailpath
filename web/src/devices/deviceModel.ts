import type { DirectoryDevice } from "../api/types";

export type DeviceControlFilter = "all" | "connected" | "disconnected";

export function deviceLabel(device: DirectoryDevice): string {
  const dnsName = device.dnsName?.replace(/\.$/, "");
  return dnsName?.split(".", 1)[0] || device.hostname || device.stableNodeId;
}

export function deviceSearchText(device: DirectoryDevice): string {
  return [
    deviceLabel(device),
    device.hostname,
    device.dnsName,
    device.stableNodeId,
    ...device.tailscaleIps,
    ...device.tags,
  ]
    .filter(Boolean)
    .join("\n")
    .toLocaleLowerCase();
}

export function formatDeviceAge(timestamp?: string): string {
  if (!timestamp) return "Never";
  const seconds = Math.max(
    0,
    Math.floor((Date.now() - new Date(timestamp).getTime()) / 1000),
  );
  if (seconds < 5) return "now";
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

export function controlStatus(device: DirectoryDevice) {
  return device.connectedToControl ? "Connected" : "Disconnected";
}

export function runtimeStatus(device: DirectoryDevice) {
  if (!device.runtime) return "Runtime unobserved";
  if (!device.runtime.observable) return "Peer observed";
  return device.runtime.online ? "Runtime online" : "Runtime stale";
}
