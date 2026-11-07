FROM golang:1.23-alpine AS build
WORKDIR /src
ARG SERVICE=api
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/service ./cmd/${SERVICE}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/service /service
EXPOSE 8080
ENTRYPOINT ["/service"]
