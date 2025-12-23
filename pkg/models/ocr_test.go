package models

import (
	"encoding/json"
	"testing"
)

func TestOCRRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     *OCRRequest
		wantErr error
	}{
		{
			name:    "empty request",
			req:     &OCRRequest{},
			wantErr: ErrNoImageSource,
		},
		{
			name: "with image path",
			req: &OCRRequest{
				ImagePath: "/path/to/image.png",
			},
			wantErr: nil,
		},
		{
			name: "with image URL",
			req: &OCRRequest{
				ImageURL: "https://example.com/image.png",
			},
			wantErr: nil,
		},
		{
			name: "with image data",
			req: &OCRRequest{
				ImageData: []byte{0x89, 0x50, 0x4E, 0x47},
			},
			wantErr: nil,
		},
		{
			name: "with valid schema",
			req: &OCRRequest{
				ImagePath: "/path/to/image.png",
				Schema:    json.RawMessage(`{"type": "object"}`),
			},
			wantErr: nil,
		},
		{
			name: "with invalid schema",
			req: &OCRRequest{
				ImagePath: "/path/to/image.png",
				Schema:    json.RawMessage(`{invalid json`),
			},
			wantErr: ErrInvalidSchema,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error = %v", err)
			}
		})
	}
}

func TestNewOCRRequest(t *testing.T) {
	req := NewOCRRequest()

	if req.Model != "gpt-5-mini" {
		t.Errorf("NewOCRRequest().Model = %v, want %v", req.Model, "gpt-5-mini")
	}
	if req.MaxTokens != 16384 {
		t.Errorf("NewOCRRequest().MaxTokens = %v, want %v", req.MaxTokens, 16384)
	}
	if req.Temperature != 0 {
		t.Errorf("NewOCRRequest().Temperature = %v, want %v", req.Temperature, 0)
	}
}

func TestModelRegistry_OCR(t *testing.T) {
	r := DefaultRegistry()

	// Test GetOCR
	cap, ok := r.GetOCR("gpt-5.2")
	if !ok {
		t.Error("GetOCR('gpt-5.2') should return true")
	}
	if cap.Name != "gpt-5.2" {
		t.Errorf("GetOCR('gpt-5.2').Name = %v, want %v", cap.Name, "gpt-5.2")
	}
	if cap.Provider != ProviderOpenAI {
		t.Errorf("GetOCR('gpt-5.2').Provider = %v, want %v", cap.Provider, ProviderOpenAI)
	}
	if !cap.SupportsSchema {
		t.Error("GetOCR('gpt-5.2').SupportsSchema should be true")
	}

	// Test non-existent model
	_, ok = r.GetOCR("non-existent")
	if ok {
		t.Error("GetOCR('non-existent') should return false")
	}

	// Test ListOCRModels
	models := r.ListOCRModels()
	if len(models) < 3 {
		t.Errorf("ListOCRModels() returned %d models, want at least 3", len(models))
	}

	// Test ListOCRByProvider
	openAIModels := r.ListOCRByProvider(ProviderOpenAI)
	if len(openAIModels) < 3 {
		t.Errorf("ListOCRByProvider(ProviderOpenAI) returned %d models, want at least 3", len(openAIModels))
	}
}
