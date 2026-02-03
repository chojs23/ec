SHELL := /bin/sh

APP := ec
ALIAS := easy-conflict
GOFILES := $(shell if command -v fd >/dev/null 2>&1; then fd -e go .; else find . -name '*.go'; fi)

.PHONY: build test fmt fmt-check install clean

build:
	go build -o $(APP) ./cmd/ec
	ln -sf $(APP) $(ALIAS)

test:
	go test ./...

fmt:
	gofmt -w $(GOFILES)

fmt-check:
	test -z "$(shell gofmt -l $(GOFILES))"

install:
	./scripts/install.sh

clean:
	rm -f $(APP) $(ALIAS)
