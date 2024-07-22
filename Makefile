GO=go
LDFLAGS?=-s -w
TESTFILE=_testok
MOUNT_PATH=/local

# go tools versions
GOLANGCI=github.com/golangci/golangci-lint/cmd/golangci-lint@v1.57.1
gofumpt=mvdan.cc/gofumpt@latest
goimports=golang.org/x/tools/cmd/goimports@latest
mockgen=go.uber.org/mock/mockgen@v0.4.0
gotestsum=gotest.tools/gotestsum@v1.11.0
actionlint=github.com/rhysd/actionlint/cmd/actionlint@latest

mocks: install-tools ## Generate all mocks
	$(GO) generate ./...

install:
		sh install-hooks.sh
default: build

test: install-tools test-run

test-run:
		gotestsum --format pkgname-and-test-fails -- -count=1 -shuffle=on  -coverprofile=coverage.txt -vet=all ./...

build:
		go build -o bin/$(NAME) ./cmd/$(NAME).go

test-with-coverage: test coverage

coverage:
	go tool cover -html=coverage.txt -o coverage.html

install-tools:
	go install mvdan.cc/gofumpt@latest
	go install gotest.tools/gotestsum@v1.8.2

.PHONY: lint
lint: fmt ## Run linters on all go files
	docker run --rm -v $(shell pwd):/app:ro -w /app golangci/golangci-lint:v1.51.1 bash -e -c \
		'golangci-lint run -v --timeout 5m'

.PHONY: fmt
fmt: install-tools ## Formats all go files
	gofumpt -l -w -extra  .
