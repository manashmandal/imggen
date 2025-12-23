package models

import (
	"encoding/json"
	"errors"
)

var (
	ErrNoImageSource = errors.New("image source is required (file path or URL)")
	ErrInvalidSchema = errors.New("invalid JSON schema")
)

type OCRRequest struct {
	ImagePath   string          `json:"image_path,omitempty"`
	ImageURL    string          `json:"image_url,omitempty"`
	ImageData   []byte          `json:"-"`
	Model       string          `json:"model"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	SchemaName  string          `json:"schema_name,omitempty"`
	Prompt      string          `json:"prompt,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
}

func NewOCRRequest() *OCRRequest {
	return &OCRRequest{
		Model:       "gpt-5-mini",
		MaxTokens:   16384,
		Temperature: 0,
	}
}

func (r *OCRRequest) Validate() error {
	if r.ImagePath == "" && r.ImageURL == "" && len(r.ImageData) == 0 {
		return ErrNoImageSource
	}
	if len(r.Schema) > 0 {
		var schema map[string]interface{}
		if err := json.Unmarshal(r.Schema, &schema); err != nil {
			return ErrInvalidSchema
		}
	}
	return nil
}

type OCRResponse struct {
	Text        string          `json:"text,omitempty"`
	Structured  json.RawMessage `json:"structured,omitempty"`
	Cost        *CostInfo       `json:"cost,omitempty"`
	InputTokens int             `json:"input_tokens,omitempty"`
	OutputTokens int            `json:"output_tokens,omitempty"`
}

type OCRModelCapabilities struct {
	Name            string
	Provider        ProviderType
	MaxImageSize    int
	SupportsSchema  bool
	DefaultMaxTokens int
}

func (r *ModelRegistry) RegisterOCR(cap *OCRModelCapabilities) {
	r.ocrModels[cap.Name] = cap
}

func (r *ModelRegistry) GetOCR(name string) (*OCRModelCapabilities, bool) {
	cap, ok := r.ocrModels[name]
	return cap, ok
}

func (r *ModelRegistry) ListOCRModels() []string {
	names := make([]string, 0, len(r.ocrModels))
	for name := range r.ocrModels {
		names = append(names, name)
	}
	return names
}

func (r *ModelRegistry) ListOCRByProvider(provider ProviderType) []string {
	var names []string
	for name, cap := range r.ocrModels {
		if cap.Provider == provider {
			names = append(names, name)
		}
	}
	return names
}
