# ---- build stage ----
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
# Cross-compile agent binaries; the API serves them at /agent/v1/binary/{os}/{arch}
# so `install.sh` works without any external release hosting.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" \
      -o /out/agents/fleetdock-agent-linux-amd64 ./cmd/agent \
 && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" \
      -o /out/agents/fleetdock-agent-linux-arm64 ./cmd/agent

# ---- runtime stage ----
# alpine (not distroless): the control plane shells out to mariadb-dump /
# mariadb and pg_dump / psql for backups and restores of *external* instances.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates mariadb-client postgresql-client su-exec \
 && addgroup -g 1000 fleetdock \
 && adduser -D -H -u 1000 -G fleetdock app \
 && mkdir -p /var/lib/fleetdock/gateway \
 && chown app:fleetdock /var/lib/fleetdock/gateway \
 && chmod 775 /var/lib/fleetdock/gateway
COPY --from=build /out/api /api
COPY --from=build /out/agents /opt/fleetdock/agents
COPY --chmod=0755 docker-entrypoint.sh /docker-entrypoint.sh
EXPOSE 8080
ENTRYPOINT ["/docker-entrypoint.sh"]
