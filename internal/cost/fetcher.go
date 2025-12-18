package cost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// PricingSource is the URL where pricing info comes from
	PricingSource = "https://openai.com/api/pricing"
)

// LocalPricing represents locally stored pricing with metadata
type LocalPricing struct {
	UpdatedAt time.Time                     `json:"updated_at"`
	Source    string                        `json:"source"`
	Image     map[string]map[string]float64 `json:"image"`
}

// SavePricing saves pricing data to the local cache file
func SavePricing(pricing *LocalPricing) error {
	path, err := pricingCachePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	data, err := json.MarshalIndent(pricing, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal pricing: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write pricing cache: %w", err)
	}

	return nil
}

// LoadPricing loads pricing data from the local cache file
func LoadPricing() (*LocalPricing, error) {
	path, err := pricingCachePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No cache exists
		}
		return nil, fmt.Errorf("failed to read pricing cache: %w", err)
	}

	var pricing LocalPricing
	if err := json.Unmarshal(data, &pricing); err != nil {
		return nil, fmt.Errorf("failed to parse pricing cache: %w", err)
	}

	return &pricing, nil
}

// DeletePricing removes the local cache file
func DeletePricing() error {
	path, err := pricingCachePath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete pricing cache: %w", err)
	}
	return nil
}

// pricingCachePath returns the path to the pricing cache file
func pricingCachePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".imggen", "pricing.json"), nil
}

// PricingCachePath returns the path to the pricing cache file (exported for testing)
func PricingCachePath() (string, error) {
	return pricingCachePath()
}

// SetPrice sets a single price in the local cache
func SetPrice(model, size, quality string, price float64) error {
	pricing, err := LoadPricing()
	if err != nil {
		return err
	}

	if pricing == nil {
		pricing = &LocalPricing{
			Source: "manual",
			Image:  make(map[string]map[string]float64),
		}
	}

	if pricing.Image == nil {
		pricing.Image = make(map[string]map[string]float64)
	}

	if pricing.Image[model] == nil {
		pricing.Image[model] = make(map[string]float64)
	}

	// Build the key
	key := buildPriceKey(model, size, quality)
	pricing.Image[model][key] = price
	pricing.UpdatedAt = time.Now()
	pricing.Source = "manual"

	return SavePricing(pricing)
}

// buildPriceKey builds a price lookup key from size and quality
func buildPriceKey(model, size, quality string) string {
	normalizedSize := normalizeSizeKey(size)
	if model == "dall-e-2" {
		return normalizedSize
	}
	return quality + "-" + normalizedSize
}

// ParsePricingKey parses a pricing key like "low-1024-1024" into quality and size
func ParsePricingKey(key string) (quality, size string) {
	// Keys are in format: "quality-width-height" or "width-height" (for dall-e-2)
	// Examples: "low-1024-1024", "medium-1536-1024", "1024-1024"
	parts := splitPricingKey(key)
	if len(parts) == 3 {
		return parts[0], parts[1] + "x" + parts[2]
	} else if len(parts) == 2 {
		return "", parts[0] + "x" + parts[1]
	}
	return "", ""
}

func splitPricingKey(key string) []string {
	var parts []string
	current := ""
	for _, c := range key {
		if c == '-' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// GetCachedPrice looks up a price from the local cache
func GetCachedPrice(model, size, quality string) (float64, bool) {
	pricing, err := LoadPricing()
	if err != nil || pricing == nil {
		return 0, false
	}

	modelPricing, ok := pricing.Image[model]
	if !ok {
		return 0, false
	}

	key := buildPriceKey(model, size, quality)
	price, ok := modelPricing[key]
	return price, ok
}

// normalizeSizeKey converts "1024x1024" to "1024-1024"
func normalizeSizeKey(size string) string {
	result := ""
	for _, c := range size {
		if c == 'x' {
			result += "-"
		} else {
			result += string(c)
		}
	}
	return result
}
