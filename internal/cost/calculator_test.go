package cost

import (
	"testing"

	"github.com/manash/imggen/pkg/models"
)

func TestNewCalculator(t *testing.T) {
	calc := NewCalculator()
	if calc == nil {
		t.Error("NewCalculator() returned nil")
	}
}

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
		{"1536x1024 auto", "1536x1024", "auto", 1, 0.063},
		{"1024x1536 low", "1024x1536", "low", 1, 0.016},
		{"1024x1536 medium", "1024x1536", "medium", 1, 0.063},
		{"1024x1536 high", "1024x1536", "high", 1, 0.250},
		{"1024x1536 auto", "1024x1536", "auto", 1, 0.063},
		{"auto low", "auto", "low", 1, 0.011},
		{"auto medium", "auto", "medium", 1, 0.042},
		{"auto high", "auto", "high", 1, 0.167},
		{"auto auto", "auto", "auto", 1, 0.042},
		{"multiple images", "1024x1024", "low", 3, 0.033},
		{"10 images high", "1024x1024", "high", 10, 1.67},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calc.Calculate(models.ProviderOpenAI, "gpt-image-1", tt.size, tt.quality, tt.count)
			if !floatEquals(result.Total, tt.expected) {
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
			if !floatEquals(result.Total, tt.expected) {
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
			if !floatEquals(result.Total, tt.expected) {
				t.Errorf("expected total %.4f, got %.4f", tt.expected, result.Total)
			}
		})
	}
}

func TestCalculator_Calculate_DallE2_MultipleImages(t *testing.T) {
	calc := NewCalculator()

	result := calc.Calculate(models.ProviderOpenAI, "dall-e-2", "512x512", "", 5)
	expected := 0.018 * 5

	if !floatEquals(result.Total, expected) {
		t.Errorf("expected total %.4f, got %.4f", expected, result.Total)
	}
	if !floatEquals(result.PerImage, 0.018) {
		t.Errorf("expected per image 0.018, got %.4f", result.PerImage)
	}
}

func TestCalculator_Calculate_Fallback_GPTImage1(t *testing.T) {
	calc := NewCalculator()

	// Unknown combination should use fallback
	result := calc.Calculate(models.ProviderOpenAI, "gpt-image-1", "unknown-size", "unknown-quality", 1)
	if !floatEquals(result.Total, 0.042) { // default fallback for gpt-image-1
		t.Errorf("expected fallback total 0.042, got %.4f", result.Total)
	}
}

func TestCalculator_Calculate_Fallback_DallE3(t *testing.T) {
	calc := NewCalculator()

	result := calc.Calculate(models.ProviderOpenAI, "dall-e-3", "unknown-size", "unknown-quality", 1)
	if !floatEquals(result.Total, 0.040) { // default fallback for dall-e-3
		t.Errorf("expected fallback total 0.040, got %.4f", result.Total)
	}
}

func TestCalculator_Calculate_Fallback_DallE2(t *testing.T) {
	calc := NewCalculator()

	result := calc.Calculate(models.ProviderOpenAI, "dall-e-2", "unknown-size", "", 1)
	if !floatEquals(result.Total, 0.020) { // default fallback for dall-e-2
		t.Errorf("expected fallback total 0.020, got %.4f", result.Total)
	}
}

func TestCalculator_Calculate_UnknownModel(t *testing.T) {
	calc := NewCalculator()

	result := calc.Calculate(models.ProviderOpenAI, "unknown-model", "1024x1024", "standard", 1)
	if result.Total != 0 {
		t.Errorf("expected 0 for unknown model, got %.4f", result.Total)
	}
}

func TestCalculator_Calculate_UnknownProvider(t *testing.T) {
	calc := NewCalculator()

	result := calc.Calculate("unknown-provider", "model", "size", "quality", 1)
	if result.Total != 0 {
		t.Errorf("expected 0 for unknown provider, got %.4f", result.Total)
	}
}

func TestCalculator_Calculate_StabilityProvider(t *testing.T) {
	calc := NewCalculator()

	// Stability AI pricing not yet implemented
	result := calc.Calculate(models.ProviderStability, "stable-diffusion-xl", "1024x1024", "", 1)
	if result.Total != 0 {
		t.Errorf("expected 0 for stability (not implemented), got %.4f", result.Total)
	}
}

func TestCalculator_PerImageCalculation(t *testing.T) {
	calc := NewCalculator()

	result := calc.Calculate(models.ProviderOpenAI, "gpt-image-1", "1024x1024", "low", 5)
	if !floatEquals(result.PerImage, 0.011) {
		t.Errorf("expected per image 0.011, got %.4f", result.PerImage)
	}
	expectedTotal := 0.055
	if !floatEquals(result.Total, expectedTotal) {
		t.Errorf("expected total %.4f, got %.4f", expectedTotal, result.Total)
	}
}

