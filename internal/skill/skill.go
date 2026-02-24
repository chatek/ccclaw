// Package skill 提供 SKILL L1/L2 记忆层查询
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
