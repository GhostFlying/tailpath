import { mkdir, writeFile } from "node:fs/promises";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import {
  CircleHelp,
  Laptop,
  Monitor,
  Server,
  Smartphone,
  TabletSmartphone,
  TriangleAlert,
  Waypoints,
} from "lucide-react";

const icons = [
  ["device-linux.svg", Server, "#33444b"],
  ["device-macos.svg", Laptop, "#33444b"],
  ["device-windows.svg", Monitor, "#33444b"],
  ["device-ios.svg", Smartphone, "#33444b"],
  ["device-android.svg", TabletSmartphone, "#33444b"],
  ["device-unknown.svg", CircleHelp, "#637078"],
  ["clock-skew.svg", TriangleAlert, "#bd7b00"],
  ["favicon.svg", Waypoints, "#16877a"],
];

const output = new URL("../public/", import.meta.url);
await mkdir(output, { recursive: true });
for (const [filename, Icon, stroke] of icons) {
  const markup = renderToStaticMarkup(
    createElement(Icon, {
      xmlns: "http://www.w3.org/2000/svg",
      size: 24,
      stroke,
      strokeWidth: 1.8,
      "aria-hidden": "true",
    }),
  );
  await writeFile(new URL(filename, output), `${markup}\n`);
}
