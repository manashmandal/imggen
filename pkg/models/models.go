package models

import (
	"errors"
	"fmt"
	"slices"
)

var (
	ErrEmptyPrompt               = errors.New("prompt cannot be empty")
	ErrInvalidCount              = errors.New("count must be at least 1")
	ErrCountExceedsMax           = errors.New("count exceeds maximum for model")
	ErrInvalidSize               = errors.New("invalid size for model")
	ErrInvalidQuality            = errors.New("invalid quality for model")
	ErrStyleNotSupported         = errors.New("style not supported by model")
	ErrTransparencyNotSupported  = errors.New("transparency not supported by model")
	ErrInvalidTransparencyFormat = errors.New("transparent background requires png or webp format")
	ErrEditNotSupported          = errors.New("image editing not supported by model")
	ErrNoImageData               = errors.New("image data is required for editing")
	ErrInvalidDuration           = errors.New("invalid duration for model")
)

type ProviderType string

const (
	ProviderOpenAI    ProviderType = "openai"
	ProviderStability ProviderType = "stability"
)

type OutputFormat string

const (
	FormatPNG  OutputFormat = "png"
	FormatJPEG OutputFormat = "jpeg"
	FormatWebP OutputFormat = "webp"
)

func ValidFormats() []OutputFormat {
	return []OutputFormat{FormatPNG, FormatJPEG, FormatWebP}
}

func (f OutputFormat) IsValid() bool {
	return slices.Contains(ValidFormats(), f)
}

func (f OutputFormat) String() string {
	return string(f)
}

type VideoFormat string

const (
	FormatMP4 VideoFormat = "mp4"
)

type Request struct {
	Prompt      string
	Model       string
	Size        string
	Quality     string
	Count       int
	Style       string
	Format      OutputFormat
	Transparent bool
}

func NewRequest(prompt string) *Request {
	return &Request{
		Prompt: prompt,
		Count:  1,
		Format: FormatPNG,
	}
}

type EditRequest struct {
	Image  []byte
	Mask   []byte
	Prompt string
	Model  string
	Size   string
	Count  int
	Format OutputFormat
}

func NewEditRequest(image []byte, prompt string) *EditRequest {
	return &EditRequest{
		Image:  image,
		Prompt: prompt,
		Count:  1,
		Format: FormatPNG,
	}
}

func (r *EditRequest) Validate() error {
	if len(r.Image) == 0 {
		return ErrNoImageData
	}
	if r.Prompt == "" {
		return ErrEmptyPrompt
	}
	return nil
}

type CostInfo struct {
	PerImage float64 `json:"per_image"`
	Total    float64 `json:"total"`
	Currency string  `json:"currency"`
}

type Response struct {
	Images        []GeneratedImage
	RevisedPrompt string
	Cost          *CostInfo
}

type GeneratedImage struct {
	Data     []byte
	URL      string
	Base64   string
	Index    int
	Filename string
}

// VideoRequest represents a request for video generation
type VideoRequest struct {
	Prompt   string
	Model    string
	Duration int
	Size     string
}

// NewVideoRequest creates a new VideoRequest with default values
func NewVideoRequest(prompt string) *VideoRequest {
	return &VideoRequest{
		Prompt: prompt,
	}
}

// VideoResponse represents the response from video generation
type VideoResponse struct {
	Videos []GeneratedVideo
	Cost   *CostInfo
}

// GeneratedVideo represents a single generated video
type GeneratedVideo struct {
	Data     []byte
	URL      string
	Filename string
}

type ModelCapabilities struct {
	Name                 string
	Provider             ProviderType
	SupportedSizes       []string
	SupportedQualities   []string
	MaxImages            int
	DefaultSize          string
	DefaultQuality       string
	SupportsStyle        bool
	SupportsTransparency bool
	SupportsEdit         bool
	StyleOptions         []string
}

