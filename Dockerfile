# 本地源码构建使用前端与后端阶段；CI 镜像直接复用已打包的二进制。
FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build
WORKDIR /src
COPY scripts/ ./scripts/
RUN node scripts/prepare-webui.mjs

FROM --platform=$BUILDPLATFORM golang:1.26.10-alpine AS backend
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=frontend /src/internal/webui/dist ./internal/webui/dist
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN case "${TARGETARCH}/${TARGETVARIANT}" in \
      arm/v5) export GOARM=5 ;; \
      arm/v6) export GOARM=6 ;; \
      arm/*) echo "不支持的 ARM 变体" >&2; exit 1 ;; \
    esac \
    && GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags=with_frontend -trimpath -ldflags="-s -w" -o /out/gatesync ./cmd/gatesync

# 从构建主机获取证书和时区文件，运行层无需目标架构基础镜像或模拟器。
FROM --platform=$BUILDPLATFORM alpine:3.23 AS runtime-files
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 65532 gatesync && adduser -D -H -u 65532 -G gatesync gatesync \
    && mkdir /data /runtime-tmp && chown 65532:65532 /data && chmod 1777 /runtime-tmp

FROM scratch AS runtime
COPY --from=runtime-files /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=runtime-files /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=runtime-files /etc/passwd /etc/passwd
COPY --from=runtime-files /etc/group /etc/group
COPY --from=runtime-files --chown=65532:65532 /data /data
COPY --from=runtime-files --chmod=1777 /runtime-tmp /tmp
ENV GATESYNC_ADDRESS=0.0.0.0:13850 GATESYNC_DATA_DIR=/data
USER 65532:65532
WORKDIR /data
EXPOSE 13850
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD ["/gatesync", "healthcheck"]
ENTRYPOINT ["/gatesync"]

FROM --platform=$BUILDPLATFORM alpine:3.23 AS release-binary
ARG TARGETARCH
ARG TARGETVARIANT
COPY .release/binaries/ /binaries/
RUN platform="${TARGETARCH}" \
    && if [ "$TARGETARCH" = arm ]; then platform="arm${TARGETVARIANT}"; fi \
    && cp "/binaries/gatesync-linux-${platform}/gatesync" /gatesync && chmod 0755 /gatesync

FROM runtime AS release
COPY --from=release-binary /gatesync /gatesync

# 默认目标保留 docker compose --build 的源码构建方式。
FROM runtime AS standalone
COPY --from=backend /out/gatesync /gatesync
