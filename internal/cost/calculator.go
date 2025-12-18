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
