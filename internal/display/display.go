package display

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/manash/imggen/pkg/models"
)

const defaultTimeout = 60 * time.Second

type Displayer struct {
	out        io.Writer
	httpClient *http.Client
}

func New(out io.Writer) *Displayer {
	return &Displayer{
		out: out,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

func (d *Displayer) Display(ctx context.Context, img *models.GeneratedImage) error {
	data, err := d.getImageData(ctx, img)
	if err != nil {
		return err
	}

	enc := NewKittyEncoder(d.out)
	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("failed to encode image: %w", err)
	}

	fmt.Fprintln(d.out)
	return nil
}

func (d *Displayer) DisplayAll(ctx context.Context, resp *models.Response) error {
	for i, img := range resp.Images {
		if err := d.Display(ctx, &img); err != nil {
			return fmt.Errorf("failed to display image %d: %w", i, err)
		}
	}
	return nil
}

func (d *Displayer) getImageData(ctx context.Context, img *models.GeneratedImage) ([]byte, error) {
	if len(img.Data) > 0 {
		return img.Data, nil
	}

	if img.URL == "" {
		return nil, fmt.Errorf("image has no data or URL")
	}

	return d.downloadImage(ctx, img.URL)
}

func (d *Displayer) downloadImage(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func IsTerminalSupported() bool {
	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	supportedPrograms := []string{"kitty", "ghostty", "iterm.app", "wezterm"}

	for _, prog := range supportedPrograms {
		if termProgram == prog {
			return true
		}
	}

	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return true
	}

	if os.Getenv("ITERM_SESSION_ID") != "" {
		return true
	}

	term := strings.ToLower(os.Getenv("TERM"))
	return strings.Contains(term, "kitty") || strings.Contains(term, "ghostty")
}
