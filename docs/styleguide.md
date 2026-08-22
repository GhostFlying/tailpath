# UI style guide

Tailpath is an operational tool: quiet, dense, predictable, and optimized for
repeated inspection rather than marketing presentation.

- Use CSS custom properties from the canonical web theme; do not add isolated
  raw colors when a semantic token exists.
- Do not rely on color alone. Path kinds require distinct line patterns and
  icons.
- Use Lucide icons for familiar actions and tooltips for unfamiliar icons.
- Keep controls compact, cards at eight-pixel radius or less, and avoid nested
  cards or decorative gradients.
- Preserve stable graph dimensions and node positions during live updates.
- Default to the active/recent subgraph; expose filters rather than rendering a
  topology hairball.
- Edge details use a desktop side panel and mobile bottom sheet.
- Validate long hostnames, narrow screens, zoom, touch, empty data, conflicts,
  and relay expansion with Playwright screenshots.
