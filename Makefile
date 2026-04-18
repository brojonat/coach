.PHONY: build run run-headless tail test vet fmt tidy clean

BIN := bin/coach
LOG_DIR := logs

build:
	go build -o $(BIN) ./cmd/coach

run: build | $(LOG_DIR)
	@set -a; . ./.env; set +a; $(BIN) 2>$(LOG_DIR)/coach.log

run-headless: build | $(LOG_DIR)
	@set -a; . ./.env; set +a; LOG_LEVEL=debug $(BIN) --scenario baseline --no-audio 2>$(LOG_DIR)/coach.log

tail: | $(LOG_DIR)
	@if command -v jq >/dev/null 2>&1; then \
		tail -f $(LOG_DIR)/coach.log | jq -C '[.time, .level, .source, .msg] + [to_entries | map(select(.key | IN("time","level","source","msg") | not)) | from_entries] | @json'; \
	else \
		tail -f $(LOG_DIR)/coach.log; \
	fi

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
