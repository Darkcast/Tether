ifdef RS_SHELL
LDFLAGS := $(LDFLAGS) -X 'main.defaultShell=$(RS_SHELL)'
endif

ifdef RS_PUB
LDFLAGS := $(LDFLAGS) -X 'main.authorizedKey=$(RS_PUB)'
endif

RS_PASS ?= $(shell hexdump -n 8 -e '2/4 "%08x"' /dev/urandom)
LDFLAGS := $(LDFLAGS) -X 'main.localPassword=$(RS_PASS)'

ifdef LUSER
LDFLAGS := $(LDFLAGS) -X 'main.LUSER=$(LUSER)'
endif

ifdef LHOST
LDFLAGS := $(LDFLAGS) -X 'main.LHOST=$(LHOST)'
endif

ifdef LPORT
LDFLAGS := $(LDFLAGS) -X 'main.LPORT=$(LPORT)'
endif

ifdef BPORT
LDFLAGS := $(LDFLAGS) -X 'main.HomeBindPort=$(BPORT)'
endif

ifdef NOCLI
LDFLAGS := $(LDFLAGS) -X 'main.NOCLI=$(NOCLI)'
endif

ifdef SESSLOG
LDFLAGS := $(LDFLAGS) -X 'main.SESSLOG=$(SESSLOG)'
endif

ifdef SNI
LDFLAGS := $(LDFLAGS) -X 'main.SNI=$(SNI)'
endif

ifdef PROXY
LDFLAGS := $(LDFLAGS) -X 'main.PROXY=$(PROXY)'
endif

# GOFLAGS keeps builds reproducible/offline once deps are vendored or cached
GO ?= go
BUILD = CGO_ENABLED=0 $(GO) build -trimpath -ldflags="$(LDFLAGS) -s -w"

.PHONY: build
build: clean
	# Host build
	$(BUILD) -o bin/ .
	# Linux
	GOOS=linux	GOARCH=amd64	$(BUILD) -o bin/tether-linux-amd64 .
	GOOS=linux	GOARCH=386	$(BUILD) -o bin/tether-linux-386 .
	GOOS=linux	GOARCH=arm64	$(BUILD) -o bin/tether-linux-arm64 .
	GOOS=linux	GOARCH=arm	GOARM=7	$(BUILD) -o bin/tether-linux-armv7 .
	# Windows
	GOOS=windows	GOARCH=amd64	$(BUILD) -o bin/tether-windows-amd64.exe .
	GOOS=windows	GOARCH=386	$(BUILD) -o bin/tether-windows-386.exe .
	GOOS=windows	GOARCH=arm64	$(BUILD) -o bin/tether-windows-arm64.exe .
	# macOS
	GOOS=darwin	GOARCH=amd64	$(BUILD) -o bin/tether-darwin-amd64 .
	GOOS=darwin	GOARCH=arm64	$(BUILD) -o bin/tether-darwin-arm64 .

.PHONY: clean
clean:
	rm -f bin/tether*

.PHONY: compressed
compressed: build
	@for f in $(shell ls bin); do upx -o "bin/upx_$${f}" "bin/$${f}"; done
