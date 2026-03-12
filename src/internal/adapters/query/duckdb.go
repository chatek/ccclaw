package query

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const commandTimeout = 30 * time.Second

type DuckDB struct {
	bin string
}

func New() *DuckDB {
	bin, _ := exec.LookPath("duckdb")
	if strings.TrimSpace(bin) == "" {
		bin = "duckdb"
	}
	return &DuckDB{bin: bin}
}

func NewWithBinary(bin string) *DuckDB {
	return &DuckDB{bin: bin}
}

func (d *DuckDB) Binary() string {
	return d.bin
}

func (d *DuckDB) Query(ctx context.Context, sql string) ([]map[string]any, error) {
	output, err := d.run(ctx, true, sql)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(output)) == 0 {
		return []map[string]any{}, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal(output, &rows); err != nil {
		return nil, fmt.Errorf("解析 duckdb JSON 输出失败: %w", err)
	}
	return rows, nil
}

func (d *DuckDB) Execute(ctx context.Context, sql string) error {
	_, err := d.run(ctx, false, sql)
	return err
}

func (d *DuckDB) CopyJSONLToParquet(ctx context.Context, sourcePath, targetPath string) error {
	sourcePath = filepath.Clean(sourcePath)
	targetPath = filepath.Clean(targetPath)
	sql := fmt.Sprintf(
		"COPY (SELECT * FROM read_json_auto('%s')) TO '%s' (FORMAT PARQUET, COMPRESSION ZSTD)",
		escapeSQLString(sourcePath),
		escapeSQLString(targetPath),
	)
	return d.Execute(ctx, sql)
}

func (d *DuckDB) run(ctx context.Context, jsonOutput bool, sql string) ([]byte, error) {
	if strings.TrimSpace(sql) == "" {
		return nil, fmt.Errorf("duckdb SQL 不能为空")
	}
	runCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	args := []string{":memory:"}
	if jsonOutput {
		args = append(args, "-json")
	}
	args = append(args, "-c", sql)
	cmd := exec.CommandContext(runCtx, d.bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if runCtx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("执行 duckdb 超时")
	}
	if err != nil {
		text := strings.TrimSpace(stderr.String())
		if text == "" {
			text = err.Error()
		}
		return nil, fmt.Errorf("执行 duckdb 失败: %s", text)
	}
	return output, nil
}

func escapeSQLString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
