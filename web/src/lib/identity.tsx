import {
  CircleHelp,
  Link2,
  ShieldCheck,
  TriangleAlert,
  type LucideIcon,
} from "lucide-react";
import type { components } from "../api/schema";

export type IdentityStatus = components["schemas"]["IdentityStatus"];

interface IdentityPresentation {
  label: string;
  shortLabel: string;
  Icon: LucideIcon;
  asset: string;
}

const presentations: Record<IdentityStatus, IdentityPresentation> = {
  resolved: {
    label: "Resolved identity",
    shortLabel: "Resolved",
    Icon: ShieldCheck,
    asset: "/identity-resolved.svg",
  },
  partial: {
    label: "Partial identity",
    shortLabel: "Partial",
    Icon: Link2,
    asset: "/identity-partial.svg",
  },
  anonymous: {
    label: "Anonymous relay client",
    shortLabel: "Anonymous",
    Icon: CircleHelp,
    asset: "/identity-anonymous.svg",
  },
  conflict: {
    label: "Conflicting identity evidence",
    shortLabel: "Conflict",
    Icon: TriangleAlert,
    asset: "/identity-conflict.svg",
  },
};

export function identityPresentation(
  status: IdentityStatus | undefined,
): IdentityPresentation | null {
  return status ? presentations[status] : null;
}

export function IdentityBadge({
  status,
  compact = false,
}: {
  status: IdentityStatus | undefined;
  compact?: boolean;
}) {
  const presentation = identityPresentation(status);
  if (!presentation) return null;
  const Icon = presentation.Icon;
  return (
    <span
      className={`identity-badge ${status}`}
      aria-label={presentation.label}
      title={presentation.label}
    >
      <Icon size={13} />
      {compact ? presentation.shortLabel : presentation.label}
    </span>
  );
}

export function unresolvedNodeLabel(
  status: IdentityStatus | undefined,
): string | null {
  switch (status) {
    case "partial":
      return "Unresolved client";
    case "anonymous":
      return "Anonymous client";
    case "conflict":
      return "Identity conflict";
    default:
      return null;
  }
}
