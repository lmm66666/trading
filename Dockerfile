FROM --platform=$BUILDPLATFORM golang:1.25.7-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/trading .


FROM --platform=$BUILDPLATFORM  alpine:3.20

RUN apk add --no-cache ca-certificates tzdata bash curl \
    && addgroup -S app && adduser -S -G app app

ENV TZ=Asia/Shanghai

WORKDIR /app

COPY --from=builder /out/trading /app/trading
COPY config-nas.yaml /app/config.yaml

USER app

EXPOSE 8080

ENTRYPOINT ["/app/trading", "-config", "/app/config.yaml"]
