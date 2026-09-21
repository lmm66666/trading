FROM --platform=$BUILDPLATFORM node:24-alpine AS web-builder

WORKDIR /src/web

COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --prefer-offline --no-audit --no-fund

COPY web/ ./
RUN npm run build


FROM --platform=$BUILDPLATFORM golang:1.25.7-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/trading .


FROM alpine:3.20 AS runtime

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app && adduser -S -G app app

ENV TZ=Asia/Shanghai

WORKDIR /app

COPY --from=builder /out/trading /app/trading

USER app

FROM runtime AS updater
EXPOSE 8081
ENTRYPOINT ["/app/trading", "-service", "updater"]
CMD ["-config", "/app/config.yaml"]

# Private NAS image: configuration is deliberately retained in the final image.
# Build with --no-cache-filter updater-configured when supplying a new secret.
FROM updater AS updater-configured
USER root
RUN --mount=type=secret,id=updater_config,required=true \
    cp /run/secrets/updater_config /app/config.yaml \
    && chown app:app /app/config.yaml \
    && chmod 0400 /app/config.yaml
USER app

FROM runtime AS workbench
COPY --from=web-builder /src/web/dist /app/web/dist
EXPOSE 8080
ENTRYPOINT ["/app/trading", "-service", "workbench"]
CMD ["-config", "/app/config.yaml"]
