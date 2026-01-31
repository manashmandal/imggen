package openai

import (
	"fmt"
	"os"

	"github.com/manash/imggen/pkg/models"
)

var supportedReferenceMimes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

func loadReferenceImages(refs []models.ReferenceImage) ([][]byte, []string, error) {
	if len(refs) == 0 {
		return nil, nil, nil
	}

	images := make([][]byte, 0, len(refs))
	mimes := make([]string, 0, len(refs))

	for i, ref := range refs {
		data, err := os.ReadFile(ref.Path)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read reference image %d: %w", i+1, err)
		}
		mimeType := detectMimeType(data)
		if !supportedReferenceMimes[mimeType] {
			return nil, nil, fmt.Errorf("unsupported reference image %d mime type: %s", i+1, mimeType)
		}
		images = append(images, data)
		mimes = append(mimes, mimeType)
	}

	return images, mimes, nil
}
