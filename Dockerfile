# Build stage
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags "-s -w" -o /app .

# Runtime stage
FROM alpine:3.21

# vmutils（vmbackup/vmrestore）のバージョン。--build-arg で上書き可能。
ARG VMUTILS_VERSION=1.112.0
# build-push-action が自動で渡すターゲットアーキ（amd64 / arm64）。vmutils の配布名と一致する。
ARG TARGETARCH

RUN apk add --no-cache ca-certificates wget tar \
    && wget -O /tmp/vmutils.tar.gz https://github.com/VictoriaMetrics/VictoriaMetrics/releases/download/v${VMUTILS_VERSION}/vmutils-linux-${TARGETARCH}-v${VMUTILS_VERSION}.tar.gz -q \
    && tar -xzf /tmp/vmutils.tar.gz -C /tmp \
    && mv /tmp/vmbackup-prod /vmbackup \
    && rm -rf /tmp/*

COPY --from=builder /app /app

EXPOSE 8000

ENTRYPOINT ["/app"]
