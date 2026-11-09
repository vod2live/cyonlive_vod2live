FROM golang:1.23-alpine AS build
WORKDIR /src
ARG SERVICE=api
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X github.com/vod2live/cyonlive_vod2live/internal/buildinfo.Version=${VERSION} -X github.com/vod2live/cyonlive_vod2live/internal/buildinfo.Commit=${COMMIT} -X github.com/vod2live/cyonlive_vod2live/internal/buildinfo.BuildTime=${BUILD_TIME}" -o /out/service ./cmd/${SERVICE}

FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget
COPY --from=build /out/service /service
EXPOSE 8080
ENTRYPOINT ["/service"]
