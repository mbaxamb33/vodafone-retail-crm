# Builds the frontend and Go binaries on the build machine's native platform, then
# assembles a small runtime image for the target platform.
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src
COPY package.json package-lock.json .npmrc ./
COPY web/package.json web/
RUN npm ci
COPY web web
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS go
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/crmctl ./cmd/crmctl

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -u 10001 app
COPY --from=go /out/ /usr/local/bin/
COPY --from=web /src/web/dist /app/web
USER app
ENV STATIC_DIR=/app/web LISTEN_ADDR=0.0.0.0:8080 APP_ENV=production
EXPOSE 8080
CMD ["server"]
