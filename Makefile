GOFMT_FILES?=$$(find . -name '*.go' | grep -v vendor)

help:
	@echo "Various utilities for managing the terragrunt repository"

fmt:
	@echo "Running source files through gofmt..."
	gofmt -w $(GOFMT_FILES)

fmtcheck:
	pre-commit run goimports --all-files

install-pre-commit-hook:
	pre-commit install

# This build target just for convenience for those building directly from
# source. See also: .github/workflows/build.yml
build: terragrunt
terragrunt: $(shell find . \( -type d -name 'vendor' -prune \) \
                        -o \( -type f -name '*.go'   -print \) )
	set -xe ;\
	vtag_maybe_extra=$$(git describe --tags --abbrev=12 --dirty --broken) ;\
	CGO_ENABLED=0 go build -o $@ -ldflags "-s -w -X github.com/gruntwork-io/go-commons/version.Version=$${vtag_maybe_extra}" .

clean:
	rm -f terragrunt

run-lint:
	golangci-lint run -v --timeout=10m ./...

run-strict-lint:
	golangci-lint run -v --timeout=10m -c .strict.golangci.yml --new-from-rev origin/main ./...

generate-mocks:
	go generate ./...

license-check:
	go mod vendor
	licensei cache --debug
	licensei check --debug
	licensei header --debug

.PHONY: help fmt fmtcheck install-pre-commit-hook clean run-lint run-strict-lint
