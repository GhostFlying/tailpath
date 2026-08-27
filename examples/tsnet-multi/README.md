# Multi-instance tsnet exporter

This example runs three independent embedded Tailscale identities and reports
each one as a Tailpath observer through a fourth, dedicated reporting identity.
It does not require system tailscaled.

Use a reusable auth key for first enrollment and a Tailpath server URL reachable
inside the same Tailnet:

```sh
go run ./examples/tsnet-multi \
  --server http://tailpath.example.ts.net:8080 \
  --state-dir ./tsnet-example-state \
  --auth-key file:/run/secrets/tailscale-auth-key
```

An empty auth key is valid only after all four state directories have already
enrolled. First startup is intentionally non-interactive and therefore requires
a reusable key that can create four nodes.

The state directory retains four identities:

```text
tsnet-example-state/
  reporter/
  runtime-a/
  runtime-b/
  runtime-c/
```

The reporter identity authenticates HTTP requests but is not an observer. The
three runtime identities appear independently in Tailpath even though they are
hosted by one process.

To exercise dynamic lifecycle, add `--lifecycle-demo`. After 30 seconds the
example withdraws `runtime-c`; after another 30 seconds it recreates that
runtime from the same persisted state. Adjust the delay with
`--lifecycle-step`.

The example starts and stops its own tsnet servers but does not ping or probe
peers. Generate ordinary application traffic separately when validating paths
and rates. For the isolated Tailpath dogfood only, `--workload-demo` starts an
HTTP listener on runtime B and repeatedly downloads a bounded stream through
runtime A's tsnet HTTP client. Requests explicitly close their connection so a
disposable namespace fault can exercise genuine path restoration. This option
is disabled by default and does not participate in observation or path
classification.

Stop the example with SIGINT or SIGTERM; it attempts accepted withdrawal for
all observers before closing their tsnet identities.
