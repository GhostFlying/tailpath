# Systematic layout rules

[中文版](layout-rules.zh-CN.md) · [UI style guide](styleguide.md)

This is the layout contract for Live, History, Devices, and shared workspace
chrome. It defines target behavior for future UI changes, not a claim that the
current renderer passes every rule. Keep both language versions aligned.

The motivating mobile screenshot shows a DERP label crowding a device and a
tight group of devices and names on the right. These are symptoms of competing
rendered footprints. Checking node centers or increasing every edge's length
does not account for names, badges, relay bodies, and unrelated paths.

## 1. Priorities and occupied space

Apply these priorities in order: truthful topology and identity; usable controls
and readable selected context; separation of bodies and essential labels;
stable positions; optional detail; visual symmetry and compactness.

- A node occupies its body, border, external name, telemetry/identity badges,
  and selection/focus decoration. A relay is a node with its own footprint.
- A label occupies its measured text plus background and padding. Measure with
  the actual font, weight, fallback, and displayed string; character count is
  only a preliminary estimate. Recheck after fonts load or text changes.
- An edge occupies its stroked curve, arrowheads, and rate label. Test the curve,
  not only its endpoint distance. Parallel edges reserve separate lanes.
- Controls, open panels, legend, and safe-area insets reserve screen space.
  Fit operates on the remaining visible workspace, including label bounds.
- Check collisions in rendered CSS pixels after pan/zoom. Convert model-space
  geometry using the current transform; do not confuse device pixels, graph
  model units, and CSS pixels. Hit regions are checked separately from paint.

The following values are project design defaults to implement and validate,
not measurements of the current UI or claims of accessibility conformance.
Use shared named tokens instead of per-element offsets.

| Quantity | Default contract |
| --- | --- |
| Spacing scale | 4, 8, 12, 16, 24, 32 CSS px |
| Unrelated node/relay footprints | At least 12 px between their occupied bounds |
| Unrelated label-to-label or label-to-body clearance | At least 8 px |
| Unrelated edge stroke/arrow to text | At least 4 px, including label padding |
| Graph content to usable viewport edge | At least 16 px after Fit |
| Node name to its own body | At least 8 px; fixed anchor until invalid |
| Ordinary device body | Preserve the existing 52 × 52 model-unit anatomy |
| Graph name / visible rate text | At least 12 / 11 rendered CSS px in readable detail mode |
| Workspace body / secondary text | At least 14 / 12 CSS px; line height at least 1.4 |
| Touch controls | At least 44 × 44 CSS px hit area; hit areas must not overlap |

Apply the larger applicable clearance. A badge may intentionally attach to its
own node and an edge must connect to its own endpoint. Label backgrounds may
mask a short segment of their own edge. These exceptions never permit covering
another object, a direction arrow, or the identity needed to read a connection.

## 2. Text and information hierarchy

- Separate identity, status, and metrics into explicit layout slots. Use grid
  or flex gaps; reserve metric columns before allocating remaining name width.
  Use shrinkable identity tracks (`minmax(0, 1fr)` / `min-width: 0`) and stable
  metric widths. Do not align text with transforms or negative margins.
- Device names use one line by default. Start with a 160 CSS px display budget;
  allow at most two lines only when their full height is reserved. Truncate
  deliberately, retaining a distinguishing suffix when names share prefixes.
  If shortened names remain ambiguous, provide a stable distinguishing label.
- Full identity must be available through keyboard focus and touch selection
  in the inspector or an accessible detail view; hover alone is insufficient.
  Expose the complete name as accessible text for the selection control.
- Keep a numeric value and its unit together. Use tabular digits and reserve
  width for expected rate-format transitions. If a value exceeds that budget,
  remeasure its label without moving canonical nodes on a traffic update.
- A logical connection has at most one compact total-rate label even when a
  relay splits it into segments. Directional rates belong in the inspector.
  Recent paths have no stale rate or arrows. Path and activity remain separate
  visual dimensions, with non-color path identification.
- Do not reduce font size to resolve collisions. At overview zoom, omit optional
  labels explicitly and reveal them on selection or zoom. Keep selected names
  and details readable in a screen-space callout or inspector.

## 3. Nodes, relays, and paths

- Test all nearby node footprints, including unconnected nodes and virtual
  markers. Long names and badges must participate in layout and Fit bounds.