func (c *ModelCapabilities) Validate(req *Request) error {
	if req.Prompt == "" {
		return ErrEmptyPrompt
	}

	if req.Count < 1 {
		return ErrInvalidCount
	}

	if req.Count > c.MaxImages {
		return fmt.Errorf("%w: max %d, got %d", ErrCountExceedsMax, c.MaxImages, req.Count)
	}

	if req.Size != "" && !slices.Contains(c.SupportedSizes, req.Size) {
		return fmt.Errorf("%w: %q not in %v", ErrInvalidSize, req.Size, c.SupportedSizes)
	}

	if req.Quality != "" && len(c.SupportedQualities) > 0 && !slices.Contains(c.SupportedQualities, req.Quality) {
		return fmt.Errorf("%w: %q not in %v", ErrInvalidQuality, req.Quality, c.SupportedQualities)
	}

	if req.Style != "" && !c.SupportsStyle {
		return ErrStyleNotSupported
	}

	if req.Style != "" && c.SupportsStyle && len(c.StyleOptions) > 0 && !slices.Contains(c.StyleOptions, req.Style) {
		return fmt.Errorf("%w: %q not in %v", ErrStyleNotSupported, req.Style, c.StyleOptions)
	}

	if req.Transparent && !c.SupportsTransparency {
		return ErrTransparencyNotSupported
	}

	if req.Transparent && req.Format != FormatPNG && req.Format != FormatWebP {
		return ErrInvalidTransparencyFormat
	}

	return nil
}

func (c *ModelCapabilities) ApplyDefaults(req *Request) {
	if req.Size == "" {
		req.Size = c.DefaultSize
	}
	if req.Quality == "" && c.DefaultQuality != "" {
		req.Quality = c.DefaultQuality
	}
	if req.Model == "" {
		req.Model = c.Name
	}
}

// VideoModelCapabilities defines the capabilities of a video generation model
type VideoModelCapabilities struct {
	Name               string
	Provider           ProviderType
	SupportedDurations []int
	SupportedSizes     []string
	DefaultDuration    int
	DefaultSize        string
}

// Validate checks if a VideoRequest is valid for this model's capabilities
func (c *VideoModelCapabilities) Validate(req *VideoRequest) error {
	if req.Prompt == "" {
		return ErrEmptyPrompt
	}

	if req.Duration != 0 && len(c.SupportedDurations) > 0 && !slices.Contains(c.SupportedDurations, req.Duration) {
		return fmt.Errorf("%w: %d not in %v", ErrInvalidDuration, req.Duration, c.SupportedDurations)
	}

	if req.Size != "" && len(c.SupportedSizes) > 0 && !slices.Contains(c.SupportedSizes, req.Size) {
		return fmt.Errorf("%w: %q not in %v", ErrInvalidSize, req.Size, c.SupportedSizes)
	}

	return nil
}

// ApplyDefaults sets default values for unspecified fields in VideoRequest
func (c *VideoModelCapabilities) ApplyDefaults(req *VideoRequest) {
	if req.Duration == 0 {
		req.Duration = c.DefaultDuration
	}
	if req.Size == "" {
		req.Size = c.DefaultSize
	}
	if req.Model == "" {
		req.Model = c.Name
	}
}

type ModelRegistry struct {
	models      map[string]*ModelCapabilities
	ocrModels   map[string]*OCRModelCapabilities
	videoModels map[string]*VideoModelCapabilities
}

func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models:      make(map[string]*ModelCapabilities),
		ocrModels:   make(map[string]*OCRModelCapabilities),
		videoModels: make(map[string]*VideoModelCapabilities),
	}
}

func (r *ModelRegistry) Register(cap *ModelCapabilities) {
	r.models[cap.Name] = cap
}

func (r *ModelRegistry) Get(name string) (*ModelCapabilities, bool) {
	cap, ok := r.models[name]
	return cap, ok
}

func (r *ModelRegistry) List() []string {
	names := make([]string, 0, len(r.models))
	for name := range r.models {
		names = append(names, name)
	}
	return names
}

func (r *ModelRegistry) ListByProvider(provider ProviderType) []string {
	var names []string
	for name, cap := range r.models {
		if cap.Provider == provider {
			names = append(names, name)
		}
	}
	return names
}

// RegisterVideo adds a video model to the registry
func (r *ModelRegistry) RegisterVideo(cap *VideoModelCapabilities) {
	r.videoModels[cap.Name] = cap
}

// GetVideo retrieves a video model from the registry
func (r *ModelRegistry) GetVideo(name string) (*VideoModelCapabilities, bool) {
	cap, ok := r.videoModels[name]
	return cap, ok
}

// ListVideoModels returns all registered video model names
func (r *ModelRegistry) ListVideoModels() []string {
	names := make([]string, 0, len(r.videoModels))
	for name := range r.videoModels {
		names = append(names, name)
	}
	return names
}

