.PHONY: build test clean install

# Default installation prefix (user-local)
PREFIX ?= $(HOME)/.local

build:
	go build -o bin/devtether ./cmd/devtether

test:
	./scripts/test.sh

clean:
	rm -rf bin/

install: build
	mkdir -p $(PREFIX)/bin
	install -m 755 bin/devtether $(PREFIX)/bin/devtether
	@echo ""
	@echo "DevTether installed to $(PREFIX)/bin/devtether"
	@echo "Make sure $(PREFIX)/bin is in your PATH."
	@echo "Note: If you want to bind to ports 80/53 on Linux without root, run:"
	@echo "sudo setcap cap_net_bind_service=+ep $(PREFIX)/bin/devtether"
