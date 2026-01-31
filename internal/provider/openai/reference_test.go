package openai

import (
	"os"
	"testing"

	"github.com/manash/imggen/pkg/models"
)

func TestLoadReferenceImages(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "ref-*.png")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write([]byte{0x89, 0x50, 0x4E, 0x47}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	images, mimes, err := loadReferenceImages([]models.ReferenceImage{{Path: tmpFile.Name()}})
	if err != nil {
		t.Fatalf("loadReferenceImages() error = %v", err)
	}
	if len(images) != 1 || len(mimes) != 1 {
		t.Fatalf("expected 1 image and mime, got %d/%d", len(images), len(mimes))
	}
}

func TestLoadReferenceImages_InvalidMime(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "ref-*.pdf")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write([]byte("%PDF-1.7")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	_, _, err = loadReferenceImages([]models.ReferenceImage{{Path: tmpFile.Name()}})
	if err == nil {
		t.Fatal("expected error for unsupported mime type")
	}
}

func TestLoadReferenceImages_MissingFile(t *testing.T) {
	_, _, err := loadReferenceImages([]models.ReferenceImage{{Path: "/nonexistent/file.png"}})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
