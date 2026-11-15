FROM golang:1.23-alpine AS build
WORKDIR /src
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN set -eu; \
    mkdir -p /out; \
    LDFLAGS="-X github.com/vod2live/cyonlive_vod2live/internal/buildinfo.Version=${VERSION} -X github.com/vod2live/cyonlive_vod2live/internal/buildinfo.Commit=${COMMIT} -X github.com/vod2live/cyonlive_vod2live/internal/buildinfo.BuildTime=${BUILD_TIME}"; \
    for service in api playout media-worker; do \
      CGO_ENABLED=0 GOOS=linux go build -ldflags "$LDFLAGS" -o "/out/service-${service}" "./cmd/${service}"; \
    done

FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget ffmpeg
COPY --from=build /out/ /usr/local/bin/
COPY deployments/docker/entrypoint.sh /entrypoint.sh
RUN chmod 0555 /entrypoint.sh
EXPOSE 8080
ENTRYPOINT ["/entrypoint.sh"]
