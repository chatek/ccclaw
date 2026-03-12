package storage

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var weeklyFilePattern = regexp.MustCompile(`-(\d{4})-W(\d{2})\.jsonl$`)

func GenesisHash(year, week int) string {
	return fmt.Sprintf("GENESIS-W%d-%02d", year, week)
}

func ComputeHash(taskID, eventType, createdAt, detail, prevHash string) (string, error) {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%s", taskID, eventType, createdAt, detail, prevHash)))
	return fmt.Sprintf("%x", sum), nil
}

func ComputeTokenHash(record tokenJSONRecord, prevHash string) (string, error) {
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%s|%s|%d|%d|%d|%d|%.8f|%d|%t|%s|%s|%s",
		record.TaskID,
		record.SessionID,
		record.InputTokens,
		record.OutputTokens,
		record.CacheCreate,
		record.CacheRead,
		record.CostUSD,
		record.DurationMS,
		record.RTKEnabled,
		record.PromptFile,
		record.RecordedAt.UTC().Format(timeLayout),
		prevHash,
	)))
	return fmt.Sprintf("%x", sum), nil
}

func VerifyChain(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开事件流失败: %w", err)
	}
	defer file.Close()

	expectedGenesis, _ := genesisFromWeeklyFile(path)

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var prevHash string
	var lastSeq int64
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var record EventRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return fmt.Errorf("解析事件记录失败: %w", err)
		}
		if record.Seq <= 0 {
			return fmt.Errorf("事件序号无效: %d", record.Seq)
		}
		if lastSeq > 0 && record.Seq <= lastSeq {
			return fmt.Errorf("事件序号未递增: %d <= %d", record.Seq, lastSeq)
		}
		wantPrev := prevHash
		if wantPrev == "" && expectedGenesis != "" {
			wantPrev = expectedGenesis
		}
		if wantPrev != "" && record.PrevHash != wantPrev {
			return fmt.Errorf("事件链前向哈希不匹配: seq=%d got=%q want=%q", record.Seq, record.PrevHash, wantPrev)
		}
		hash, err := ComputeHash(
			record.TaskID,
			string(record.EventType),
			record.CreatedAt.UTC().Format(timeLayout),
			record.Detail,
			record.PrevHash,
		)
		if err != nil {
			return err
		}
		if record.Hash != hash {
			return fmt.Errorf("事件链哈希不匹配: seq=%d", record.Seq)
		}
		prevHash = record.Hash
		lastSeq = record.Seq
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取事件流失败: %w", err)
	}
	return nil
}

func VerifyTokenChain(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开 token 流失败: %w", err)
	}
	defer file.Close()

	expectedGenesis, _ := genesisFromWeeklyFile(path)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var prevHash string
	var lastSeq int64
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var record tokenJSONRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return fmt.Errorf("解析 token 记录失败: %w", err)
		}
		if record.Seq <= 0 {
			return fmt.Errorf("token 序号无效: %d", record.Seq)
		}
		if lastSeq > 0 && record.Seq <= lastSeq {
			return fmt.Errorf("token 序号未递增: %d <= %d", record.Seq, lastSeq)
		}
		wantPrev := prevHash
		if wantPrev == "" && expectedGenesis != "" {
			wantPrev = expectedGenesis
		}
		if wantPrev != "" && record.PrevHash != wantPrev {
			return fmt.Errorf("token 链前向哈希不匹配: seq=%d got=%q want=%q", record.Seq, record.PrevHash, wantPrev)
		}
		hash, err := ComputeTokenHash(record, record.PrevHash)
		if err != nil {
			return err
		}
		if record.Hash != hash {
			return fmt.Errorf("token 链哈希不匹配: seq=%d", record.Seq)
		}
		prevHash = record.Hash
		lastSeq = record.Seq
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取 token 流失败: %w", err)
	}
	return nil
}

func genesisFromWeeklyFile(path string) (string, bool) {
	matches := weeklyFilePattern.FindStringSubmatch(filepath.Base(path))
	if len(matches) != 3 {
		return "", false
	}
	var year, week int
	if _, err := fmt.Sscanf(matches[1], "%d", &year); err != nil {
		return "", false
	}
	if _, err := fmt.Sscanf(matches[2], "%d", &week); err != nil {
		return "", false
	}
	return GenesisHash(year, week), true
}
