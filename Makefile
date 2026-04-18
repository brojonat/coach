.PHONY: build run run-headless test vet fmt tidy clean

BIN := bin/coach
LOG_DIR := logs

build:
	go build -o $(BIN) ./cmd/coach

run: build | $(LOG_DIR)
	@set -a; . ./.env; set +a; $(BIN) 2>&1 | tee $(LOG_DIR)/run.log

run-headless: build | $(LOG_DIR)
	@set -a; . ./.env; set +a; $(BIN) --no-audio --debug 2>&1 | tee $(LOG_DIR)/headless.log

$(LOG_DIR):
	@mkdir -p $(LOG_DIR)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

clean:
	rm -rf bin $(LOG_DIR)
