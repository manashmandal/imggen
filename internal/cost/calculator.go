package cost

import "github.com/manash/imggen/pkg/models"

const (
	CurrencyUSD = "USD"
)

type Calculator struct{}

func NewCalculator() *Calculator {
	return &Calculator{}
}

func (c *Calculator) Calculate(provider models.ProviderType, model, size, quality string, count int) *models.CostInfo {
	var perImage float64

	switch provider {
	case models.ProviderOpenAI:
		perImage = c.calculateOpenAI(model, size, quality)
	case models.ProviderStability:
		perImage = c.calculateStability(model, size, quality)
	default:
		perImage = 0
	}

	return &models.CostInfo{
		PerImage: perImage,
		Total:    perImage * float64(count),
		Currency: CurrencyUSD,
	}
}

func (c *Calculator) calculateOpenAI(model, size, quality string) float64 {
	price, ok := GetOpenAIPrice(model, size, quality)
	if ok {
		return price
	}

	// Fallback: try without quality for DALL-E 2
	if model == "dall-e-2" {
		price, ok = GetOpenAIPrice(model, size, "")
		if ok {
			return price
		}
	}

	// Default fallback prices
	switch model {
	case "gpt-image-1":
		return 0.042 // medium quality default
	case "dall-e-3":
		return 0.040 // standard quality default
	case "dall-e-2":
		return 0.020 // 1024x1024 default
	default:
		return 0
	}
}

func (c *Calculator) calculateStability(model, size, quality string) float64 {
	// TODO: Add Stability AI pricing when integrated
	return 0
}

// CalculateVideo calculates the cost for video generation
func (c *Calculator) CalculateVideo(provider models.ProviderType, model string, durationSeconds int) *models.CostInfo {
	var pricePerSecond float64

	switch provider {
	case models.ProviderOpenAI:
		price, ok := GetVideoPricePerSecond(model)
		if ok {
			pricePerSecond = price
		} else {
			pricePerSecond = 0.10 // default to sora-2 pricing
		}
	default:
		pricePerSecond = 0
	}

	total := pricePerSecond * float64(durationSeconds)

	return &models.CostInfo{
		PerImage: pricePerSecond, // Per-second cost stored here
		Total:    total,
		Currency: CurrencyUSD,
	}
}

// CalculateOCR calculates the cost for OCR operations based on token usage
// Pricing is per 1M tokens for GPT-5 series models
func (c *Calculator) CalculateOCR(model string, inputTokens, outputTokens int) *models.CostInfo {
	var inputCostPer1M, outputCostPer1M float64

	switch model {
	case "gpt-5.2":
		inputCostPer1M = 1.75   // $1.75 per 1M input tokens
		outputCostPer1M = 14.00 // $14.00 per 1M output tokens
	case "gpt-5-mini":
		inputCostPer1M = 0.25 // $0.25 per 1M input tokens
		outputCostPer1M = 2.00 // $2.00 per 1M output tokens
	case "gpt-5-nano":
		inputCostPer1M = 0.05 // $0.05 per 1M input tokens
		outputCostPer1M = 0.40 // $0.40 per 1M output tokens
	default:
		// Default to gpt-5-mini pricing
		inputCostPer1M = 0.25
		outputCostPer1M = 2.00
	}

	inputCost := (float64(inputTokens) / 1_000_000) * inputCostPer1M
	outputCost := (float64(outputTokens) / 1_000_000) * outputCostPer1M
	total := inputCost + outputCost

	return &models.CostInfo{
		PerImage: total, // For OCR, we use PerImage as per-request
		Total:    total,
		Currency: CurrencyUSD,
	}
}
