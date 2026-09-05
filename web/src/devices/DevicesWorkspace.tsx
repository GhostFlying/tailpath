import {
  ArrowLeft,
  ChevronRight,
  CircleAlert,
  Copy,
  Info,
  Monitor,
  RefreshCw,
  Search,
  TriangleAlert,
  Waypoints,
  X,
} from "lucide-react";
import {
  useDeferredValue,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import type { DirectoryDevice, DirectorySyncState } from "../api/types";
import { MetadataConflictList } from "../components/MetadataConflictList";
import {
  WorkspaceTopbar,
  type WorkspaceConnection,
} from "../components/WorkspaceTopbar";
import { useCapabilities } from "../hooks/useCapabilities";
import { copyText } from "../lib/clipboard";
import { platformPresentation } from "../lib/platform";
import {
  controlStatus,
  deviceLabel,
  deviceSearchText,
  formatDeviceAge,
  runtimeStatus,
  type DeviceControlFilter,
} from "./deviceModel";
import { useDevices, type DeviceConnectionState } from "./useDevices";
import "./devices.css";

export default function DevicesWorkspace() {
  const { nodeId } = useParams<{ nodeId: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const capabilities = useCapabilities();
  const enabled = capabilities.deviceDirectoryEnabled;
  const devices = useDevices(enabled);
  const [copyAnnouncement, setCopyAnnouncement] = useState("");
  const isMobile = useMediaQuery("(max-width: 620px)");
  const query = searchParams.get("q") ?? "";
  const platform = searchParams.get("platform") ?? "all";
  const status = parseControlFilter(searchParams.get("status"));
  const deferredQuery = useDeferredValue(query.trim().toLocaleLowerCase());
  const allDevices = devices.directory?.devices ?? [];
  const platforms = useMemo(
    () =>
      [
        ...new Set(allDevices.map((device) => device.platform).filter(Boolean)),
      ].sort((left, right) => left!.localeCompare(right!)) as string[],
    [allDevices],
  );
  const filteredDevices = useMemo(
    () =>
      allDevices.filter((device) => {
        if (platform !== "all" && device.platform !== platform) return false;
        if (status === "connected" && !device.connectedToControl) return false;
        if (status === "disconnected" && device.connectedToControl)
          return false;
        return (
          !deferredQuery || deviceSearchText(device).includes(deferredQuery)
        );
      }),
    [allDevices, deferredQuery, platform, status],
  );
  const deviceByID = useMemo(
    () => new Map(allDevices.map((device) => [device.id, device])),
    [allDevices],
  );
  const selectedDevice = nodeId ? (deviceByID.get(nodeId) ?? null) : null;
  const search = searchParams.toString();

  useEffect(() => {
    if (
      !enabled ||
      isMobile ||
      nodeId ||
      devices.error ||
      !filteredDevices[0]
    ) {
      return;
    }
    navigate(
      `/devices/${encodeURIComponent(filteredDevices[0].id)}${search ? `?${search}` : ""}`,
      { replace: true },
    );
  }, [
    devices.error,
    enabled,
    filteredDevices,
    isMobile,
    navigate,
    nodeId,
    search,
  ]);

  useEffect(() => {
    if (!nodeId || !selectedDevice || isMobile) return;
    if (!filteredDevices.some((device) => device.id === nodeId)) {
      navigate(`/devices${search ? `?${search}` : ""}`, { replace: true });
    }
  }, [filteredDevices, isMobile, navigate, nodeId, search, selectedDevice]);

  function updateFilter(name: string, value: string) {
    const next = new URLSearchParams(searchParams);
    if (!value || value === "all") next.delete(name);
    else next.set(name, value);
    setSearchParams(next, { replace: true });
  }

  function selectDevice(device: DirectoryDevice) {
    navigate(
      `/devices/${encodeURIComponent(device.id)}${search ? `?${search}` : ""}`,
    );
  }

  function backToList() {
    navigate(`/devices${search ? `?${search}` : ""}`);
  }

  async function copyValue(label: string, value: string) {
    const copied = await copyText(value);
    setCopyAnnouncement("");
    window.setTimeout(
      () =>
        setCopyAnnouncement(
          copied ? `${label} copied` : `Could not copy ${label}`,
        ),
      0,
    );
  }

  const ready =
    !capabilities.loading &&
    (!enabled || devices.directory !== null || devices.error !== null);
  const connection = directoryConnection(
    capabilities,
    devices.connection,
    devices.error,
    devices.directory,
  );

  return (
    <main
      className={`devices-shell ${nodeId ? "has-detail" : ""}`}
      data-devices-ready={String(ready)}
    >
      <WorkspaceTopbar className="devices-app-topbar" connection={connection} />
      {enabled ? (
        <DeviceFilters
          query={query}
          platform={platform}
          status={status}
          platforms={platforms}
          onChange={updateFilter}
        />
      ) : null}
      <div className="devices-workspace">
        {capabilities.loading ? (
          <WorkspaceState loading title="Loading configuration" />
        ) : capabilities.error ? (
          <WorkspaceState
            title="Configuration unavailable"
            detail={capabilities.error}
            action="Retry"
            onAction={capabilities.retry}
          />
        ) : !enabled ? (
          <WorkspaceState
            title="Device directory is not configured"
            detail="Live traffic observability remains available without control-plane enrichment."
          />
        ) : (
          <>
            <section
              className="devices-list-pane"
              aria-label="Device directory"
            >
              <DirectorySummary
                count={filteredDevices.length}
                total={allDevices.length}
                sync={devices.directory?.sync}
              />
              {devices.directory?.sync.status === "stale" ? (
                <div className="devices-stale-banner" role="status">
                  <TriangleAlert size={16} />
                  <span>
                    Directory stale ·{" "}
                    {syncErrorLabel(devices.directory.sync.errorCode)}
                  </span>
                </div>
              ) : null}
              <DeviceList
                devices={filteredDevices}
                selectedID={nodeId}
                loading={!devices.directory && !devices.error}
                error={devices.error}
                onSelect={selectDevice}
                onRetry={() => void devices.refresh()}
              />
            </section>
            <DeviceInspector
              device={selectedDevice}
              unknownID={nodeId && !selectedDevice ? nodeId : null}
              liveVisible={Boolean(
                selectedDevice &&
                  devices.liveVisibleNodeIDs.has(selectedDevice.id),
              )}
              onCopy={copyValue}
              onViewInLive={() => {
                if (selectedDevice) {
                  navigate(`/?nodeId=${encodeURIComponent(selectedDevice.id)}`);
                }
              }}
              onBack={backToList}
              onClose={backToList}
            />
          </>
        )}
      </div>
      <div
        className="copy-announcement sr-only"
        aria-live="polite"
        role="status"
      >
        {copyAnnouncement}
      </div>
    </main>
  );
}

function DeviceFilters({
  query,
  platform,
  status,
  platforms,
  onChange,
}: {
  query: string;
  platform: string;
  status: DeviceControlFilter;
  platforms: string[];
  onChange: (name: string, value: string) => void;
}) {
  return (
    <section className="devices-filters" aria-label="Device filters">
      <label className="devices-search">
        <Search size={18} />
        <span className="sr-only">Find device</span>
        <input
          aria-label="Find device"
          value={query}
          onChange={(event) => onChange("q", event.target.value)}
          placeholder="Find device"
        />
      </label>
      <label className="devices-select">
        <Monitor size={17} aria-hidden="true" />
        <span className="sr-only">Platform</span>
        <select
          aria-label="Platform"
          value={platform}
          onChange={(event) => onChange("platform", event.target.value)}
        >
          <option value="all">All platforms</option>
          {platforms.map((value) => (
            <option key={value} value={value}>
              {platformPresentation(value).label}
            </option>
          ))}
        </select>
      </label>
      <label className="devices-select">
        <span className={`control-dot ${status}`} aria-hidden="true" />
        <span className="sr-only">Control status</span>
        <select
          aria-label="Control status"
          value={status}
          onChange={(event) => onChange("status", event.target.value)}
        >
          <option value="all">All status</option>
          <option value="connected">Connected</option>
          <option value="disconnected">Disconnected</option>
        </select>
      </label>
    </section>
  );
}

function DirectorySummary({
  count,
  total,
  sync,
}: {
  count: number;
  total: number;
  sync: DirectorySyncState | undefined;
}) {
  return (
    <header className="devices-summary">
      <strong>{count}</strong>
      <span>{count === 1 ? "device" : "devices"}</span>
      {count !== total ? <small>of {total}</small> : null}
      <span className="devices-summary-sync">
        <RefreshCw size={14} />
        {sync?.status === "healthy" && sync.lastSuccessAt
          ? `Synced ${formatDeviceAge(sync.lastSuccessAt)}`
          : sync?.status === "stale" && sync.lastSuccessAt
            ? `Last synced ${formatDeviceAge(sync.lastSuccessAt)}`
            : sync?.status === "syncing"
              ? "Syncing"
              : "Waiting for sync"}
      </span>
    </header>
  );
}

function DeviceList({
  devices,
  selectedID,
  loading,
  error,
  onSelect,
  onRetry,
}: {
  devices: DirectoryDevice[];
  selectedID?: string;
  loading: boolean;
  error: string | null;
  onSelect: (device: DirectoryDevice) => void;
  onRetry: () => void;
}) {
  if (loading) {
    return (
      <div className="devices-list-loading" aria-label="Loading devices">
        {Array.from({ length: 8 }, (_, index) => (
          <span key={index} />
        ))}
      </div>
    );
  }
  if (error) {
    return (
      <WorkspaceState
        title="Device directory unavailable"
        detail={error}
        action="Retry"
        onAction={onRetry}
      />
    );
  }
  if (!devices.length) {
    return <WorkspaceState title="No matching devices" />;
  }
  return (
    <div className="devices-table" role="table" aria-label="Devices">
      <div className="devices-table-head" role="row">
        <span role="columnheader">Device</span>
        <span role="columnheader">Platform</span>
        <span role="columnheader">Tailscale IP</span>
        <span role="columnheader">Tags</span>
        <span role="columnheader">Control status</span>
        <span role="columnheader">Last seen</span>
      </div>
      <div className="devices-table-body">
        {devices.map((device) => (
          <DeviceRow
            key={device.id}
            device={device}
            selected={device.id === selectedID}
            onSelect={() => onSelect(device)}
          />
        ))}
      </div>
    </div>
  );
}

function DeviceRow({
  device,
  selected,
  onSelect,
}: {
  device: DirectoryDevice;
  selected: boolean;
  onSelect: () => void;
}) {
  const platform = platformPresentation(device.platform);
  const PlatformIcon = platform.Icon;
  return (
    <button
      className={`device-row ${selected ? "selected" : ""}`}
      role="row"
      aria-label={`Open ${deviceLabel(device)}`}
      onClick={onSelect}
    >
      <span className="device-cell device-identity" role="cell">
        <PlatformIcon size={24} />
        <span>
          <strong>
            {deviceLabel(device)}
            {device.conflicts.length ? (
              <TriangleAlert size={14} aria-label="Metadata conflict" />
            ) : null}
          </strong>
          <small>{device.hostname || device.stableNodeId}</small>
        </span>
      </span>
      <span className="device-cell device-platform" role="cell">
        <PlatformIcon size={17} /> {platform.label}
      </span>
      <span className="device-cell device-ip" role="cell">
        {device.tailscaleIps[0] ?? "—"}
      </span>
      <span className="device-cell device-tags" role="cell">
        {device.tags.slice(0, 2).map((tag) => (
          <small key={tag}>{tag.replace(/^tag:/, "")}</small>
        ))}
      </span>
      <span className="device-cell device-control" role="cell">
        <span
          className={`control-dot ${device.connectedToControl ? "connected" : "disconnected"}`}
        />
        {controlStatus(device)}
        <small>{runtimeStatus(device)}</small>
      </span>
      <span className="device-cell device-last-seen" role="cell">
        {device.connectedToControl
          ? "At sync"
          : formatDeviceAge(device.lastSeen)}
      </span>
      <ChevronRight className="device-chevron" size={20} aria-hidden="true" />
    </button>
  );
}

function DeviceInspector({
  device,
  unknownID,
  liveVisible,
  onCopy,
  onViewInLive,
  onBack,
  onClose,
}: {
  device: DirectoryDevice | null;
  unknownID: string | null;
  liveVisible: boolean;
  onCopy: (label: string, value: string) => Promise<void>;
  onViewInLive: () => void;
  onBack: () => void;
  onClose: () => void;
}) {
  if (!device && !unknownID) return null;
  if (!device) {
    return (
      <aside className="device-inspector" aria-label="Device detail">
        <DeviceMobileHeader onBack={onBack} />
        <WorkspaceState title="Device not found" detail={unknownID ?? ""} />
      </aside>
    );
  }
  const platform = platformPresentation(device.platform);
  const PlatformIcon = platform.Icon;
  return (
    <aside className="device-inspector" aria-label="Device detail">
      <DeviceMobileHeader onBack={onBack} />
      <button
        className="icon-button device-inspector-close"
        onClick={onClose}
        aria-label="Close device detail"
        title="Close device detail"
      >
        <X size={18} />
      </button>
      <header className="device-detail-title">
        <PlatformIcon size={32} />
        <span>
          <h1>{deviceLabel(device)}</h1>
          <ControlState device={device} />
        </span>
      </header>
      {device.conflicts.length ? (
        <div className="device-conflict-summary" role="status">
          <TriangleAlert size={17} />
          <span>{device.conflicts.length} metadata conflicts</span>
        </div>
      ) : null}
      <dl className="device-detail-list">
        <DeviceDetail label="Canonical identity">
          {deviceLabel(device)}
        </DeviceDetail>
        <DeviceDetail label="Stable node ID">
          <span className="selectable-value">{device.stableNodeId}</span>
        </DeviceDetail>
        <DeviceDetail label="MagicDNS / hostname">
          {device.dnsName ? (
            <CopyValue
              value={device.dnsName}
              label="MagicDNS"
              onCopy={onCopy}
            />
          ) : (
            <span>—</span>
          )}
          {device.hostname && device.hostname !== deviceLabel(device) ? (
            <small>{device.hostname}</small>
          ) : null}
        </DeviceDetail>
        <DeviceDetail label="Platform">{platform.label}</DeviceDetail>
        <DeviceDetail label="Tailscale IPs">
          {device.tailscaleIps.length
            ? device.tailscaleIps.map((address) => (
                <CopyValue
                  key={address}
                  value={address}
                  label={`IP ${address}`}
                  code
                  onCopy={onCopy}
                />
              ))
            : "—"}
        </DeviceDetail>
        <DeviceDetail label="Tags">
          <span className="device-detail-tags">
            {device.tags.length
              ? device.tags.map((tag) => (
                  <small key={tag}>{tag.replace(/^tag:/, "")}</small>
                ))
              : "—"}
          </span>
        </DeviceDetail>
        <DeviceDetail label="Control status">
          <ControlState device={device} />
        </DeviceDetail>
        <DeviceDetail label="Last seen">
          {device.connectedToControl
            ? "At sync"
            : formatDeviceAge(device.lastSeen)}
        </DeviceDetail>
        <DeviceDetail label="Runtime observed">
          <span>{runtimeStatus(device)}</span>
          {device.runtime ? (
            <small>{formatDeviceAge(device.runtime.lastEvidenceAt)}</small>
          ) : null}
        </DeviceDetail>
      </dl>
      <MetadataConflictList conflicts={device.conflicts} />
      {liveVisible ? (
        <button
          className="view-live-button"
          type="button"
          onClick={onViewInLive}
        >
          <Waypoints size={16} /> View in Live
        </button>
      ) : null}
      <p className="device-directory-note">
        <Info size={15} />
        Directory presence is not traffic activity.
      </p>
    </aside>
  );
}

function DeviceMobileHeader({ onBack }: { onBack: () => void }) {
  return (
    <header className="device-mobile-header">
      <button onClick={onBack} aria-label="Back to devices">
        <ArrowLeft size={24} />
      </button>
      <strong>Device</strong>
    </header>
  );
}

function DeviceDetail({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}

function CopyValue({
  value,
  label,
  code = false,
  onCopy,
}: {
  value: string;
  label: string;
  code?: boolean;
  onCopy: (label: string, value: string) => Promise<void>;
}) {
  return (
    <span className="copy-value">
      {code ? <code>{value}</code> : <span>{value}</span>}
      <button
        type="button"
        onClick={() => void onCopy(label, value)}
        title={`Copy ${label}`}
        aria-label={`Copy ${label}`}
      >
        <Copy size={14} />
      </button>
    </span>
  );
}

function ControlState({ device }: { device: DirectoryDevice }) {
  return (
    <span
      className={`device-control-state ${device.connectedToControl ? "connected" : "disconnected"}`}
    >
      <span className="control-dot" /> {controlStatus(device)}
    </span>
  );
}

function WorkspaceState({
  loading = false,
  title,
  detail,
  action,
  onAction,
}: {
  loading?: boolean;
  title: string;
  detail?: string;
  action?: string;
  onAction?: () => void;
}) {
  return (
    <div className="devices-workspace-state">
      {loading ? <span className="loading-ring" /> : <CircleAlert size={22} />}
      <strong>{title}</strong>
      {detail ? <span>{detail}</span> : null}
      {action && onAction ? <button onClick={onAction}>{action}</button> : null}
    </div>
  );
}

function directoryConnection(
  capabilities: ReturnType<typeof useCapabilities>,
  connection: DeviceConnectionState,
  error: string | null,
  directory: ReturnType<typeof useDevices>["directory"],
): WorkspaceConnection {
  if (capabilities.error || error || connection === "error") {
    return {
      state: "error",
      label: "Unavailable",
      ariaLabel: "Device directory unavailable",
    };
  }
  if (capabilities.loading) {
    return {
      state: "connecting",
      label: "Connecting",
      ariaLabel: "Loading device directory configuration",
    };
  }
  if (!capabilities.deviceDirectoryEnabled) {
    return {
      state: "reachable",
      label: "Not configured",
      ariaLabel: "Device directory not configured",
    };
  }
  if (directory?.sync.status === "stale" || connection === "reconnecting") {
    return {
      state: "reconnecting",
      label: "Stale",
      ariaLabel: "Device directory data is stale",
    };
  }
  if (!directory || directory.sync.status === "syncing") {
    return {
      state: "connecting",
      label: "Syncing",
      ariaLabel: "Device directory syncing",
    };
  }
  return {
    state: "reachable",
    label: directory.sync.lastSuccessAt
      ? `Synced ${formatDeviceAge(directory.sync.lastSuccessAt)}`
      : "Synced",
    ariaLabel: "Device directory synchronized",
  };
}

function parseControlFilter(value: string | null): DeviceControlFilter {
  return value === "connected" || value === "disconnected" ? value : "all";
}

function syncErrorLabel(value?: string): string {
  switch (value) {
    case "unauthorized":
      return "OAuth rejected";
    case "forbidden":
      return "scope denied";
    case "rate-limited":
      return "rate limited";
    case "timeout":
      return "request timed out";
    case "invalid-response":
      return "invalid response";
    default:
      return "service unavailable";
  }
}

function useMediaQuery(query: string) {
  const [matches, setMatches] = useState(() =>
    typeof window === "undefined" ? false : window.matchMedia(query).matches,
  );
  useEffect(() => {
    const media = window.matchMedia(query);
    const update = () => setMatches(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, [query]);
  return matches;
}
