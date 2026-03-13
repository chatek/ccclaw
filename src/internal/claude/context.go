package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	defaultContextWindowSize = 200000
	usableContextRatio       = 0.8
)

type ContextMetrics struct {
	Model             string
	ContextWindowSize int
	UsableTokens      int
	ContextLength     int
	UsablePercent     float64
	UpdatedAt         time.Time
}

type transcriptLine struct {
	IsSidechain       bool      `json:"isSidechain"`
	IsAPIErrorMessage bool      `json:"isApiErrorMessage"`
	Timestamp         string    `json:"timestamp"`
	Message           *struct {
		Model string `json:"model"`
		Usage *struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func ContextMetricsFromTranscript(transcriptPath, fallbackModel string, fallbackWindowSize int) (*ContextMetrics, error) {
	file, err := os.Open(strings.TrimSpace(transcriptPath))
	if err != nil {
		return nil, fmt.Errorf("打开 Claude transcript 失败: %w", err)
	}
	defer file.Close()

	var (
		latestLine *transcriptLine
		latestTime time.Time
	)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item transcriptLine
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		if item.IsSidechain || item.IsAPIErrorMessage || item.Message == nil || item.Message.Usage == nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, item.Timestamp)
		if err != nil {
			continue
		}
		if latestLine == nil || ts.After(latestTime) {
			copy := item
			latestLine = &copy
			latestTime = ts
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 Claude transcript 失败: %w", err)
	}
	if latestLine == nil || latestLine.Message == nil || latestLine.Message.Usage == nil {
		return nil, nil
	}
	usage := latestLine.Message.Usage
	contextLength := usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens
	model := strings.TrimSpace(latestLine.Message.Model)
	if model == "" {
		model = strings.TrimSpace(fallbackModel)
	}
	windowSize := resolveContextWindowSize(model, fallbackWindowSize)
	usableTokens := int(float64(windowSize) * usableContextRatio)
	if usableTokens <= 0 {
		usableTokens = int(float64(defaultContextWindowSize) * usableContextRatio)
	}
	usablePercent := float64(contextLength) / float64(usableTokens) * 100
	if usablePercent < 0 {
		usablePercent = 0
	}
	return &ContextMetrics{
		Model:             model,
		ContextWindowSize: windowSize,
		UsableTokens:      usableTokens,
		ContextLength:     contextLength,
		UsablePercent:     usablePercent,
		UpdatedAt:         latestTime,
	}, nil
}

func resolveContextWindowSize(model string, fallback int) int {
	if fallback > 0 {
		return fallback
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return defaultContextWindowSize
	}
	re := regexp.MustCompile(`(?i)(\d+(?:[._,]\d+)?)\s*([mk])(?:\s*(?:token\s*)?context)?`)
	match := re.FindStringSubmatch(model)
	if len(match) == 3 {
		value := strings.ReplaceAll(match[1], ",", "")
		value = strings.ReplaceAll(value, "_", "")
		var multiplier int
		switch strings.ToLower(match[2]) {
		case "m":
			multiplier = 1000000
		case "k":
			multiplier = 1000
		}
		if multiplier > 0 {
			if parsed, err := parseFloatString(value); err == nil {
				return int(parsed * float64(multiplier))
			}
		}
	}
	return defaultContextWindowSize
}

func parseFloatString(text string) (float64, error) {
	var value float64
	_, err := fmt.Sscanf(text, "%f", &value)
	return value, err
}
