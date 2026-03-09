package memory

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Document struct {
	Path    string
	Content string
	Score   int
}

type Index struct {
	docs []Document
}

func Build(root string) (*Index, error) {
	index := &Index{}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return index, nil
		}
		return nil, err
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		index.docs = append(index.docs, Document{Path: path, Content: string(data)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return index, nil
}

func (idx *Index) Match(keywords []string, limit int) []Document {
	if idx == nil || len(idx.docs) == 0 || len(keywords) == 0 {
		return nil
	}
	matches := make([]Document, 0)
	for _, doc := range idx.docs {
		score := 0
		joined := strings.ToLower(doc.Path + "\n" + doc.Content)
		for _, keyword := range keywords {
			keyword = strings.TrimSpace(strings.ToLower(keyword))
			if keyword == "" {
				continue
			}
			if strings.Contains(joined, keyword) {
				score++
			}
		}
		if score > 0 {
			doc.Score = score
			matches = append(matches, doc)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].Path < matches[j].Path
		}
		return matches[i].Score > matches[j].Score
	})
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}
