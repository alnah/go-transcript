BINARY := transcript

# Version info injected at build time
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

.PHONY: help build test test-integration test-e2e test-all test-cover test-cover-all bench run clean fmt vet lint sec check check-all tools deps version labels testdata

.DEFAULT_GOAL := help

help: ## Display this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

tools: ## Install development tools (staticcheck, gosec)
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest

deps: ## Install dependencies
	go mod download

build: ## Build the binary
	go build $(LDFLAGS) -o $(BINARY) ./cmd/transcript

version: ## Show version that would be injected
	@echo "Version: $(VERSION)"
	@echo "Commit:  $(COMMIT)"

test: ## Run unit tests
	go test -v ./...

test-integration: ## Run integration tests (requires FFmpeg)
	go test -v -tags=integration ./...

test-e2e: ## Run E2E tests (requires OPENAI_API_KEY + FFmpeg)
	go test -v -tags=e2e ./...

test-all: ## Run all tests (unit + integration + e2e)
	go test -v -tags=integration,e2e ./...

test-cover: ## Run unit tests with coverage report
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

test-cover-all: ## Run all tests with coverage report
	go test -v -tags=integration -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

bench: ## Run benchmarks
	go test -bench=. -benchmem ./... | tee bench.out

run: build ## Build and run the binary
	./$(BINARY)

clean: ## Remove build artifacts and temp files
	rm -f $(BINARY) coverage.out coverage.html bench.out bench.old
	rm -f *.ogg *.mp3 *.wav *.m4a

fmt: ## Format source code
	go fmt ./...

vet: ## Run go vet for static analysis
	go vet ./...

lint: ## Run staticcheck linter
	staticcheck ./...

sec: ## Run gosec security scanner
	gosec ./...

check: fmt vet lint test ## Run all checks (unit tests only)

check-all: fmt vet lint sec test-integration ## Run all checks including integration tests

# Development helpers
record-test: build ## Record a 10s test audio
	./$(BINARY) record -d 10s -o test.ogg

transcribe-test: build ## Transcribe test audio (requires test.ogg)
	./$(BINARY) transcribe test.ogg -o test.md

transcribe-test-brainstorm: build ## Transcribe with brainstorm template
	./$(BINARY) transcribe test.ogg -o test_brainstorm.md -t brainstorm

live-test: build ## Full live test (30s recording + transcription)
	./$(BINARY) live -d 30s -o live_test.md -t brainstorm --keep-audio

labels: ## Configure GitHub labels (requires gh CLI)
	./scripts/setup-labels.sh

# Test fixtures (do NOT regenerate unless necessary - see testdata/README.md)
testdata: testdata/sample.ogg testdata/short.ogg ## Regenerate test audio fixtures

testdata/sample.ogg:
	ffmpeg -y \
	  -f lavfi -i "sine=frequency=440:duration=2" \
	  -f lavfi -i "anullsrc=r=16000:cl=mono" \
	  -f lavfi -i "sine=frequency=880:duration=2" \
	  -f lavfi -i "anullsrc=r=16000:cl=mono" \
	  -f lavfi -i "sine=frequency=660:duration=2" \
	  -filter_complex "[0]aresample=16000[s0];[1]atrim=duration=1[p1];[2]aresample=16000[s1];[3]atrim=duration=1[p2];[4]aresample=16000[s2];[s0][p1][s1][p2][s2]concat=n=5:v=0:a=1[out]" \
	  -map "[out]" -ac 1 -c:a libvorbis -q:a 2 \
	  testdata/sample.ogg

testdata/short.ogg:
	ffmpeg -y \
	  -f lavfi -i "sine=frequency=440:duration=3" \
	  -ar 16000 -ac 1 -c:a libvorbis -q:a 2 \
	  testdata/short.ogg
