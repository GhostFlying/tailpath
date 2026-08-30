# UI style guide

Tailpath is an operational tool: quiet, dense, predictable, and optimized for
repeated inspection rather than marketing presentation.

- Use CSS custom properties from the canonical web theme; do not add isolated
  raw colors when a semantic token exists.
- Do not rely on color alone. Path kinds require explicit relay topology or a
  matching non-color mini-topology glyph.
- Use Lucide icons for familiar actions and tooltips for unfamiliar icons.
- Keep controls compact, cards at eight-pixel radius or less, and avoid nested
  cards or decorative gradients.
- Preserve stable graph dimensions and node positions during live updates.
- Center a sparse component when it is the first visible topology or replaces
  an empty live graph. Bound automatic sparse-graph zoom so a two-node edge
  uses the workspace without turning device nodes into oversized controls.
- Default to the active/recent subgraph; expose filters rather than rendering a
  topology hairball.
- Path filtering and the `Show recent` option hide nodes that are not endpoints
  of a visible relationship. Search keeps graph structure stable and dims
  non-matches. Active edges are always included; v0.1 has no recent-only mode.
- Keep graph edge labels to one compact total rate; path text is redundant with
  path color and structure. Derive layout edge length from the rendered label
  budget and enforce minimum clearance for node bodies, arrowheads, and labels
  after sparse layout and cache restoration. Use a logarithmic
  line-width scale to distinguish light chatter from bulk traffic; arrowheads
  carry direction, while the inspector shows both directional rates.
- Treat path kind and activity as independent visual dimensions. Activity owns
  line continuity: active is solid with current arrows/rate, while recent is
  dashed without stale arrows/rate. Path uses color plus explicit relay
  topology or an Unknown marker. Present path and activity as separate graph
  legend groups.
- Mark runtime telemetry on the Tailnet/tsnet node whose runtime is being
  observed, not on the process that reports it. One reporter may export several
  runtime views. Label peer-only nodes separately and retain collector clock
  warnings.
- Keep the path-specific color and shape for a resolved canonical Peer Relay,
  while rendering its reported platform icon inside that anatomy. Virtual or
  unresolved relay markers must not invent a platform icon.
- Runtime status reports known views as `N runtimes reporting`, adding
  `N reporting · M stale` only when previously known views expire. Do not use
  an `N of M` ratio without an explicit expected-runtime inventory.
- Edge details use a desktop side panel and mobile bottom sheet.
- Workspace connectivity must name its actual signal. Live reports SSE state;
  History reports required HTTP request state. Shared chrome has no optimistic
  green default.
- History windows with no traffic must render a bounded empty state with the
  selected connection context; never leave an otherwise blank detail pane.
- Shared mobile workspace navigation keeps identical geometry across routes;
  active state and connection status must not resize or shift its tabs.
- Mobile History rows reserve separate identity and traffic-metadata columns;
  recency and directional totals use explicit grid spacing rather than visual
  transforms, while long connection names truncate before metadata moves.
- Devices is a separate catalog workspace. Desktop uses an open table and one
  inspector rather than a grid of cards; mobile uses flat rows and a full-screen
  detail route. Directory control status and Tailpath runtime observation are
  always labeled as independent dimensions. Do not reuse Live active/recent
  language for control-plane connection state.
- Device search and filters must preserve URL state. Long MagicDNS names,
  StableNodeIDs, IPv4, IPv6, and tags must remain selectable and wrap or
  truncate inside stable columns without resizing workspace navigation.
- Use one familiar copy icon for each MagicDNS name and each individual IP.
  StableNodeID remains selectable without a copy affordance. Clipboard actions
  must work on insecure Tailnet HTTP pages through a controlled textarea
  fallback and announce the result through `aria-live`; native long-press text
  selection must remain available.
- Directory/runtime metadata conflicts use the same amber warning in the
  Devices list, device detail, and Live inspector, with both normalized values
  and collection times. Color is never the only conflict signal.
- Validate long hostnames, narrow screens, zoom, touch, empty data, conflicts,
  and relay expansion with Playwright screenshots.