- A DERP pill sizes to its displayed name plus padding, within a documented
  width budget. If it reaches that budget, truncate and expose its full name.
  Never let text spill from a fixed-width pill onto a neighboring device.
- A neighbor centroid is only a relay placement candidate. Search deterministic
  alternatives around it until the full relay footprint clears occupied space;
  route its segments consistently. Do not attach a shared relay to whichever
  connection happened to be processed last.
- Preserve canonical Peer Relay identity and the A → relay → B structure.
  Never merge distinct relays or invent a direct path to simplify geometry.
- Keep paths out of unrelated node bodies and text. Prefer short, consistent
  curves with separated entry angles for parallel or bidirectional traffic.
  Place arrows outside endpoint bodies with room to read their direction.
- Put a rate label on a sufficiently long, clear segment, away from arrows,
  relay bodies, and junctions. Try stable alternative positions along that edge
  before suppressing an optional rate label. Edge crossings are allowed when
  unavoidable; they must not look like a node or a path junction.

## 4. Conflict resolution and density

Use a bounded, deterministic sequence; each step must preserve path semantics:

1. Measure occupied bounds and reserve workspace exclusions.
2. Reuse valid label anchors; try alternative anchors and edge-label positions.
3. Reposition virtual markers and reroute affected paths without moving existing
   canonical nodes. Apply the same collision checks to the new routes.
4. Shorten optional names, suppress unselected rate labels, then suppress
   unselected names if needed. Retain bodies and non-color status/path markers;
   disclose reduced detail and keep complete information available on selection.
5. During initial placement or explicit Relayout, spread canonical nodes using
   complete footprints. During incremental placement, move only new nodes.
6. If the viewport still cannot contain readable detail, use an explicit
   overview with a prompt to zoom/filter or inspect a neighborhood. Fit may show
   reduced detail; it must not present microscopic text as a readable result.

Selected context has first claim on label space; then focused/hovered context,
active identities, recent identities, and unselected rates. Conflict and clock
warnings keep a non-color marker even if their explanatory text moves to details.
Use stable IDs to break ties. Do not hide whole relationships merely to pass a
collision check, or silently aggregate identities. Declare omitted label counts
or a reduced-detail state in an accessible workspace status.

At the project boundary of 250 known nodes and 1,000 visible relationships, an
all-label overview is not required. Bounded layout work, responsive controls,
truthful topology, and an inspectable selected neighborhood are required. Dense
overview crossings are acceptable; unreadable selected context is not.

## 5. Stability and layout triggers

| Trigger | Permitted response |
| --- | --- |
| Rates, activity, SSE reconnect, search or visibility filters | Preserve canonical positions and pan/zoom; revalidate affected labels and routes |
| New canonical node | Place the newcomer; keep established nodes fixed |
| Relay/identity/name/font change | Remeasure and adjust presentation; preserve existing canonical positions |
| Cached position restoration | Validate footprints; use presentation fallbacks without silently rewriting cached positions |
| Resize, orientation, panel, or browser chrome change | Recompute usable area and label visibility; preserve user pan/zoom |
| First visible topology, including after an empty graph | Bounded initial layout and automatic Fit |
| User Fit | Change pan/zoom only; include complete content bounds and screen exclusions |
| User Relayout | Recompute canonical positions, resolve collisions, then Fit and save |
| User drag | Preserve the requested position; revalidate affected presentation and save |

Stability does not excuse overlapping labels. Resolve those through presentation
first; if fixed or user-positioned bodies still collide, expose the limitation
and explicit Relayout. A fallback avoids disruption but does not count as a
collision-free detail layout. Do not silently move a user's node on the next
telemetry tick. Keep valid label placements through small rate changes; use
hysteresis around detail thresholds so labels do not flicker with small zoom
changes. Cap placement attempts and avoid unrestricted layout on each frame.

## 6. Responsive workspace chrome

- Navigation uses the same track sizes across routes; active indicators and
  connection state do not shift tabs. Do not express connectivity only by color.
- Filters wrap by whole control or move secondary options into an accessible
  disclosure. Never squeeze names, values, or touch targets to preserve one row.
- Legend groups retain their headings and symbol/text pairs. Wrap complete
  items; on short screens offer a labeled expandable legend rather than allowing
  it to consume the graph. Keep current path/activity meaning discoverable.
