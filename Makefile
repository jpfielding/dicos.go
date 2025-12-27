SHELL := /bin/bash
SCRIPTS_DIR := $(CURDIR)/scripts

MODULE_NAME = uio.go
REPO_PATH = $(shell git rev-parse --show-toplevel || pwd)
REPO_NAME = $(shell basename $$REPO_PATH)
GIT_SHA = $(shell git rev-parse --short HEAD)
BUILD_DATE = $(shell date +%Y-%m-%d)
BUILD_TIME = $(shell date +%H:%M:%S)

all: test build

help:  ## Prints the help/usage docs.
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST) | sort

nuke:  ## Resets the project to its initial state.
	git clean -ffdqx

clean:  ## Removes build/test outputs.
	rm -rf bin *.test
	go clean

update-deps:  ## Tidies up the go module.
	go get -u ./... && go mod tidy && go mod vendor	

### TEST commands ####
test:  ## Runs short tests.
	go test -short -v ./pkg/...

test-report: ## Runs ALL tests with junit report output
	mkdir -p tmp && gotestsum --junitfile tmp/report.xml --format testname ./pkg/...

.PHONY: integration-test
	go test -v ./pkg/...

lint:  ## Run static code analysis
	golangci-lint run ./pkg/...

lint-report: ## Run golangci-lint report
	mkdir -p tmp && golangci-lint run --issues-exit-code 0 --out-format code-climate:tmp/gl-code-quality-report.json,line-number

vet:  ## Runs Golang's static code analysis
	go vet ./pkg/...

vulnerability: install-govulncheck ## Runs the vulnerability check.
	govulncheck ./pkg/...

vulnerability-report: ## Runs the vulnerability check.
	mkdir -p tmp && govulncheck -json ./pkg/... > tmp/go-vuln-report.json

#### INSTALL TOOLS ####
install-tools: install-claude install-cert-tools install-cicd-tools

.PHONY: install-claude
install-claude:
	claude update || \
	curl -fsSL https://claude.ai/install.sh | bash 2>/dev/null && \
	claude doctor 

.PHONY: install-codex
install-codex: # opena-ai codex.rs
	ARCH="$(shell arch | sed 's/arm64/aarch64/' | sed 's/amd64/x86_64/')" && \
	OS=$(shell uname  | sed 's/Darwin/apple-darwin/' | sed 's/Linux/unknown-linux-musl/') && \
	curl -o /tmp/codex.tar.gz -L "https://github.com/openai/codex/releases/download/rust-v0.98.0/codex-$${ARCH}-$${OS}.tar.gz" && \
	tar -xvzf /tmp/codex.tar.gz -C /tmp && \
	mv /tmp/codex-$${ARCH}-$${OS} ${HOME}/bin/codex && \
	chmod +x ${HOME}/bin/codex

install-cert-tools: install-mkcert install-certigo

install-mkcert: # 
	go install filippo.io/mkcert@latest

install-certigo: #
	go install github.com/square/certigo@latest

install-cicd-tools: install-gotestsum install-govulncheck install-golint

install-gotestsum: #
	go install gotest.tools/gotestsum@latest

install-govulncheck: #
	go install golang.org/x/vuln/cmd/govulncheck@latest

install-golint: #
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

#### BUILD commands ####
build:  ## Build the library
	mkdir -p bin
	CGO_ENABLED=0 go build \
		-trimpath \
		-ldflags "-X 'main.GitSHA=$(GIT_SHA)'" \
		-o bin/dicosctl \
		cmd/ctl/*.go

