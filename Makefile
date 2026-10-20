name := gooze
bin := ./.bin
# Versions
COBRA_CLI_VERSION := v1.3.0
GOLANGCI_LINT_VERSION := v2.15.0
MOCKERY_VERSION := v2.53.8
SEMGREP_VERSION := 1.181.0

# Default benchtime for benchmarks run via `make test`.
BENCHTIME ?= 100ms

# Whitelisted packages (exclude examples explicitly)
PKG_WHITELIST :=  ./cmd/... ./internal/... ./pkg/...
# Same tree as PKG_WHITELIST, as plain paths for tools that do not understand `/...`.
SEMGREP_TARGETS := ./cmd ./internal ./pkg main.go

.PHONY: all install-tools build lint test clean run fmt mocks clean-mocks install-precommit semgrep install-semgrep

all: install-precommit build


prepare-env:
	@echo "Preparing development environment..."
	@mkdir -p $(bin)

install-precommit-tools: prepare-env
	@echo "Installing pre-commit..."
	@pip install --user pre-commit || pip3 install --user pre-commit

install-cobra-cli: prepare-env
	@echo "Installing cobra-cli $(COBRA_CLI_VERSION)..."
	@GOBIN=$(abspath $(bin)) go install github.com/spf13/cobra-cli@$(COBRA_CLI_VERSION)

install-golangci-lint: prepare-env
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."
	@# Build from source with the local Go toolchain so the linter's embedded
	@# type-checker matches the installed Go version (the prebuilt release binary
	@# panics when run against a newer Go than it was compiled with).
	@GOBIN=$(abspath $(bin)) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

install-mockery: prepare-env
	@echo "Installing mockery $(MOCKERY_VERSION)..."
	@GOBIN=$(abspath $(bin)) go install github.com/vektra/mockery/v2@$(MOCKERY_VERSION)

install-semgrep: prepare-env
	@echo "Installing semgrep $(SEMGREP_VERSION)..."
	@pip install --user semgrep==$(SEMGREP_VERSION) || pip3 install --user semgrep==$(SEMGREP_VERSION)

install-tools: install-cobra-cli install-precommit-tools install-golangci-lint install-mockery install-semgrep
	@echo "All tools installed."


build:
	@go build -o $(bin)/$(name) main.go
	@echo "Built $(name) binary at $(PWD)/$(bin)/$(name)"

lint:
	@echo "Running golangci-lint..."
	@$(bin)/golangci-lint run $(PKG_WHITELIST)

semgrep:
	@echo "Running semgrep..."
	@semgrep scan --config .semgrep/ --config p/golang --error --disable-version-check --quiet $(SEMGREP_TARGETS)

test:
	@packages=$$(go list $(PKG_WHITELIST) | grep -v '/mocks$$' | grep -v '/examples/'); \
	coverpkgs=$$(echo "$$packages" | paste -sd, -); \
	go test -bench=. -benchtime=$(BENCHTIME) -coverpkg="$$coverpkgs" $$packages -coverprofile=coverage.out -cover; \
	go tool cover -html=coverage.out -o coverage.html

clean: clean-mocks
	@rm -rf $(bin)
	@rm -f coverage.out coverage.html
	@rm -f gooze.yaml
	@rm -f .gooze.log .gooze-*.log.gz
	@rm -rf .gooze-reports

run: build
	@$(bin)/$(name) $$(echo "$(filter-out $@,$(MAKECMDGOALS))" | sed 's/^-/-/')

%:
	@:

fmt:
	@go fmt $(PKG_WHITELIST)
	@$(bin)/golangci-lint fmt $(PKG_WHITELIST)

clean-mocks:
	@echo "Cleaning mocks..."
	@rm -rf internal/*/mocks
	@rm -f pkg/mock_*.go pkg/*/mock_*.go

mocks: clean-mocks
	@echo "Generating mocks..."
	@$(bin)/mockery --all --config .mockery.yaml

install-precommit: install-precommit-tools
	@echo "Installing pre-commit hooks..."
	@pre-commit install
	@echo "✅ Pre-commit hooks installed!"
