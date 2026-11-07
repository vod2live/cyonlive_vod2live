APP=vod2live

.PHONY: run test fmt compose-up

run:
	go run ./cmd/api

test:
	go test ./...

fmt:
	gofmt -w cmd internal pkg

compose-up:
	docker compose up --build
