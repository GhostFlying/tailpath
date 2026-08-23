FROM node:24-bookworm-slim AS web-build
WORKDIR /src
RUN corepack enable
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY web/package.json web/package.json
RUN --mount=type=cache,id=tailpath-pnpm,target=/pnpm/store \
    pnpm config set store-dir /pnpm/store \
    && pnpm install --frozen-lockfile
COPY web web
RUN pnpm --dir web build

FROM golang:1.26.6-bookworm AS go-build
ARG VERSION=dev
ARG GOPROXY=https://proxy.golang.org,direct
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,id=tailpath-go-mod,target=/go/pkg/mod \
    GOPROXY=${GOPROXY} go mod download
COPY . .
RUN --mount=type=cache,id=tailpath-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=tailpath-go-build,target=/root/.cache/go-build \
    GOPROXY=${GOPROXY} CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/tailpath ./cmd/tailpath \
    && test "$(/out/tailpath version)" = "${VERSION}" \
    && mkdir -p /out/state/tsnet

FROM go-build AS perf-build
RUN --mount=type=cache,id=tailpath-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=tailpath-go-build,target=/root/.cache/go-build \
    GOPROXY=${GOPROXY} CGO_ENABLED=0 go build -trimpath -o /out/tailpath-load ./cmd/tailpath-load

FROM gcr.io/distroless/static-debian12:nonroot AS perf-client
COPY --from=perf-build /out/tailpath-load /usr/local/bin/tailpath-load
ENTRYPOINT ["/usr/local/bin/tailpath-load"]

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=go-build /out/tailpath /usr/local/bin/tailpath
COPY --from=web-build /src/web/dist /opt/tailpath/web
COPY --from=go-build --chown=nonroot:nonroot /out/state /var/lib/tailpath
VOLUME ["/var/lib/tailpath"]
ENTRYPOINT ["/usr/local/bin/tailpath"]
CMD ["server", "--web-dir", "/opt/tailpath/web"]
