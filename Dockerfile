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


FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app && adduser -S -G app app

ENV TZ=Asia/Shanghai

WORKDIR /app

COPY --from=builder /out/trading /app/trading
COPY --from=web-builder /src/web/dist /app/web/dist

USER app

EXPOSE 8080

ENTRYPOINT ["/app/trading"]
CMD ["-config", "/app/config.yaml"]
