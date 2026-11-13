FROM golang:1.23.6-alpine AS build

ARG MINIO_COMMIT=7d4c64d0753217248679a93749daaa352ffe6fde

RUN apk add --no-cache git
WORKDIR /src
RUN git init . \
    && git remote add origin https://github.com/minio/minio.git \
    && git fetch --depth=1 origin "$MINIO_COMMIT" \
    && git checkout --detach FETCH_HEAD
RUN mkdir -p /out \
    && CGO_ENABLED=0 go build -tags=kqueue -trimpath -o /out/minio .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /out/minio /usr/local/bin/minio

EXPOSE 9000 9001
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/minio"]
CMD ["server", "/data", "--console-address", ":9001"]
