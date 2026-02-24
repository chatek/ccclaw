// Package skill 提供 SKILL L1/L2 记忆层查询及 docs/ 记忆注入
package skill

import (
	"os"
	"path/filepath"
	"strings"
)

// Skill 技能定义
type Skill struct {
	Name    string
	Level   string // L1 or L2
	Content string
	Path    string
}

// Index SKILL 索引
type Index struct {
	dir string
}

// NewIndex 创建索引，dir 为 skills/ 目录
func NewIndex(dir string) *Index {
	return &Index{dir: dir}
}

// Match 根据关键词匹配相关 skill，返回命中的 skill 列表
func (idx *Index) Match(keywords []string) ([]Skill, error) {
	var matched []Skill
	for _, level := range []string{"L1", "L2"} {
		levelDir := filepath.Join(idx.dir, level)
		entries, err := os.ReadDir(levelDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			path := filepath.Join(levelDir, e.Name())
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			lower := strings.ToLower(string(content))
			for _, kw := range keywords {
				if strings.Contains(lower, strings.ToLower(kw)) {
					matched = append(matched, Skill{
						Name:    strings.TrimSuffix(e.Name(), ".md"),
						Level:   level,
						Content: string(content),
						Path:    path,
					})
					break
				}
			}
		}
	}
	return matched, nil
}

// DocMemory docs/ 目录中匹配到的文档片段
type DocMemory struct {
	SubDir  string // designs / plans / assay / reports
	Name    string
	Content string
}

// MatchDocs 在 docs/ 目录下按关键词匹配相关文档（Q3=B：关键词匹配注入）
// docsDir 为仓库根下的 docs/ 路径
func MatchDocs(docsDir string, keywords []string) ([]DocMemory, error) {
	var matched []DocMemory
	subDirs := []string{"designs", "plans", "assay", "reports"}
	for _, sub := range subDirs {
		dir := filepath.Join(docsDir, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			lower := strings.ToLower(string(content))
			for _, kw := range keywords {
				if strings.Contains(lower, strings.ToLower(kw)) {
					matched = append(matched, DocMemory{
						SubDir:  sub,
						Name:    strings.TrimSuffix(e.Name(), ".md"),
						Content: string(content),
					})
					break
				}
			}
		}
	}
	return matched, nil
}
