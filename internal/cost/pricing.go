package cost

// OpenAI Image Generation Pricing (USD per image)
// Source: https://openai.com/api/pricing/

type PricingKey struct {
	Model   string
	Size    string
	Quality string
}

var openAIPricing = map[PricingKey]float64{
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
