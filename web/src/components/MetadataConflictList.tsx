import { TriangleAlert } from "lucide-react";
import type { MetadataConflict } from "../api/types";
import { formatAgo } from "../lib/format";

export function MetadataConflictList({
  conflicts,
}: {
  conflicts: MetadataConflict[];
}) {
  if (!conflicts.length) return null;
  return (
    <section className="metadata-conflicts" aria-label="Metadata conflicts">
      <h3>
        <TriangleAlert size={15} /> Metadata conflicts
      </h3>
      {conflicts.map((conflict) => (
        <article className="metadata-conflict-row" key={conflict.field}>
          <strong>{metadataFieldLabel(conflict.field)}</strong>
          <ConflictValue
            authority="Directory"
            values={conflict.directoryValues}
            collectedAt={conflict.directoryCollectedAt}
          />
          <ConflictValue
            authority="Runtime"
            values={conflict.runtimeValues}
            collectedAt={conflict.runtimeCollectedAt}
          />
        </article>
      ))}
    </section>
  );
}

function ConflictValue({
  authority,
  values,
  collectedAt,
}: {
  authority: string;
  values: string[];
  collectedAt: string;
}) {
  return (
    <div>
      <span>{authority}</span>
      <b>{values.join(", ")}</b>
      <time
        dateTime={collectedAt}
        title={new Date(collectedAt).toLocaleString()}
      >
        {formatAgo(collectedAt)}
      </time>
    </div>
  );
}

function metadataFieldLabel(field: MetadataConflict["field"]): string {
  switch (field) {
    case "dnsName":
      return "MagicDNS";
    case "tailscaleIps":
      return "Tailscale IPs";
    case "os":
      return "Platform";
    default:
      return "Hostname";
  }
}
