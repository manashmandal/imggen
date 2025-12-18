package cost

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildPriceKey(t *testing.T) {
	// Test buildPriceKey for models with quality
	if got := buildPriceKey("gpt-image-1", "1024x1024", "low"); got != "low-1024-1024" {
		t.Errorf("buildPriceKey() = %v, want low-1024-1024", got)
	}

	// Test buildPriceKey for dall-e-2 (no quality)
	if got := buildPriceKey("dall-e-2", "1024x1024", ""); got != "1024-1024" {
		t.Errorf("buildPriceKey() for dall-e-2 = %v, want 1024-1024", got)
	}

	// Test buildPriceKey for dall-e-3 with quality
	if got := buildPriceKey("dall-e-3", "1024x1024", "standard"); got != "standard-1024-1024" {
		t.Errorf("buildPriceKey() for dall-e-3 = %v, want standard-1024-1024", got)
	}
}

func TestParsePricingKey(t *testing.T) {
	tests := []struct {
		key         string
		wantQuality string
		wantSize    string
	}{
		{"low-1024-1024", "low", "1024x1024"},
		{"medium-1536-1024", "medium", "1536x1024"},
		{"high-1024-1536", "high", "1024x1536"},
		{"standard-1024-1024", "standard", "1024x1024"},
		{"hd-1792-1024", "hd", "1792x1024"},
		{"1024-1024", "", "1024x1024"},
		{"512-512", "", "512x512"},
		{"256-256", "", "256x256"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			quality, size := ParsePricingKey(tt.key)
			if quality != tt.wantQuality {
				t.Errorf("ParsePricingKey() quality = %v, want %v", quality, tt.wantQuality)
			}
			if size != tt.wantSize {
				t.Errorf("ParsePricingKey() size = %v, want %v", size, tt.wantSize)
			}
		})
	}
}

func TestSplitPricingKey(t *testing.T) {
	tests := []struct {
		key  string
		want []string
	}{
		{"low-1024-1024", []string{"low", "1024", "1024"}},
		{"1024-1024", []string{"1024", "1024"}},
		{"medium-1536-1024", []string{"medium", "1536", "1024"}},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := splitPricingKey(tt.key)
			if len(got) != len(tt.want) {
				t.Errorf("splitPricingKey() = %v, want %v", got, tt.want)
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("splitPricingKey()[%d] = %v, want %v", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestNormalizeSizeKey(t *testing.T) {
	tests := []struct {
		size string
		want string
	}{
		{"1024x1024", "1024-1024"},
		{"1536x1024", "1536-1024"},
		{"1024x1536", "1024-1536"},
		{"1792x1024", "1792-1024"},
		{"256x256", "256-256"},
	}

	for _, tt := range tests {
		t.Run(tt.size, func(t *testing.T) {
			if got := normalizeSizeKey(tt.size); got != tt.want {
				t.Errorf("normalizeSizeKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetPrice_Integration(t *testing.T) {
	// This test actually writes to ~/.imggen/pricing.json
	// Clean up after test
	defer func() {
		DeletePricing()
	}()

	err := SetPrice("test-model", "1024x1024", "low", 0.05)
	if err != nil {
		t.Errorf("SetPrice() error = %v", err)
	}

	// Verify the price was saved
	price, ok := GetCachedPrice("test-model", "1024x1024", "low")
	if !ok {
		t.Error("GetCachedPrice() should find the price")
	}
	if price != 0.05 {
		t.Errorf("GetCachedPrice() = %v, want 0.05", price)
	}
}

func TestGetCachedPrice_NotFound(t *testing.T) {
	// Clean up any existing pricing
	DeletePricing()

	price, ok := GetCachedPrice("nonexistent", "1024x1024", "low")
	if ok {
		t.Error("GetCachedPrice() should return false for nonexistent model")
	}
	if price != 0 {
		t.Errorf("GetCachedPrice() = %v, want 0", price)
	}
}

func TestDeletePricing(t *testing.T) {
	// Create a price first
	SetPrice("test-model", "1024x1024", "low", 0.05)

	// Delete
	err := DeletePricing()
	if err != nil {
		t.Errorf("DeletePricing() error = %v", err)
	}

	// Verify it's gone
	pricing, err := LoadPricing()
	if err != nil {
		t.Errorf("LoadPricing() error = %v", err)
	}
	if pricing != nil {
		t.Error("LoadPricing() should return nil after delete")
	}
}

func TestPricingCachePath(t *testing.T) {
	path, err := PricingCachePath()
	if err != nil {
		t.Errorf("PricingCachePath() error = %v", err)
	}

	homeDir, _ := os.UserHomeDir()
	expected := filepath.Join(homeDir, ".imggen", "pricing.json")
	if path != expected {
		t.Errorf("PricingCachePath() = %v, want %v", path, expected)
	}
}

func TestLocalPricing_Structure(t *testing.T) {
	pricing := &LocalPricing{
		UpdatedAt: time.Now(),
		Source:    "manual",
		Image: map[string]map[string]float64{
			"gpt-image-1": {
				"low-1024-1024": 0.011,
			},
		},
	}

	if pricing.Source != "manual" {
		t.Errorf("Source = %v, want manual", pricing.Source)
	}
	if len(pricing.Image) != 1 {
		t.Errorf("Image map length = %d, want 1", len(pricing.Image))
	}
}
