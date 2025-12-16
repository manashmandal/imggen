package image

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/manash/imggen/pkg/models"
)

type Saver struct {
	httpClient *http.Client
}

func NewSaver() *Saver {
	return &Saver{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (s *Saver) Save(ctx context.Context, img *models.GeneratedImage, path string) error {
	var data []byte
	var err error

	if len(img.Data) > 0 {
		data = img.Data
	} else if img.URL != "" {
		data, err = s.downloadFromURL(ctx, img.URL)
		if err != nil {
			return fmt.Errorf("failed to download image: %w", err)
		}
	} else {
		return fmt.Errorf("no image data available")
	}

	if err := s.ensureDir(path); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	img.Filename = path
	return nil
}

func (s *Saver) SaveAll(ctx context.Context, resp *models.Response, basePath string, format models.OutputFormat) ([]string, error) {
	paths := make([]string, 0, len(resp.Images))

	for i := range resp.Images {
		path := s.generatePath(basePath, i, len(resp.Images), format)
		if err := s.Save(ctx, &resp.Images[i], path); err != nil {
			return paths, fmt.Errorf("failed to save image %d: %w", i+1, err)
		}
		paths = append(paths, path)
	}

	return paths, nil
}

func (s *Saver) downloadFromURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (s *Saver) ensureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}

func (s *Saver) generatePath(basePath string, index, total int, format models.OutputFormat) string {
	if basePath != "" {
		if total == 1 {
			return basePath
		}
		ext := filepath.Ext(basePath)
		base := basePath[:len(basePath)-len(ext)]
		return fmt.Sprintf("%s-%d%s", base, index+1, ext)
	}
	return GenerateFilename(index, format)
}

func GenerateFilename(index int, format models.OutputFormat) string {
	timestamp := time.Now().Format("20060102-150405")
	if index > 0 {
		return fmt.Sprintf("image-%s-%d.%s", timestamp, index+1, format)
	}
	return fmt.Sprintf("image-%s.%s", timestamp, format)
}

func GenerateFilenameWithTime(index int, format models.OutputFormat, t time.Time) string {
	timestamp := t.Format("20060102-150405")
	if index > 0 {
		return fmt.Sprintf("image-%s-%d.%s", timestamp, index+1, format)
	}
	return fmt.Sprintf("image-%s.%s", timestamp, format)
}
