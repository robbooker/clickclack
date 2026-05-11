FROM node:25-alpine AS web
ARG CLICKCLACK_WEB_VERSION=dev
ENV CLICKCLACK_WEB_VERSION=$CLICKCLACK_WEB_VERSION
WORKDIR /src
RUN npm install -g pnpm@11.0.7
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/package.json
COPY packages/protocol/package.json packages/protocol/package.json
COPY packages/sdk-ts/package.json packages/sdk-ts/package.json
RUN pnpm install --frozen-lockfile
COPY apps apps
COPY packages packages
COPY scripts scripts
RUN pnpm build

FROM golang:1.26-alpine AS api
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY apps/api apps/api
COPY infra infra
COPY --from=web /src/apps/api/internal/webassets/dist apps/api/internal/webassets/dist
RUN go build -o /out/clickclack ./apps/api/cmd/clickclack

FROM alpine:3.23
RUN adduser -D -H clickclack
WORKDIR /app
COPY --from=api /out/clickclack /usr/local/bin/clickclack
RUN mkdir -p /app/data && chown -R clickclack:clickclack /app
USER clickclack
EXPOSE 8080
VOLUME ["/app/data"]
ENTRYPOINT ["clickclack"]
CMD ["serve", "--addr", ":8080", "--data", "/app/data"]
