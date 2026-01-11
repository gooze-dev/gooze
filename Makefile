name := gooze
bin := ./.bin
# Versions
COBRA_CLI_VERSION := v1.3.0
GOLANGCI_LINT_VERSION := v2.8.0

# Whitelisted packages (exclude examples explicitly)
PKG_WHITELIST :=  ./cmd/... ./internal/...


.PHONY: install-tools

install-tools:
	@echo "Installing development tools into $(bin)..."
	@mkdir -p $(bin)
	GOBIN=$(abspath $(bin)) go install github.com/spf13/cobra-cli@$(COBRA_CLI_VERSION)
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(abspath $(bin)) $(GOLANGCI_LINT_VERSION)


build:
	@go build -o $(bin)/$(name) main.go

lint:
	$(bin)/golangci-lint run $(PKG_WHITELIST)

test:
	@go test -v $(PKG_WHITELIST)

clean:
	@rm -rf $(bin)

run: build
	@$(bin)/$(name)

fmt:
	@go fmt $(PKG_WHITELIST)
	@$(bin)/golangci-lint fmt $(PKG_WHITELIST)