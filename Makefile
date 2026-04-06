BINARY  = ghs
VERSION = $(shell grep 'const version' utils.go | awk -F'"' '{print $$2}')
LDFLAGS = -s -w -X main.version=$(VERSION)

PLATFORMS = \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/arm64

.PHONY: all build clean dist install $(PLATFORMS)

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

# Cross-compile all platforms
dist: $(PLATFORMS)
	@echo "Built all platforms in dist/"

$(PLATFORMS):
	@mkdir -p dist
	$(eval GOOS := $(word 1,$(subst /, ,$@)))
	$(eval GOARCH := $(word 2,$(subst /, ,$@)))
	$(eval EXT := $(if $(filter windows,$(GOOS)),.exe,))
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-$(GOOS)-$(GOARCH)$(EXT) .
	@echo "  ✓ $(BINARY)-$(GOOS)-$(GOARCH)$(EXT)"

# Install locally
install: build
	cp $(BINARY) /usr/local/bin/$(BINARY)
	@echo "Installed to /usr/local/bin/$(BINARY)"

clean:
	rm -f $(BINARY)
	rm -rf dist/

# Show version
version:
	@echo "$(BINARY) v$(VERSION)"
