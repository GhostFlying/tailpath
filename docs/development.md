# Development

The dev container is the canonical environment. Host Go and Node versions are
not supported as a reproducibility contract.

```sh
make bootstrap   # install locked frontend and tool dependencies
make generate    # refresh OpenAPI-derived Go and TypeScript files
make test        # Go and frontend unit tests
make e2e         # Playwright
make check       # generation, lint, test, build, docs, and container checks
```

Use fixtures for normal CI. Real Tailnet verification is a manual milestone
gate and must generate ordinary application traffic rather than product probes.

PR titles and commits use Conventional Commits. Human reviewers squash merge.
See `AGENTS.md` and `CONTRIBUTING.md` for the complete workflow.
