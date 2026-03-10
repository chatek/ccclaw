VERSION ?= $(shell date '+%y.%m.%d.%H%M')
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf 'unknown')
GOOS ?= linux
GOARCH ?= amd64
GO ?= $(shell if [ -x /usr/local/go/bin/go ]; then printf '/usr/local/go/bin/go'; else command -v go; fi)
GOFMT ?= $(shell if [ -x /usr/local/go/bin/gofmt ]; then printf '/usr/local/go/bin/gofmt'; else command -v gofmt; fi)
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

.PHONY: fmt test test-install tidy build dist-sync package checksum sign archive release clean

fmt:
	$(GOFMT) -w ./cmd ./internal

test:
	$(GO) test ./...

test-install: dist-sync
	bash tests/install_regression.sh

tidy:
	$(GO) mod tidy

build:
	@mkdir -p $(DIST_DIR)/bin
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/ccclaw

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
	@echo "当前发布链路已改为 SHA256SUMS 校验；sign 目标保留为 checksum 兼容别名"

archive: checksum
	mkdir -p $(ARCHIVE_ROOT)/$(VERSION)
	cp $(PACKAGE_FILE) $(CHECKSUM_FILE) $(ARCHIVE_ROOT)/$(VERSION)/

release: archive
	@git diff --quiet && git diff --cached --quiet || { echo "工作树存在未提交变更，禁止创建 release"; exit 1; }
	@gh auth status >/dev/null 2>&1 || { echo "gh 未登录，无法创建 release"; exit 1; }
	gh release create $(VERSION) \
		$(PACKAGE_FILE) \
		$(CHECKSUM_FILE) \
		--title "$(VERSION)" \
		--generate-notes

clean:
	rm -rf $(BUILD_DIR) $(RELEASE_DIR) $(DIST_DIR)/bin/$(APP_NAME)
