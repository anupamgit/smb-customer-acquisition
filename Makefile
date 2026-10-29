# Run from repository root. Loads .env from here via src/api env loader.

.PHONY: test generate-scenarios

test:
	cd src/api && go test ./...

generate-scenarios:
	cd src/api && go run ./cmd/scenario-generate --all
