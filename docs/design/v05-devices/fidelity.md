# v0.5 Devices workspace fidelity ledger

Issue: [#149](https://github.com/GhostFlying/tailpath/issues/149)

## References

- `desktop.png`: accepted desktop table and inspector.
- `mobile-list.png`: accepted mobile filters and directory rows.
- `mobile-detail.png`: accepted full-screen mobile detail.

All names, addresses, tags, and identifiers in the references and browser
fixtures are synthetic.

## Implemented faithfully

- Quiet operational table with a fixed right-side inspector on desktop.
- Search above platform and control-status filters on mobile.
- Platform icon, directory control status, runtime observation, IP, and tags
  remain separately readable in each row.
- Mobile selection replaces the list with a full-screen detail and uses browser
  history for return navigation.
- Directory presence is explicitly separated from traffic activity.
- Teal indicates control-connected; neutral gray indicates disconnected or
  unobserved. Amber is reserved for metadata conflicts and stale sync state.

## Intentional differences

- The full directory is returned and rendered as a scrollable list; the
  accepted desktop pagination footer is omitted because v0.5 explicitly has no
  server pagination and 250 rows meet the browser gate with
  `content-visibility`.
- Sync status and count share a dedicated summary row on desktop as well as
  mobile. This keeps stale/error status stable when filters change.
- Runtime status is shown beneath control status instead of replacing the
  directory last-seen value. The two signals have different authorities.

## Browser evidence

Playwright Chromium is the documented fallback because the Browser/IAB tool is
unavailable. The suite captures:

- `devices-desktop.png` at 1440x900.
- `devices-mobile-list.png` and `devices-mobile-detail.png` at 390x844.
- `devices-mobile-320.png` at 320x700.

The screenshots were inspected with `view_image` against all three accepted
references. No horizontal overflow, overlapping controls, tab-size changes, or
visible assistive-only labels remain.
