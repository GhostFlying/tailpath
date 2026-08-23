import {
  CircleHelp,
  Laptop,
  Monitor,
  Server,
  Smartphone,
  TabletSmartphone,
  type LucideIcon,
} from "lucide-react";

interface PlatformPresentation {
  label: string;
  asset: string;
  Icon: LucideIcon;
}

const knownPlatforms: Record<string, PlatformPresentation> = {
  linux: { label: "Linux", asset: "/device-linux.svg", Icon: Server },
  macos: { label: "macOS", asset: "/device-macos.svg", Icon: Laptop },
  windows: { label: "Windows", asset: "/device-windows.svg", Icon: Monitor },
  ios: { label: "iOS", asset: "/device-ios.svg", Icon: Smartphone },
  android: {
    label: "Android",
    asset: "/device-android.svg",
    Icon: TabletSmartphone,
  },
};

const unknownPlatform: PlatformPresentation = {
  label: "Unknown platform",
  asset: "/device-unknown.svg",
  Icon: CircleHelp,
};

export function platformPresentation(os?: string): PlatformPresentation {
  if (!os) return unknownPlatform;
  const known = knownPlatforms[os.toLowerCase()];
  return known ?? { ...unknownPlatform, label: os };
}
