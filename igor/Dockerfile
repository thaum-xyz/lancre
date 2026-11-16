FROM --platform=$BUILDPLATFORM golang:1.26 AS build
ARG TARGETOS TARGETARCH TARGETVARIANT VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /igor ./cmd/igor

# Runs as root: /dev/i2c-1 and /dev/gpiochip0 are root-owned on the host.
FROM scratch
COPY --from=build /igor /igor
EXPOSE 9101
# Supervisor's watchdog restarts the HAOS app when this turns unhealthy.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD ["/igor", "-healthcheck", "http://127.0.0.1:9101/healthz"]
ENTRYPOINT ["/igor"]
