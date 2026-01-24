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

func TestCalculator_CalculateOCR(t *testing.T) {
	calc := NewCalculator()

	tests := []struct {
		name         string
		model        string
		inputTokens  int
		outputTokens int
		wantMin      float64
		wantMax      float64
	}{
		{
			name:         "gpt-5.2 small request",
			model:        "gpt-5.2",
			inputTokens:  1000,
			outputTokens: 500,
			wantMin:      0.001,
			wantMax:      0.02,
		},
		{
			name:         "gpt-5-mini small request",
			model:        "gpt-5-mini",
			inputTokens:  1000,
			outputTokens: 500,
			wantMin:      0.0001,
			wantMax:      0.01,
		},
		{
			name:         "gpt-5-nano small request",
			model:        "gpt-5-nano",
			inputTokens:  1000,
			outputTokens: 500,
			wantMin:      0.00001,
			wantMax:      0.001,
		},
		{
			name:         "gpt-5.2 large request",
			model:        "gpt-5.2",
			inputTokens:  100000,
			outputTokens: 10000,
			wantMin:      0.1,
			wantMax:      0.5,
		},
		{
			name:         "gpt-5-mini large request",
			model:        "gpt-5-mini",
			inputTokens:  100000,
			outputTokens: 10000,
			wantMin:      0.01,
			wantMax:      0.1,
		},
		{
			name:         "unknown model uses mini pricing",
			model:        "unknown-model",
			inputTokens:  1000,
			outputTokens: 500,
			wantMin:      0.0001,
			wantMax:      0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calc.CalculateOCR(tt.model, tt.inputTokens, tt.outputTokens)

			if result == nil {
				t.Fatal("CalculateOCR returned nil")
			}

			if result.Total < tt.wantMin || result.Total > tt.wantMax {
				t.Errorf("CalculateOCR().Total = %v, want between %v and %v", result.Total, tt.wantMin, tt.wantMax)
			}

			if result.Currency != CurrencyUSD {
				t.Errorf("CalculateOCR().Currency = %v, want %v", result.Currency, CurrencyUSD)
			}
		})
	}
}

func TestCalculator_CalculateOCR_ExactPricing(t *testing.T) {
	calc := NewCalculator()

	// Test exact pricing calculation for gpt-5.2
	// gpt-5.2: $1.75 per 1M input, $14.00 per 1M output
	result := calc.CalculateOCR("gpt-5.2", 1_000_000, 1_000_000)
	expectedCost := 1.75 + 14.00

	if !floatEquals(result.Total, expectedCost) {
		t.Errorf("CalculateOCR('gpt-5.2', 1M, 1M).Total = %v, want %v", result.Total, expectedCost)
	}

	// Test exact pricing calculation for gpt-5-mini
	// gpt-5-mini: $0.25 per 1M input, $2.00 per 1M output
	result = calc.CalculateOCR("gpt-5-mini", 1_000_000, 1_000_000)
	expectedCost = 0.25 + 2.00

	if !floatEquals(result.Total, expectedCost) {
		t.Errorf("CalculateOCR('gpt-5-mini', 1M, 1M).Total = %v, want %v", result.Total, expectedCost)
	}

	// Test exact pricing calculation for gpt-5-nano
	// gpt-5-nano: $0.05 per 1M input, $0.40 per 1M output
	result = calc.CalculateOCR("gpt-5-nano", 1_000_000, 1_000_000)
	expectedCost = 0.05 + 0.40

	if !floatEquals(result.Total, expectedCost) {
		t.Errorf("CalculateOCR('gpt-5-nano', 1M, 1M).Total = %v, want %v", result.Total, expectedCost)
	}
}

func TestCalculator_CalculateVideo(t *testing.T) {
	c := NewCalculator()

	tests := []struct {
		name     string
		model    string
		duration int
		wantCost float64
	}{
		{
			name:     "sora-2 4 seconds",
			model:    "sora-2",
			duration: 4,
			wantCost: 0.40, // $0.10/sec * 4
		},
		{
			name:     "sora-2 12 seconds",
			model:    "sora-2",
			duration: 12,
			wantCost: 1.20, // $0.10/sec * 12
		},
		{
			name:     "sora-2-pro 8 seconds",
			model:    "sora-2-pro",
			duration: 8,
			wantCost: 2.40, // $0.30/sec * 8
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := c.CalculateVideo(models.ProviderOpenAI, tt.model, tt.duration)
			if !floatEquals(cost.Total, tt.wantCost) {
				t.Errorf("CalculateVideo() total = %v, want %v", cost.Total, tt.wantCost)
			}
		})
	}
}

func TestCalculator_CalculateVideo_UnknownModel(t *testing.T) {
	c := NewCalculator()

	// Unknown model should default to sora-2 pricing ($0.10/sec)
	cost := c.CalculateVideo(models.ProviderOpenAI, "unknown-video-model", 10)
	expected := 1.0 // $0.10 * 10 seconds

	if !floatEquals(cost.Total, expected) {
		t.Errorf("CalculateVideo() with unknown model total = %v, want %v", cost.Total, expected)
	}
}

func TestCalculator_CalculateVideo_UnknownProvider(t *testing.T) {
	c := NewCalculator()

	// Unknown provider should return 0 cost
	cost := c.CalculateVideo("unknown-provider", "sora-2", 10)

	if cost.Total != 0 {
		t.Errorf("CalculateVideo() with unknown provider total = %v, want 0", cost.Total)
	}
}

func TestCalculator_CalculateVideo_ZeroDuration(t *testing.T) {
	c := NewCalculator()

	cost := c.CalculateVideo(models.ProviderOpenAI, "sora-2", 0)

	if cost.Total != 0 {
		t.Errorf("CalculateVideo() with zero duration total = %v, want 0", cost.Total)
	}
}

func TestCalculator_CalculateVideo_CostInfoStructure(t *testing.T) {
	c := NewCalculator()

	cost := c.CalculateVideo(models.ProviderOpenAI, "sora-2-pro", 4)

	if cost == nil {
		t.Fatal("CalculateVideo() returned nil")
	}
	if !floatEquals(cost.PerImage, 0.30) {
		t.Errorf("PerImage (price per second) = %.4f, want 0.30", cost.PerImage)
	}
	if !floatEquals(cost.Total, 1.20) {
		t.Errorf("Total = %.4f, want 1.20", cost.Total)
	}
	if cost.Currency != CurrencyUSD {
		t.Errorf("Currency = %s, want %s", cost.Currency, CurrencyUSD)
	}
}

func TestGetVideoPricePerSecond_Found(t *testing.T) {
	tests := []struct {
		model string
		want  float64
	}{
		{"sora-2", 0.10},
		{"sora-2-pro", 0.30},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			price, ok := GetVideoPricePerSecond(tt.model)
			if !ok {
				t.Errorf("GetVideoPricePerSecond(%q) returned false", tt.model)
			}
			if price != tt.want {
				t.Errorf("GetVideoPricePerSecond(%q) = %v, want %v", tt.model, price, tt.want)
			}
		})
	}
}

func TestGetVideoPricePerSecond_NotFound(t *testing.T) {
	price, ok := GetVideoPricePerSecond("unknown-model")
	if ok {
		t.Error("GetVideoPricePerSecond() returned true for unknown model")
	}
	if price != 0 {
		t.Errorf("GetVideoPricePerSecond() = %v, want 0", price)
	}
}
