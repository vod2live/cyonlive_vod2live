APP=vod2live

.PHONY: run test fmt compose-up compose-down e2e contract load-test playback-load-test ha-smoke migrate verify-migrations db-backup db-verify-backup db-restore

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

contract:
	tests/integration/contract.sh

load-test:
	scripts/load-test.sh

playback-load-test:
	scripts/playback-load-test.sh

ha-smoke:
	scripts/ha-smoke.sh

migrate:
	scripts/migrate.sh

verify-migrations:
	scripts/verify-migrations.sh

db-backup:
	scripts/db-backup.sh

db-verify-backup:
	scripts/db-verify-backup.sh $(BACKUP_FILE)

db-restore:
	CONFIRM_RESTORE=YES scripts/db-restore.sh $(BACKUP_FILE)
