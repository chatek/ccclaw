VERSION ?= $(shell date '+%y.%m.%d.%H%M')
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf 'unknown')
GOOS ?= linux
GOARCH ?= amd64
APP_NAME := ccclaw
DIST_DIR := dist
OPS_DIR := ops
BUILD_DIR := .build
RELEASE_DIR := .release
ARCHIVE_ROOT ?= /ops/logs/ccclaw
PACKAGE_BASENAME := $(APP_NAME)_$(VERSION)_$(GOOS)_$(GOARCH)
PACKAGE_DIR := $(RELEASE_DIR)/$(PACKAGE_BASENAME)
PACKAGE_FILE := $(RELEASE_DIR)/$(PACKAGE_BASENAME).tar.gz
CHECKSUM_FILE := $(RELEASE_DIR)/SHA256SUMS
BIN := $(DIST_DIR)/bin/$(APP_NAME)
LDFLAGS := -X github.com/41490/ccclaw/internal/buildinfo.Version=$(VERSION) -X github.com/41490/ccclaw/internal/buildinfo.Commit=$(COMMIT)

.PHONY: fmt test tidy build dist-sync package checksum sign archive release clean

fmt:
	gofmt -w ./cmd ./internal

test:
	go test ./...

tidy:
	go mod tidy

build:
	@mkdir -p $(DIST_DIR)/bin
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/ccclaw

dist-sync: build
	@mkdir -p $(DIST_DIR)/ops
	rsync -a --delete $(OPS_DIR)/ $(DIST_DIR)/ops/
	chmod 755 $(DIST_DIR)/install.sh $(DIST_DIR)/upgrade.sh

package: dist-sync
	mkdir -p $(RELEASE_DIR)
	rm -rf $(PACKAGE_DIR)
	mkdir -p $(PACKAGE_DIR)
	rsync -a --delete $(DIST_DIR)/ $(PACKAGE_DIR)/
	tar -C $(RELEASE_DIR) -czf $(PACKAGE_FILE) $(PACKAGE_BASENAME)

checksum: package
	cd $(RELEASE_DIR) && sha256sum $(notdir $(PACKAGE_FILE)) > $(notdir $(CHECKSUM_FILE))

sign: checksum
	@if ! command -v gpg >/dev/null 2>&1; then echo "缺少 gpg，无法生成签名"; exit 1; fi
	@if ! command -v minisign >/dev/null 2>&1; then echo "缺少 minisign，无法生成签名"; exit 1; fi
	@if [ -n "$(GPG_KEY_ID)" ]; then \
		gpg --batch --yes --armor --local-user "$(GPG_KEY_ID)" --detach-sign -o $(PACKAGE_FILE).asc $(PACKAGE_FILE); \
		gpg --batch --yes --armor --local-user "$(GPG_KEY_ID)" --detach-sign -o $(CHECKSUM_FILE).asc $(CHECKSUM_FILE); \
	else \
		gpg --batch --yes --armor --detach-sign -o $(PACKAGE_FILE).asc $(PACKAGE_FILE); \
		gpg --batch --yes --armor --detach-sign -o $(CHECKSUM_FILE).asc $(CHECKSUM_FILE); \
	fi
	@if [ -z "$(MINISIGN_KEY_FILE)" ]; then echo "请提供 MINISIGN_KEY_FILE=/path/to/minisign.key"; exit 1; fi
	@if [ "$(MINISIGN_UNPROTECTED)" = "1" ]; then \
		minisign -S -W -s "$(MINISIGN_KEY_FILE)" -m $(PACKAGE_FILE) -x $(PACKAGE_FILE).minisig -t "$(VERSION)"; \
		minisign -S -W -s "$(MINISIGN_KEY_FILE)" -m $(CHECKSUM_FILE) -x $(CHECKSUM_FILE).minisig -t "$(VERSION)"; \
	else \
		minisign -S -s "$(MINISIGN_KEY_FILE)" -m $(PACKAGE_FILE) -x $(PACKAGE_FILE).minisig -t "$(VERSION)"; \
		minisign -S -s "$(MINISIGN_KEY_FILE)" -m $(CHECKSUM_FILE) -x $(CHECKSUM_FILE).minisig -t "$(VERSION)"; \
	fi

archive: sign
	mkdir -p $(ARCHIVE_ROOT)/$(VERSION)
	cp $(PACKAGE_FILE) $(PACKAGE_FILE).asc $(PACKAGE_FILE).minisig $(CHECKSUM_FILE) $(CHECKSUM_FILE).asc $(CHECKSUM_FILE).minisig $(ARCHIVE_ROOT)/$(VERSION)/

release: archive
	gh release create $(VERSION) \
		$(PACKAGE_FILE) \
		$(PACKAGE_FILE).asc \
		$(PACKAGE_FILE).minisig \
		$(CHECKSUM_FILE) \
		$(CHECKSUM_FILE).asc \
		$(CHECKSUM_FILE).minisig \
		--title "$(VERSION)" \
		--generate-notes

clean:
	rm -rf $(BUILD_DIR) $(RELEASE_DIR) $(DIST_DIR)/bin/$(APP_NAME)
