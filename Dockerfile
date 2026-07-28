# syntax=docker/dockerfile:1.7
#
# One image: the Go control plane plus the Next.js dashboard it supervises.
# The Go binary is the only listener (:8080); it serves the API in-process and
# reverse-proxies everything else to a node child on loopback. That is what
# keeps a deployment to one image, one port and one domain.

# ---- dashboard build ----
# Pinned to BUILDPLATFORM: the standalone output is JavaScript, so one native
# build serves every target arch and multi-arch stays cheap. Emulating a Next
# build under QEMU costs 15+ minutes.
FROM --platform=$BUILDPLATFORM node:22-alpine AS ui
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
# No NEXT_PUBLIC_API_URL build arg by design: the dashboard calls the API on
# whatever origin serves it, so nothing deployment-specific is inlined here and
# this image works unmodified for every self-hoster.
RUN npm run build \
 && mkdir -p /out \
 && cp -a .next/standalone/. /out/ \
 && mkdir -p /out/.next \
 && cp -a .next/static /out/.next/static \
 && cp -a public /out/public

# ---- control plane build ----
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS api
ARG TARGETARCH
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH \
      go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
# Cross-compile agent binaries; the API serves them at /agent/v1/binary/{os}/{arch}
# so `install.sh` works without any external release hosting. Both arches ship
# in every image, because a control plane manages servers of either.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" \
      -o /out/agents/fleetdock-agent-linux-amd64 ./cmd/agent \
 && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" \
      -o /out/agents/fleetdock-agent-linux-arm64 ./cmd/agent

# ---- runtime ----
# alpine (not distroless): the control plane shells out to mariadb-dump /
# mariadb and pg_dump / psql for backups and restores of *external* instances,
# and hosts the dashboard's node runtime.
# 3.22 rather than 3.20 for nodejs 22 — 3.20 ships node 20.15, which satisfies
# Next 16's >=20.9 only by coincidence. It also moves postgresql-client 16 -> 17,
# which is the safe direction (a newer pg_dump reads older servers).
FROM alpine:3.22
RUN apk add --no-cache ca-certificates mariadb-client postgresql-client su-exec nodejs \
 && addgroup -g 1000 fleetdock \
 && adduser -D -H -u 1000 -G fleetdock app \
 && mkdir -p /var/lib/fleetdock/gateway \
 && chown app:fleetdock /var/lib/fleetdock/gateway \
 && chmod 775 /var/lib/fleetdock/gateway
COPY --from=api /out/api /api
COPY --from=api /out/agents /opt/fleetdock/agents
COPY --from=ui  /out /opt/fleetdock/ui
COPY --chmod=0755 backend/docker-entrypoint.sh /docker-entrypoint.sh
# Presence of this directory is what enables the dashboard; a bare `api` binary
# leaves it unset and behaves exactly as it did before the merge.
ENV FLEETDOCK_UI_DIR=/opt/fleetdock/ui
EXPOSE 8080
ENTRYPOINT ["/docker-entrypoint.sh"]
