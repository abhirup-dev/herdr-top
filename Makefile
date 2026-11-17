PREFIX ?= /usr/local
GOBIN ?=
BINDIR ?= $(if $(GOBIN),$(GOBIN),$(PREFIX)/bin)
GO ?= go
BINARY := herdr-top

.PHONY: build test race vet install install-mod uninstall uninstall-mod clean
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

# Herdr desktop app mod: a link to mods/herdr-top; re-run after edits (re-linking reloads it).
APP_MODS = $${HERDR_APP_HOME:-$$HOME/.config/herdr-app}/mods
install-mod:
	ln -sfn "$(CURDIR)/mods/herdr-top" "$(APP_MODS)/herdr-top"

uninstall-mod:
	@if [ -L "$(APP_MODS)/herdr-top" ]; then rm "$(APP_MODS)/herdr-top"; echo "removed the herdr-top mod link"; fi

uninstall: uninstall-mod
	rm -f "$(DESTDIR)$(BINDIR)/$(BINARY)"

clean:
	rm -rf bin coverage.out
