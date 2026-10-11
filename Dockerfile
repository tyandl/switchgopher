FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# modernc.org/sqlite is pure Go, so no cgo toolchain is needed.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/switchgopher ./cmd/switchgopher

FROM alpine:3.21
RUN adduser -D -u 10001 switchgopher \
 && mkdir -p /data /app/resources/configs/dated \
 && chown -R switchgopher /data /app
WORKDIR /app
COPY --from=build /out/switchgopher ./switchgopher
COPY resources/ resources/
COPY configuration/ configuration/
RUN chown -R switchgopher /app
USER switchgopher

# The app uses paths relative to /app (templates, MIBs, configuration/,
# resources/configs/), so those stay put; only the database is relocated.
VOLUME ["/data"]
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
  CMD wget -q -O /dev/null http://127.0.0.1:8000/login/ || exit 1
ENTRYPOINT ["/app/switchgopher", "-db", "/data/inventory.sqlite"]
