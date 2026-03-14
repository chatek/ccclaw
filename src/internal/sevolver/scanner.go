package sevolver

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	skillPathPattern = regexp.MustCompile(`kb/skills/[A-Za-z0-9._/\-]+\.md`)
	journalDateParts = regexp.MustCompile(`(20\d{2})[./-](\d{2})[./-](\d{2})`)
)

var defaultGapKeywords = []string{
	"失败",
	"无法",
	"报错",
	"error",
	"failed",
	"重试",
	"不会",
	"找不到",
}

type SkillHit struct {
	SkillPath string
	Date      time.Time
	Source    string
}

type GapSignal struct {
	ID                    string
	Date                  time.Time
	Keyword               string
	Context               string
	Source                string
	RelatedSkills         []string
	EscalationStatus      string
	EscalationFingerprint string
	EscalationIssueNumber int
	EscalationIssueURL    string
	EscalationUpdatedAt   string
	EscalationCloseReason string   // 关闭原因子分类
}

func ScanJournal(journalRoot string, since time.Time) ([]SkillHit, error) {
	files, err := collectJournalFiles(journalRoot, since)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	hits := make([]SkillHit, 0)
	for _, file := range files {
		found, err := scanFileForSkillHits(journalRoot, file.Path, file.Day)
		if err != nil {
			return nil, err
		}
		for _, hit := range found {
			key := hit.Date.Format("2006-01-02") + "|" + hit.SkillPath
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			hits = append(hits, hit)
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if !hits[i].Date.Equal(hits[j].Date) {
			return hits[i].Date.Before(hits[j].Date)
		}
		return hits[i].SkillPath < hits[j].SkillPath
	})
	return hits, nil
}

func ScanJournalForGaps(journalRoot, kbDir string, since time.Time) ([]GapSignal, error) {
	files, err := collectJournalFiles(journalRoot, since)
	if err != nil {
		return nil, err
	}
	keywords, err := loadGapKeywords(kbDir)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	gaps := make([]GapSignal, 0)
	for _, file := range files {
		found, err := scanFileForGapSignals(journalRoot, file.Path, file.Day, keywords)
		if err != nil {
			return nil, err
		}
		for _, gap := range found {
			if _, ok := seen[gap.ID]; ok {
				continue
			}
			seen[gap.ID] = struct{}{}
			gaps = append(gaps, gap)
		}
	}
	sort.Slice(gaps, func(i, j int) bool {
		if !gaps[i].Date.Equal(gaps[j].Date) {
			return gaps[i].Date.Before(gaps[j].Date)
		}
		if gaps[i].Keyword != gaps[j].Keyword {
			return gaps[i].Keyword < gaps[j].Keyword
		}
		return gaps[i].ID < gaps[j].ID
	})
	return gaps, nil
}

type journalFile struct {
	Path string
	Day  time.Time
}

func collectJournalFiles(journalRoot string, since time.Time) ([]journalFile, error) {
	root := strings.TrimSpace(journalRoot)
	if root == "" {
		return nil, fmt.Errorf("journal 根目录不能为空")
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 journal 根目录失败: %w", err)
	}
	files := make([]journalFile, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		day := journalFileDate(path)
		if !day.IsZero() && day.Before(dateFloor(since)) {
			return nil
		}
		files = append(files, journalFile{Path: path, Day: day})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描 journal 文件失败: %w", err)
	}
	sort.Slice(files, func(i, j int) bool {
		if !files[i].Day.Equal(files[j].Day) {
			return files[i].Day.Before(files[j].Day)
		}
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func scanFileForSkillHits(journalRoot, path string, day time.Time) ([]SkillHit, error) {
	handle, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 journal 文件失败: %w", err)
	}
	defer handle.Close()

	relative := relativePath(journalRoot, path)
	hits := make([]SkillHit, 0)
	scanner := bufio.NewScanner(handle)
	for scanner.Scan() {
		for _, match := range skillPathPattern.FindAllString(scanner.Text(), -1) {
			hits = append(hits, SkillHit{
				SkillPath: normalizeSkillPath(match),
				Date:      day,
				Source:    relative,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 journal 文件失败: %w", err)
	}
	return hits, nil
}

func scanFileForGapSignals(journalRoot, path string, day time.Time, keywords []string) ([]GapSignal, error) {
	handle, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 journal 文件失败: %w", err)
	}
	defer handle.Close()

	relative := relativePath(journalRoot, path)
	gaps := make([]GapSignal, 0)
	scanner := bufio.NewScanner(handle)
	lineNo := 0
	fileSkills := make([]string, 0)
	fileSkillSeen := map[string]struct{}{}
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineSkills := extractSkillPaths(line)
		for _, skillPath := range lineSkills {
			if _, ok := fileSkillSeen[skillPath]; ok {
				continue
			}
			fileSkillSeen[skillPath] = struct{}{}
			fileSkills = append(fileSkills, skillPath)
		}
		lower := strings.ToLower(line)
		for _, keyword := range keywords {
			if !strings.Contains(lower, strings.ToLower(keyword)) {
				continue
			}
			relatedSkills := lineSkills
			if len(relatedSkills) == 0 {
				relatedSkills = fileSkills
			}
			gaps = append(gaps, GapSignal{
				ID:            buildGapID(day, relative, lineNo, keyword, line),
				Date:          day,
				Keyword:       keyword,
				Context:       line,
				Source:        relative,
				RelatedSkills: append([]string(nil), relatedSkills...),
			})
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 journal 文件失败: %w", err)
	}
	return gaps, nil
}

func extractSkillPaths(text string) []string {
	matches := skillPathPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		skillPath := normalizeSkillPath(match)
		if skillPath == "" {
			continue
		}
		if _, ok := seen[skillPath]; ok {
			continue
		}
		seen[skillPath] = struct{}{}
		paths = append(paths, skillPath)
	}
	sort.Strings(paths)
	return paths
}

func loadGapKeywords(kbDir string) ([]string, error) {
	path := filepath.Join(strings.TrimSpace(kbDir), "assay", "signal-rules.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return append([]string(nil), defaultGapKeywords...), nil
		}
		return nil, fmt.Errorf("读取 signal-rules 失败: %w", err)
	}
	keywords := make([]string, 0)
	seen := map[string]struct{}{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "关键词:") {
			continue
		}
		parts := strings.SplitN(line, "关键词:", 2)
		for _, item := range strings.Split(parts[1], ",") {
			keyword := strings.TrimSpace(strings.TrimPrefix(item, "-"))
			if keyword == "" {
				continue
			}
			key := strings.ToLower(keyword)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			keywords = append(keywords, keyword)
		}
	}
	if len(keywords) == 0 {
		return append([]string(nil), defaultGapKeywords...), nil
	}
	return keywords, nil
}

func journalFileDate(path string) time.Time {
	matches := journalDateParts.FindStringSubmatch(filepath.ToSlash(path))
	if len(matches) != 4 {
		return time.Time{}
	}
	day, err := time.ParseInLocation("2006-01-02", fmt.Sprintf("%s-%s-%s", matches[1], matches[2], matches[3]), time.Local)
	if err != nil {
		return time.Time{}
	}
	return dateFloor(day)
}

func dateFloor(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func normalizeSkillPath(raw string) string {
	raw = filepath.ToSlash(strings.TrimSpace(raw))
	return strings.TrimPrefix(raw, "kb/")
}

func relativePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}
