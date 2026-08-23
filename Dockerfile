# syntax=docker/dockerfile:1.7
FROM node:24-bookworm-slim AS web-build
WORKDIR /src
RUN corepack enable
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY web/package.json web/package.json
RUN pnpm install --frozen-lockfile
COPY web web
RUN pnpm --dir web build

FROM golang:1.26.6-bookworm AS go-build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/tailpath ./cmd/tailpath \
    && test "$(/out/tailpath version)" = "${VERSION}" \
    && mkdir -p /out/state/tsnet

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go-build /out/tailpath /usr/local/bin/tailpath
COPY --from=web-build /src/web/dist /opt/tailpath/web
COPY --from=go-build --chown=nonroot:nonroot /out/state /var/lib/tailpath
VOLUME ["/var/lib/tailpath"]
ENTRYPOINT ["/usr/local/bin/tailpath"]
CMD ["server", "--web-dir", "/opt/tailpath/web"]
