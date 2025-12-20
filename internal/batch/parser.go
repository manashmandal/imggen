package batch

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Item struct {
	Index   int
	Prompt  string
	Model   string
	Size    string
	Quality string
	Style   string
}

type jsonItem struct {
	Prompt  string `json:"prompt"`
	Model   string `json:"model,omitempty"`
	Size    string `json:"size,omitempty"`
	Quality string `json:"quality,omitempty"`
	Style   string `json:"style,omitempty"`
}

func ParseFile(path string) ([]Item, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return ParseJSON(file)
	case ".txt", "":
		return ParseText(file)
	default:
		return nil, fmt.Errorf("unsupported file format %q: use .txt or .json", ext)
	}
}

func ParseText(r io.Reader) ([]Item, error) {
	var items []Item
	scanner := bufio.NewScanner(r)
	index := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		index++
		items = append(items, Item{
			Index:  index,
			Prompt: line,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no prompts found in file")
	}

	return items, nil
}

func ParseJSON(r io.Reader) ([]Item, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var jsonItems []jsonItem
	if err := json.Unmarshal(data, &jsonItems); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if len(jsonItems) == 0 {
		return nil, fmt.Errorf("no prompts found in file")
	}

	items := make([]Item, len(jsonItems))
	for i, ji := range jsonItems {
		if strings.TrimSpace(ji.Prompt) == "" {
			return nil, fmt.Errorf("item %d has empty prompt", i+1)
		}
		items[i] = Item{
			Index:   i + 1,
			Prompt:  ji.Prompt,
			Model:   ji.Model,
			Size:    ji.Size,
			Quality: ji.Quality,
			Style:   ji.Style,
		}
	}

	return items, nil
}
