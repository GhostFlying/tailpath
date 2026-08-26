# Production-asset browser gates

Issue: [#103](https://github.com/GhostFlying/tailpath/issues/103)

## Goal

Make browser gates deterministic and representative by testing the built web
assets and fixture API from the same HTTP server instead of routing production
API calls through Vite's development proxy.

## Decisions

- The fixture server owns the Playwright base URL and serves both `web/dist`
  and `/api`; Vite is not part of browser acceptance.
- `scripts/e2e.sh` builds the web bundle before starting the fixture so a
  direct `make test-e2e` cannot exercise stale assets.
- Ordinary browser tests retain four CI workers. Scale and relay-scale modes
  retain one worker and their existing deterministic fixtures.
- Browser traces and screenshots remain Playwright artifacts. No production
  runtime or observer protocol behavior changes.

## Implementation

1. Make the Playwright base URL configurable with the fixture listener as its
   scripted default.
2. Remove Vite process management from the E2E harness and wait for the
   fixture-served UI before launching browsers.
3. Document the production-asset boundary and add a shell regression check for
   the harness contract.

## Verification

- Pending: shell harness regression test.
- Pending: repeated four-worker browser suite.
- Pending: `make check` and hosted CI.

## Current state

Local and hosted `make check` reproduced stalled History readiness while Vite
proxied concurrent StrictMode request replacement. Direct fixture API queries
and single-worker browser execution remained healthy.

## Next step

Replace the development proxy path and exercise the full browser matrix.
