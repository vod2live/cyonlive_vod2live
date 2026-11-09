APP=vod2live

.PHONY: run test fmt compose-up compose-down e2e

run:
	go run ./cmd/api

test:
	GOTOOLCHAIN=local go test ./...

fmt:
	gofmt -w cmd internal pkg

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down -v

e2e:
	tests/integration/e2e.sh
