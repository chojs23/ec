SHELL := /bin/sh

APP := easy-conflict
GOFILES := $(shell if command -v fd >/dev/null 2>&1; then fd -e go .; else find . -name '*.go'; fi)

.PHONY: build test fmt fmt-check install clean

build:
	go build -o $(APP) ./cmd/easy-conflict

test:
	go test ./...

fmt:
	gofmt -w $(GOFILES)

fmt-check:
	test -z "$(shell gofmt -l $(GOFILES))"

install:
	./scripts/install.sh

clean:
	rm -f $(APP)