func DefaultRegistry() *ModelRegistry {
	r := NewModelRegistry()

	r.Register(&ModelCapabilities{
		Name:                 "gpt-image-1",
		Provider:             ProviderOpenAI,
		SupportedSizes:       []string{"1024x1024", "1536x1024", "1024x1536", "auto"},
		SupportedQualities:   []string{"auto", "low", "medium", "high"},
		MaxImages:            10,
		DefaultSize:          "1024x1024",
		DefaultQuality:       "auto",
		SupportsStyle:        false,
		SupportsTransparency: true,
		SupportsEdit:         true,
	})

	r.Register(&ModelCapabilities{
		Name:                 "dall-e-3",
		Provider:             ProviderOpenAI,
		SupportedSizes:       []string{"1024x1024", "1024x1792", "1792x1024"},
		SupportedQualities:   []string{"standard", "hd"},
		MaxImages:            1,
		DefaultSize:          "1024x1024",
		DefaultQuality:       "standard",
		SupportsStyle:        true,
		SupportsTransparency: false,
		SupportsEdit:         false,
		StyleOptions:         []string{"vivid", "natural"},
	})

	r.Register(&ModelCapabilities{
		Name:                 "dall-e-2",
		Provider:             ProviderOpenAI,
		SupportedSizes:       []string{"256x256", "512x512", "1024x1024"},
		SupportedQualities:   nil,
		MaxImages:            10,
		DefaultSize:          "1024x1024",
		DefaultQuality:       "",
		SupportsStyle:        false,
		SupportsTransparency: false,
		SupportsEdit:         true,
	})

	r.Register(&ModelCapabilities{
		Name:                 "stable-diffusion-xl",
		Provider:             ProviderStability,
		SupportedSizes:       []string{"1024x1024", "1152x896", "896x1152", "1216x832", "832x1216"},
		SupportedQualities:   nil,
		MaxImages:            10,
		DefaultSize:          "1024x1024",
		DefaultQuality:       "",
		SupportsStyle:        false,
		SupportsTransparency: false,
	})

	r.Register(&ModelCapabilities{
		Name:                 "stable-diffusion-3",
		Provider:             ProviderStability,
		SupportedSizes:       []string{"1024x1024", "1536x1024", "1024x1536"},
		SupportedQualities:   nil,
		MaxImages:            10,
		DefaultSize:          "1024x1024",
		DefaultQuality:       "",
		SupportsStyle:        false,
		SupportsTransparency: false,
	})

	// OCR models (GPT-5 series with vision capabilities)
	r.RegisterOCR(&OCRModelCapabilities{
		Name:             "gpt-5.2",
		Provider:         ProviderOpenAI,
		MaxImageSize:     20 * 1024 * 1024, // 20MB
		SupportsSchema:   true,
		DefaultMaxTokens: 16384,
	})

	r.RegisterOCR(&OCRModelCapabilities{
		Name:             "gpt-5-mini",
		Provider:         ProviderOpenAI,
		MaxImageSize:     20 * 1024 * 1024, // 20MB
		SupportsSchema:   true,
		DefaultMaxTokens: 16384,
	})

	r.RegisterOCR(&OCRModelCapabilities{
		Name:             "gpt-5-nano",
		Provider:         ProviderOpenAI,
		MaxImageSize:     20 * 1024 * 1024, // 20MB
		SupportsSchema:   true,
		DefaultMaxTokens: 16384,
	})

	// Video models (OpenAI Sora)
	r.RegisterVideo(&VideoModelCapabilities{
		Name:               "sora-2",
		Provider:           ProviderOpenAI,
		SupportedDurations: []int{4, 8, 12},
		SupportedSizes:     []string{"720x1280", "1280x720", "1024x1792", "1792x1024"},
		DefaultDuration:    4,
		DefaultSize:        "720x1280",
	})

	r.RegisterVideo(&VideoModelCapabilities{
		Name:               "sora-2-pro",
		Provider:           ProviderOpenAI,
		SupportedDurations: []int{4, 8, 12, 16, 20},
		SupportedSizes:     []string{"720x1280", "1280x720", "1024x1792", "1792x1024", "1080x1920", "1920x1080"},
		DefaultDuration:    4,
		DefaultSize:        "720x1280",
	})

	return r
}
