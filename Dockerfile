# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o /app .

# Runtime stage
FROM alpine:latest

ENV VMUTILS_VERSION=1.112.0

RUN apk add --no-cache ca-certificates wget tar \
    && wget -O /tmp/vmutils.tar.gz https://github.com/VictoriaMetrics/VictoriaMetrics/releases/download/v${VMUTILS_VERSION}/vmutils-linux-amd64-v${VMUTILS_VERSION}.tar.gz -q \
    && tar -xzf /tmp/vmutils.tar.gz -C /tmp \
    && mv /tmp/vmbackup-prod /vmbackup \
    && rm -rf /tmp/*

COPY --from=builder /app /app

EXPOSE 8000

ENTRYPOINT ["/app"]
