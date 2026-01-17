name := gooze
bin := ./.bin
# Versions
COBRA_CLI_VERSION := v1.3.0
GOLANGCI_LINT_VERSION := v2.8.0
MOCKERY_VERSION := v2.53.5

# Whitelisted packages (exclude examples explicitly)
PKG_WHITELIST :=  ./cmd/... ./internal/...

.PHONY: all install-tools build lint test clean run fmt mocks clean-mocks install-precommit

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
	@curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(abspath $(bin)) $(GOLANGCI_LINT_VERSION)

install-mockery: prepare-env
	@echo "Installing mockery $(MOCKERY_VERSION)..."
	@GOBIN=$(abspath $(bin)) go install github.com/vektra/mockery/v2@$(MOCKERY_VERSION)

install-tools: install-cobra-cli install-precommit-tools install-golangci-lint install-mockery
	@echo "All tools installed."


build:
	@go build -o $(bin)/$(name) main.go
	@echo "Built $(name) binary at $(PWD)/$(bin)/$(name)"

lint:
	@echo "Running golangci-lint..."
	@$(bin)/golangci-lint run $(PKG_WHITELIST)

test:
	@packages=$$(go list $(PKG_WHITELIST) | grep -v '/mocks$$' | grep -v '/examples/'); \
	coverpkgs=$$(echo "$$packages" | paste -sd, -); \
	go test -coverpkg="$$coverpkgs" $$packages -coverprofile=coverage.out -cover; \
	go tool cover -html=coverage.out -o coverage.html

clean:
	@rm -rf $(bin)

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

mocks: clean-mocks
	@echo "Generating mocks..."
	@$(bin)/mockery --all --config .mockery.yaml

install-precommit: install-precommit-tools
	@echo "Installing pre-commit hooks..."
	@pre-commit install
	@echo "✅ Pre-commit hooks installed!"
