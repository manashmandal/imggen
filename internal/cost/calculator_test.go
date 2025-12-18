package cost

import (
	"testing"

	"github.com/manash/imggen/pkg/models"
)

func TestCalculator_Calculate_OpenAI_GPTImage1(t *testing.T) {
	calc := NewCalculator()

	tests := []struct {
		name     string
		size     string
		quality  string
		count    int
		expected float64
	}{
		{"1024x1024 low", "1024x1024", "low", 1, 0.011},
		{"1024x1024 medium", "1024x1024", "medium", 1, 0.042},
		{"1024x1024 high", "1024x1024", "high", 1, 0.167},
		{"1024x1024 auto", "1024x1024", "auto", 1, 0.042},
		{"1536x1024 low", "1536x1024", "low", 1, 0.016},
		{"1536x1024 medium", "1536x1024", "medium", 1, 0.063},
		{"1536x1024 high", "1536x1024", "high", 1, 0.250},
		{"multiple images", "1024x1024", "low", 3, 0.033},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calc.Calculate(models.ProviderOpenAI, "gpt-image-1", tt.size, tt.quality, tt.count)
			if result.Total != tt.expected {
				t.Errorf("expected total %.4f, got %.4f", tt.expected, result.Total)
			}
			if result.Currency != CurrencyUSD {
				t.Errorf("expected currency %s, got %s", CurrencyUSD, result.Currency)
			}
		})
	}
}

func TestCalculator_Calculate_OpenAI_DallE3(t *testing.T) {
	calc := NewCalculator()

	tests := []struct {
		name     string
		size     string
		quality  string
		expected float64
	}{
		{"1024x1024 standard", "1024x1024", "standard", 0.040},
		{"1024x1024 hd", "1024x1024", "hd", 0.080},
		{"1024x1792 standard", "1024x1792", "standard", 0.080},
		{"1024x1792 hd", "1024x1792", "hd", 0.120},
		{"1792x1024 standard", "1792x1024", "standard", 0.080},
		{"1792x1024 hd", "1792x1024", "hd", 0.120},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calc.Calculate(models.ProviderOpenAI, "dall-e-3", tt.size, tt.quality, 1)
			if result.Total != tt.expected {
				t.Errorf("expected total %.4f, got %.4f", tt.expected, result.Total)
			}
		})
	}
}

func TestCalculator_Calculate_OpenAI_DallE2(t *testing.T) {
	calc := NewCalculator()

	tests := []struct {
		name     string
		size     string
		expected float64
	}{
		{"256x256", "256x256", 0.016},
		{"512x512", "512x512", 0.018},
		{"1024x1024", "1024x1024", 0.020},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calc.Calculate(models.ProviderOpenAI, "dall-e-2", tt.size, "", 1)
			if result.Total != tt.expected {
				t.Errorf("expected total %.4f, got %.4f", tt.expected, result.Total)
			}
		})
	}
}

func TestCalculator_Calculate_Fallback(t *testing.T) {
	calc := NewCalculator()

	// Unknown combination should use fallback
	result := calc.Calculate(models.ProviderOpenAI, "gpt-image-1", "unknown-size", "unknown-quality", 1)
	if result.Total != 0.042 { // default fallback for gpt-image-1
		t.Errorf("expected fallback total 0.042, got %.4f", result.Total)
	}
}

func TestCalculator_Calculate_UnknownProvider(t *testing.T) {
	calc := NewCalculator()

	result := calc.Calculate("unknown-provider", "model", "size", "quality", 1)
	if result.Total != 0 {
		t.Errorf("expected 0 for unknown provider, got %.4f", result.Total)
	}
}

func TestCalculator_PerImageCalculation(t *testing.T) {
	calc := NewCalculator()

	result := calc.Calculate(models.ProviderOpenAI, "gpt-image-1", "1024x1024", "low", 5)
	if result.PerImage != 0.011 {
		t.Errorf("expected per image 0.011, got %.4f", result.PerImage)
	}
	expectedTotal := 0.055
	if !floatEquals(result.Total, expectedTotal) {
		t.Errorf("expected total %.4f, got %.4f", expectedTotal, result.Total)
	}
}

func floatEquals(a, b float64) bool {
	const epsilon = 0.0001
	return (a-b) < epsilon && (b-a) < epsilon
}
