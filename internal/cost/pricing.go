package cost

// OpenAI Image Generation Pricing (USD per image)
// Source: https://openai.com/api/pricing/

type PricingKey struct {
	Model   string
	Size    string
	Quality string
}

var openAIPricing = map[PricingKey]float64{
	// gpt-image-2 pricing — token-based; values below are per-image
	// estimates extrapolated from OpenAI's published 1024x1024 reference
	// (low ~$0.006, medium ~$0.053, high ~$0.211). Actual cost varies with
	// prompt token count.
	{Model: "gpt-image-2", Size: "1024x1024", Quality: "low"}:    0.006,
	{Model: "gpt-image-2", Size: "1024x1024", Quality: "medium"}: 0.053,
	{Model: "gpt-image-2", Size: "1024x1024", Quality: "high"}:   0.211,
	{Model: "gpt-image-2", Size: "1024x1024", Quality: "auto"}:   0.053,

	{Model: "gpt-image-2", Size: "1536x1024", Quality: "low"}:    0.009,
	{Model: "gpt-image-2", Size: "1536x1024", Quality: "medium"}: 0.080,
	{Model: "gpt-image-2", Size: "1536x1024", Quality: "high"}:   0.317,
	{Model: "gpt-image-2", Size: "1536x1024", Quality: "auto"}:   0.080,

	{Model: "gpt-image-2", Size: "1024x1536", Quality: "low"}:    0.009,
	{Model: "gpt-image-2", Size: "1024x1536", Quality: "medium"}: 0.080,
	{Model: "gpt-image-2", Size: "1024x1536", Quality: "high"}:   0.317,
	{Model: "gpt-image-2", Size: "1024x1536", Quality: "auto"}:   0.080,

	{Model: "gpt-image-2", Size: "2048x2048", Quality: "low"}:    0.024,
	{Model: "gpt-image-2", Size: "2048x2048", Quality: "medium"}: 0.212,
	{Model: "gpt-image-2", Size: "2048x2048", Quality: "high"}:   0.844,
	{Model: "gpt-image-2", Size: "2048x2048", Quality: "auto"}:   0.212,

	{Model: "gpt-image-2", Size: "2560x1440", Quality: "low"}:    0.021,
	{Model: "gpt-image-2", Size: "2560x1440", Quality: "medium"}: 0.186,
	{Model: "gpt-image-2", Size: "2560x1440", Quality: "high"}:   0.738,
	{Model: "gpt-image-2", Size: "2560x1440", Quality: "auto"}:   0.186,

	{Model: "gpt-image-2", Size: "auto", Quality: "low"}:    0.006,
	{Model: "gpt-image-2", Size: "auto", Quality: "medium"}: 0.053,
	{Model: "gpt-image-2", Size: "auto", Quality: "high"}:   0.211,
	{Model: "gpt-image-2", Size: "auto", Quality: "auto"}:   0.053,

	// gpt-image-1.5 pricing (20% cheaper than gpt-image-1)
	{Model: "gpt-image-1.5", Size: "1024x1024", Quality: "low"}:    0.009,
	{Model: "gpt-image-1.5", Size: "1024x1024", Quality: "medium"}: 0.034,
	{Model: "gpt-image-1.5", Size: "1024x1024", Quality: "high"}:   0.134,
	{Model: "gpt-image-1.5", Size: "1024x1024", Quality: "auto"}:   0.034,

	{Model: "gpt-image-1.5", Size: "1536x1024", Quality: "low"}:    0.013,
	{Model: "gpt-image-1.5", Size: "1536x1024", Quality: "medium"}: 0.050,
	{Model: "gpt-image-1.5", Size: "1536x1024", Quality: "high"}:   0.200,
	{Model: "gpt-image-1.5", Size: "1536x1024", Quality: "auto"}:   0.050,

	{Model: "gpt-image-1.5", Size: "1024x1536", Quality: "low"}:    0.013,
	{Model: "gpt-image-1.5", Size: "1024x1536", Quality: "medium"}: 0.050,
	{Model: "gpt-image-1.5", Size: "1024x1536", Quality: "high"}:   0.200,
	{Model: "gpt-image-1.5", Size: "1024x1536", Quality: "auto"}:   0.050,

	{Model: "gpt-image-1.5", Size: "auto", Quality: "low"}:    0.009,
	{Model: "gpt-image-1.5", Size: "auto", Quality: "medium"}: 0.034,
	{Model: "gpt-image-1.5", Size: "auto", Quality: "high"}:   0.134,
	{Model: "gpt-image-1.5", Size: "auto", Quality: "auto"}:   0.034,

	// gpt-image-1 pricing
	{Model: "gpt-image-1", Size: "1024x1024", Quality: "low"}:    0.011,
	{Model: "gpt-image-1", Size: "1024x1024", Quality: "medium"}: 0.042,
	{Model: "gpt-image-1", Size: "1024x1024", Quality: "high"}:   0.167,
	{Model: "gpt-image-1", Size: "1024x1024", Quality: "auto"}:   0.042, // default to medium

	{Model: "gpt-image-1", Size: "1536x1024", Quality: "low"}:    0.016,
	{Model: "gpt-image-1", Size: "1536x1024", Quality: "medium"}: 0.063,
	{Model: "gpt-image-1", Size: "1536x1024", Quality: "high"}:   0.250,
	{Model: "gpt-image-1", Size: "1536x1024", Quality: "auto"}:   0.063,

	{Model: "gpt-image-1", Size: "1024x1536", Quality: "low"}:    0.016,
	{Model: "gpt-image-1", Size: "1024x1536", Quality: "medium"}: 0.063,
	{Model: "gpt-image-1", Size: "1024x1536", Quality: "high"}:   0.250,
	{Model: "gpt-image-1", Size: "1024x1536", Quality: "auto"}:   0.063,

	{Model: "gpt-image-1", Size: "auto", Quality: "low"}:    0.011,
	{Model: "gpt-image-1", Size: "auto", Quality: "medium"}: 0.042,
	{Model: "gpt-image-1", Size: "auto", Quality: "high"}:   0.167,
	{Model: "gpt-image-1", Size: "auto", Quality: "auto"}:   0.042,

	// gpt-image-1-mini pricing (budget model, ~50-70% cheaper than gpt-image-1)
	{Model: "gpt-image-1-mini", Size: "1024x1024", Quality: "low"}:    0.005,
	{Model: "gpt-image-1-mini", Size: "1024x1024", Quality: "medium"}: 0.011,
	{Model: "gpt-image-1-mini", Size: "1024x1024", Quality: "high"}:   0.036,
	{Model: "gpt-image-1-mini", Size: "1024x1024", Quality: "auto"}:   0.011,

	{Model: "gpt-image-1-mini", Size: "1536x1024", Quality: "low"}:    0.006,
	{Model: "gpt-image-1-mini", Size: "1536x1024", Quality: "medium"}: 0.015,
	{Model: "gpt-image-1-mini", Size: "1536x1024", Quality: "high"}:   0.052,
	{Model: "gpt-image-1-mini", Size: "1536x1024", Quality: "auto"}:   0.015,

	{Model: "gpt-image-1-mini", Size: "1024x1536", Quality: "low"}:    0.006,
	{Model: "gpt-image-1-mini", Size: "1024x1536", Quality: "medium"}: 0.015,
	{Model: "gpt-image-1-mini", Size: "1024x1536", Quality: "high"}:   0.052,
	{Model: "gpt-image-1-mini", Size: "1024x1536", Quality: "auto"}:   0.015,

	{Model: "gpt-image-1-mini", Size: "auto", Quality: "low"}:    0.005,
	{Model: "gpt-image-1-mini", Size: "auto", Quality: "medium"}: 0.011,
	{Model: "gpt-image-1-mini", Size: "auto", Quality: "high"}:   0.036,
	{Model: "gpt-image-1-mini", Size: "auto", Quality: "auto"}:   0.011,

	// DALL-E 3 pricing
	{Model: "dall-e-3", Size: "1024x1024", Quality: "standard"}: 0.040,
	{Model: "dall-e-3", Size: "1024x1024", Quality: "hd"}:       0.080,
	{Model: "dall-e-3", Size: "1024x1792", Quality: "standard"}: 0.080,
	{Model: "dall-e-3", Size: "1024x1792", Quality: "hd"}:       0.120,
	{Model: "dall-e-3", Size: "1792x1024", Quality: "standard"}: 0.080,
	{Model: "dall-e-3", Size: "1792x1024", Quality: "hd"}:       0.120,

	// DALL-E 2 pricing (no quality option)
	{Model: "dall-e-2", Size: "256x256", Quality: ""}:   0.016,
	{Model: "dall-e-2", Size: "512x512", Quality: ""}:   0.018,
	{Model: "dall-e-2", Size: "1024x1024", Quality: ""}: 0.020,
}

func GetOpenAIPrice(model, size, quality string) (float64, bool) {
	key := PricingKey{Model: model, Size: size, Quality: quality}
	price, ok := openAIPricing[key]
	return price, ok
}

// Video pricing (USD per second)
var videoPricing = map[string]float64{
	"sora-2":     0.10, // $0.10 per second
	"sora-2-pro": 0.30, // $0.30 per second (720p estimate)
}

func GetVideoPricePerSecond(model string) (float64, bool) {
	price, ok := videoPricing[model]
	return price, ok
}