- Derive graph height from available viewport space, with dynamic browser chrome
  and safe areas accounted for. If less than 240 CSS px remains, first collapse
  secondary chrome; if still constrained, allow document scrolling to a usable
  graph region rather than clipping controls or reducing the graph to a strip.
- Desktop inspectors use a side panel; mobile uses a bottom sheet. Reserve their
  occluded area, keep close/actions reachable, and handle the on-screen keyboard.
  Use an explicit focus action if viewing the selection requires camera movement.
- History/Devices rows keep identity separate from status and traffic metadata.
  At narrow widths, move metadata to a reserved second row before permitting
  overlap. Empty/loading/error states keep the same workspace structure.

## 7. Verification contract

The following is the target regression matrix. New cases and browser coverage
must be added as the corresponding rules are implemented; they are not all
present in today's suite.

| Dimension | Required cases |
| --- | --- |
| CSS viewport | 320 × 568, 390 × 844, 430 × 932, 844 × 390, 1440 × 900 |
| Browser | Desktop/mobile Chromium plus WebKit for Safari-sensitive changes; explicitly record real iOS checks or their absence |
| Data | Empty, two nodes, screenshot-like six-device shared DERP cluster, long and duplicate-prefix names, CJK names, multiple relays, partial/anonymous/conflicting identity, clock warnings, 250-node/1,000-edge scale fixture |
| Dynamics | Rate/unit-length change, active/recent switch, relay change, late fonts, reconnect, cold/cached load, search, filters, resize, drag, Fit, Relayout |
| Interaction | Mouse, keyboard, touch, inspector open/close, input keyboard, 200% browser zoom, graph zoom at both detail thresholds |

Use deterministic synthetic fixtures, clearly labeled as synthetic. Reproduce
the screenshot's geometry and long relay label without requiring private device
names or a live Tailnet. Run the viewport/data cases on initial and cached loads;
exercise dynamics on the small relay fixture and the scale fixture. Run the
interaction cases at least at 390 × 844 and 1440 × 900.

Geometry assertions must use post-render screen bounds for bodies, labels,
badges, controls, and stroked paths. Canvas content needs renderer diagnostics;
DOM rectangles around the canvas cannot prove internal non-overlap. Exclude only
the intentional own-object overlaps defined above. Assert clearances, clipping,
separate hit targets, and stable canonical positions/viewport across passive
updates. Verify the declared detail mode and selection recovery when labels are
omitted. A count of rendered nodes or absence of console errors is insufficient.

Inspect and attach desktop/mobile screenshots for default, selected, and dense
states. Record viewport, zoom, fixture, browser, and detail mode. Before marking
a UI PR ready, run canonical dev-container `make check` plus the relevant new
geometry and interaction cases. A screenshot alone is not proof of touch or
keyboard behavior; Chromium mobile emulation is not proof of iOS Safari behavior.

## 8. Implementation status

Issue #179 starts from af9c331, including the merged obstacle-routing fix and
Devices workspace. The original screenshot review used an older coordination
checkout; the implementation preserves the newer behavior.

Stage one adds screen-space geometry, measured text, eight stable name anchors,
content-sized DERP pills, bounded presentation scheduling, detail hysteresis,
object-list selection and complete-footprint separation for movable nodes.
Existing obstacle routing now uses adaptive curve subdivision. Fit reserves
controls/status, freezes virtual positions through at most three correction passes,
and never changes canonical coordinates. Short screens allow scrolling to a
420px graph region. Legend rows occupy their measured height.

Geometry diagnostics are compiled only with VITE_LAYOUT_DIAGNOSTICS=1. The
synthetic layout suite independently checks rendered label/body overlap and
clipping. Chromium and WebKit cover five viewports, cold/cache, Fit, selection,
Relayout, late fonts and changing rates. Real iOS has not been verified. The
8ms scheduler yields between work items; style batches and bounded routing are
atomic, so this is a scheduling budget rather than a hard frame-time guarantee.

Graphs above 80 rendered nodes use overview presentation: retain every object
and connection, suppress optional names/rates, and recover details by selection
or the object list. Dense virtual markers use neighbor centroids and one Fit
correction. Rate/status-only updates preserve settled geometry. Unknown-path
symbols retain their semantic label without device-name disambiguation.

Stage two implements shared 44px touch controls, narrow identity/metadata rows,
whole-control filter wrapping, safe-area padding and visual-viewport keyboard
insets. Independent 320/390px workspace checks and existing route tests pass.
