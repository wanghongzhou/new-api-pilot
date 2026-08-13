ARG BUN_IMAGE=docker.m.daocloud.io/oven/bun:1.3.13-alpine
ARG GO_IMAGE=docker.m.daocloud.io/library/golang:1.25-alpine
ARG RUNTIME_IMAGE=docker.m.daocloud.io/library/alpine:3.22

FROM ${BUN_IMAGE} AS web-builder

WORKDIR /build/web

COPY web/package.json web/bun.lock web/.npmrc ./
RUN bun install --frozen-lockfile

COPY web/ ./
RUN bun run check \
    && bun run build \
    && test -s dist/index.html

FROM ${GO_IMAGE} AS go-deps

ARG GO_MODULE_PROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct
ARG GO_SUM_DATABASE=sum.golang.google.cn
ENV GO111MODULE=on CGO_ENABLED=0
ENV GOPROXY=${GO_MODULE_PROXY}
ENV GOSUMDB=${GO_SUM_DATABASE}
WORKDIR /build

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOMODCACHE=/root/.cache/go-mod go mod download \
    && mkdir -p /go/pkg/mod \
    && cp -a /root/.cache/go-mod/. /go/pkg/mod/

FROM go-deps AS go-test-runner

COPY . .
COPY --from=web-builder /build/web/dist ./webui/dist

FROM go-test-runner AS go-builder

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    test -s webui/dist/index.html \
    && go build -tags=embed_web -trimpath -ldflags="-s -w" -o /out/new-api-pilot .

FROM ${RUNTIME_IMAGE}

ARG ALPINE_MIRROR=https://mirrors.aliyun.com/alpine
RUN if [ -n "${ALPINE_MIRROR}" ]; then \
        sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${ALPINE_MIRROR}|g" /etc/apk/repositories; \
    fi \
    && apk add --no-cache ca-certificates tzdata \
    && mkdir -p /data/exports

COPY --from=go-builder --chown=0:0 --chmod=0555 /out/new-api-pilot /usr/local/bin/new-api-pilot

ENV PORT=3000 \
    TZ=Asia/Shanghai \
    EXPORT_DIR=/data/exports

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 \
    CMD wget -q -Y off -T 2 --spider "http://127.0.0.1:${PORT}/healthz" || exit 1

STOPSIGNAL SIGTERM

ENTRYPOINT ["/usr/local/bin/new-api-pilot"]
