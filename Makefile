PREFIX ?= /usr/local
GOBIN ?=
BINDIR ?= $(if $(GOBIN),$(GOBIN),$(PREFIX)/bin)
GO ?= go
BINARY := herdr-top

.PHONY: build test race vet install uninstall clean
build:
	$(GO) build -trimpath -o bin/$(BINARY) ./cmd/herdr-top

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

install:
	mkdir -p "$(DESTDIR)$(BINDIR)"
	$(GO) build -trimpath -o "$(DESTDIR)$(BINDIR)/$(BINARY)" ./cmd/herdr-top

uninstall:
	rm -f "$(DESTDIR)$(BINDIR)/$(BINARY)"

clean:
	rm -rf bin coverage.out
