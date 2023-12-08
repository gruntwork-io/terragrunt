# Some of the targets in this file were taken from the terraform project:
# https://github.com/hashicorp/terraform/blob/master/scripts/gofmtcheck.sh

GOFMT_FILES?=$$(find . -name '*.go' | grep -v vendor)

help:
	@echo "Various utilities for managing the terragrunt repository"

fmtcheck:
	@sh -c "'$(CURDIR)/scripts/gofmtcheck.sh'"

fmt:
	@echo "Running source files through gofmt..."
	gofmt -w $(GOFMT_FILES)

install-pre-commit-hook:
	@if [ -f .git/hooks/pre-commit -o -L .git/hooks/pre-commit ]; then \
       echo ""; \
       echo "There is already a pre-commit hook installed. Remove it and run 'make"; \
       echo "install-pre-commit-hook again, or manually alter it to add the contents"; \
       echo "of 'scripts/pre-commit'."; \
       echo ""; \
       exit 1; \
   fi
	@ln -s scripts/pre-commit .git/hooks/pre-commit
	@echo "pre-commit hook installed."


# This build target just for convenience for those building directly from
# source. See also: .circleci/config.yml
build: terragrunt
terragrunt: $(shell find . \( -type d -name 'vendor' -prune \) \
                        -o \( -type f -name '*.go'   -print \) )
	set -xe ;\
	vtag_maybe_extra=$$(git describe --tags --abbrev=12 --dirty --broken) ;\
	go build -o $@ -ldflags "-X github.com/gruntwork-io/go-commons/version.Version=$${vtag_maybe_extra} -extldflags '-static'" .

clean:
	rm -f terragrunt

install-lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.54.2

run-lint:
	golangci-lint run -v --timeout=5m ./...

.PHONY: help fmtcheck fmt install-fmt-hook clean install-lint run-lint
