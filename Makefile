# ccclaw/Makefile
# 编译产物输出到 bin/（已在 .gitignore 中排除版本追踪）

BIN_DIR := bin
CMDS    := ingest run status

.PHONY: build test install systemd-install systemd-enable systemd-status lint clean help

build: ## 编译所有 cmd/ → bin/
	@mkdir -p $(BIN_DIR)
	@for cmd in $(CMDS); do \
		echo "编译 ccclaw-$$cmd ..."; \
		go build -o $(BIN_DIR)/ccclaw-$$cmd ./cmd/$$cmd/; \
	done
	@echo "构建完成: $(BIN_DIR)/"

test: ## 运行单元测试
	go test ./...

lint: ## 代码检查（需安装 golangci-lint）
	@which golangci-lint > /dev/null 2>&1 || (echo "请安装 golangci-lint: https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run ./...

install: build ## 从 bin/ 安装到 /usr/local/bin/
	@for cmd in $(CMDS); do \
		install -m 755 $(BIN_DIR)/ccclaw-$$cmd /usr/local/bin/ccclaw-$$cmd; \
		echo "已安装 /usr/local/bin/ccclaw-$$cmd"; \
	done

systemd-install: ## 安装 systemd 单元文件（需 root）
	install -m 644 systemd/ccclaw-ingest.service /etc/systemd/system/
	install -m 644 systemd/ccclaw-ingest.timer   /etc/systemd/system/
	install -m 644 systemd/ccclaw-run.service    /etc/systemd/system/
	install -m 644 systemd/ccclaw-run.timer      /etc/systemd/system/
	systemctl daemon-reload
	@echo "systemd 单元文件已安装"

systemd-enable: systemd-install ## 启用并启动 timer（需 root）
	systemctl enable --now ccclaw-ingest.timer
	systemctl enable --now ccclaw-run.timer
	@echo "ccclaw timers 已启用"

systemd-status: ## 查看 timer 状态
	@systemctl status ccclaw-ingest.timer ccclaw-run.timer --no-pager || true

clean: ## 清理 bin/ 产物
	rm -rf $(BIN_DIR)

help: ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