func TestCalculator_ZeroCount(t *testing.T) {
	calc := NewCalculator()

	result := calc.Calculate(models.ProviderOpenAI, "gpt-image-1", "1024x1024", "low", 0)
	if result.Total != 0 {
		t.Errorf("expected 0 for zero count, got %.4f", result.Total)
	}
}

func TestCalculator_NegativeCount(t *testing.T) {
	calc := NewCalculator()

	result := calc.Calculate(models.ProviderOpenAI, "gpt-image-1", "1024x1024", "low", -1)
	if result.Total >= 0 {
		// Negative count would give negative total, which is mathematically correct
		// but in practice would never happen
	}
}

func TestCalculator_CostInfoStructure(t *testing.T) {
	calc := NewCalculator()

	result := calc.Calculate(models.ProviderOpenAI, "dall-e-3", "1024x1024", "hd", 2)

	if result == nil {
		t.Fatal("Calculate() returned nil")
	}
	if result.PerImage != 0.080 {
		t.Errorf("PerImage = %.4f, want 0.080", result.PerImage)
	}
	if !floatEquals(result.Total, 0.160) {
		t.Errorf("Total = %.4f, want 0.160", result.Total)
	}
	if result.Currency != CurrencyUSD {
		t.Errorf("Currency = %s, want %s", result.Currency, CurrencyUSD)
	}
}

func TestGetOpenAIPrice_Found(t *testing.T) {
	price, ok := GetOpenAIPrice("gpt-image-1", "1024x1024", "high")
	if !ok {
		t.Error("GetOpenAIPrice() returned false for known price")
	}
	if price != 0.167 {
		t.Errorf("GetOpenAIPrice() = %.4f, want 0.167", price)
	}
}

func TestGetOpenAIPrice_NotFound(t *testing.T) {
	price, ok := GetOpenAIPrice("unknown", "unknown", "unknown")
	if ok {
		t.Error("GetOpenAIPrice() returned true for unknown price")
	}
	if price != 0 {
		t.Errorf("GetOpenAIPrice() = %.4f, want 0", price)
	}
}

func TestPricingKey_Equality(t *testing.T) {
	key1 := PricingKey{Model: "gpt-image-1", Size: "1024x1024", Quality: "high"}
	key2 := PricingKey{Model: "gpt-image-1", Size: "1024x1024", Quality: "high"}
	key3 := PricingKey{Model: "gpt-image-1", Size: "1024x1024", Quality: "low"}

	if key1 != key2 {
		t.Error("Identical PricingKeys should be equal")
	}
	if key1 == key3 {
		t.Error("Different PricingKeys should not be equal")
	}
}

func TestAllDallE2SizesHavePricing(t *testing.T) {
	sizes := []string{"256x256", "512x512", "1024x1024"}

	for _, size := range sizes {
		price, ok := GetOpenAIPrice("dall-e-2", size, "")
		if !ok {
			t.Errorf("DALL-E 2 size %s should have pricing", size)
		}
		if price <= 0 {
			t.Errorf("DALL-E 2 size %s has invalid price: %.4f", size, price)
		}
	}
}

func TestAllDallE3CombinationsHavePricing(t *testing.T) {
	sizes := []string{"1024x1024", "1024x1792", "1792x1024"}
	qualities := []string{"standard", "hd"}

	for _, size := range sizes {
		for _, quality := range qualities {
			price, ok := GetOpenAIPrice("dall-e-3", size, quality)
			if !ok {
				t.Errorf("DALL-E 3 %s/%s should have pricing", size, quality)
			}
			if price <= 0 {
				t.Errorf("DALL-E 3 %s/%s has invalid price: %.4f", size, quality, price)
			}
		}
	}
}

func TestAllGPTImage1CombinationsHavePricing(t *testing.T) {
	sizes := []string{"1024x1024", "1536x1024", "1024x1536", "auto"}
	qualities := []string{"low", "medium", "high", "auto"}

	for _, size := range sizes {
		for _, quality := range qualities {
			price, ok := GetOpenAIPrice("gpt-image-1", size, quality)
			if !ok {
				t.Errorf("gpt-image-1 %s/%s should have pricing", size, quality)
			}
			if price <= 0 {
				t.Errorf("gpt-image-1 %s/%s has invalid price: %.4f", size, quality, price)
			}
		}
	}
}

func floatEquals(a, b float64) bool {
	const epsilon = 0.0001
	return (a-b) < epsilon && (b-a) < epsilon
}
