package models

import (
	"errors"
	"testing"
)

func TestOutputFormat_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		format OutputFormat
		want   bool
	}{
		{"valid png", FormatPNG, true},
		{"valid jpeg", FormatJPEG, true},
		{"valid webp", FormatWebP, true},
		{"invalid format", OutputFormat("gif"), false},
		{"empty format", OutputFormat(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.format.IsValid(); got != tt.want {
				t.Errorf("OutputFormat.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOutputFormat_String(t *testing.T) {
	tests := []struct {
		format OutputFormat
		want   string
	}{
		{FormatPNG, "png"},
		{FormatJPEG, "jpeg"},
		{FormatWebP, "webp"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.format.String(); got != tt.want {
				t.Errorf("OutputFormat.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidFormats(t *testing.T) {
	formats := ValidFormats()
	if len(formats) != 3 {
		t.Errorf("ValidFormats() returned %d formats, want 3", len(formats))
	}

	expected := map[OutputFormat]bool{FormatPNG: true, FormatJPEG: true, FormatWebP: true}
	for _, f := range formats {
		if !expected[f] {
			t.Errorf("unexpected format in ValidFormats(): %v", f)
		}
	}
}

func TestNewRequest(t *testing.T) {
	prompt := "test prompt"
	req := NewRequest(prompt)

	if req.Prompt != prompt {
		t.Errorf("NewRequest().Prompt = %v, want %v", req.Prompt, prompt)
	}
	if req.Count != 1 {
		t.Errorf("NewRequest().Count = %v, want 1", req.Count)
	}
	if req.Format != FormatPNG {
		t.Errorf("NewRequest().Format = %v, want %v", req.Format, FormatPNG)
	}
}

func TestModelCapabilities_Validate(t *testing.T) {
	cap := &ModelCapabilities{
		Name:                 "test-model",
		Provider:             ProviderOpenAI,
		SupportedSizes:       []string{"1024x1024", "512x512"},
		SupportedQualities:   []string{"standard", "hd"},
		MaxImages:            5,
		DefaultSize:          "1024x1024",
		DefaultQuality:       "standard",
		SupportsStyle:        true,
		StyleOptions:         []string{"vivid", "natural"},
		SupportsTransparency: true,
	}

	tests := []struct {
		name    string
		req     *Request
		wantErr error
	}{
		{
			name:    "valid request",
			req:     &Request{Prompt: "test", Count: 1, Size: "1024x1024", Quality: "standard", Format: FormatPNG},
			wantErr: nil,
		},
		{
			name:    "empty prompt",
			req:     &Request{Prompt: "", Count: 1},
			wantErr: ErrEmptyPrompt,
		},
		{
			name:    "count zero",
			req:     &Request{Prompt: "test", Count: 0},
			wantErr: ErrInvalidCount,
		},
		{
			name:    "count negative",
			req:     &Request{Prompt: "test", Count: -1},
			wantErr: ErrInvalidCount,
		},
		{
			name:    "count exceeds max",
			req:     &Request{Prompt: "test", Count: 10},
			wantErr: ErrCountExceedsMax,
		},
		{
			name:    "invalid size",
			req:     &Request{Prompt: "test", Count: 1, Size: "2048x2048"},
			wantErr: ErrInvalidSize,
		},
		{
			name:    "invalid quality",
			req:     &Request{Prompt: "test", Count: 1, Quality: "ultra"},
			wantErr: ErrInvalidQuality,
		},
		{
			name:    "valid style",
			req:     &Request{Prompt: "test", Count: 1, Style: "vivid", Format: FormatPNG},
			wantErr: nil,
		},
		{
			name:    "invalid style option",
			req:     &Request{Prompt: "test", Count: 1, Style: "abstract"},
			wantErr: ErrStyleNotSupported,
		},
		{
			name:    "valid transparency",
			req:     &Request{Prompt: "test", Count: 1, Transparent: true, Format: FormatPNG},
			wantErr: nil,
		},
		{
			name:    "transparency with webp",
			req:     &Request{Prompt: "test", Count: 1, Transparent: true, Format: FormatWebP},
			wantErr: nil,
		},
		{
			name:    "transparency with invalid format",
			req:     &Request{Prompt: "test", Count: 1, Transparent: true, Format: FormatJPEG},
			wantErr: ErrInvalidTransparencyFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cap.Validate(tt.req)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("Validate() error = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() error = nil, want %v", tt.wantErr)
				} else if !errors.Is(err, tt.wantErr) {
					t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestModelCapabilities_Validate_NoStyleSupport(t *testing.T) {
	cap := &ModelCapabilities{
		Name:          "no-style-model",
		MaxImages:     10,
		SupportsStyle: false,
	}

	req := &Request{Prompt: "test", Count: 1, Style: "vivid"}
	err := cap.Validate(req)
	if !errors.Is(err, ErrStyleNotSupported) {
		t.Errorf("Validate() error = %v, want %v", err, ErrStyleNotSupported)
	}
}

func TestModelCapabilities_Validate_NoTransparencySupport(t *testing.T) {
	cap := &ModelCapabilities{
		Name:                 "no-transparency-model",
		MaxImages:            10,
		SupportsTransparency: false,
	}

	req := &Request{Prompt: "test", Count: 1, Transparent: true, Format: FormatPNG}
	err := cap.Validate(req)
	if !errors.Is(err, ErrTransparencyNotSupported) {
		t.Errorf("Validate() error = %v, want %v", err, ErrTransparencyNotSupported)
	}
}

func TestModelCapabilities_Validate_EmptyQualities(t *testing.T) {
	cap := &ModelCapabilities{
		Name:               "no-quality-model",
		MaxImages:          10,
		SupportedQualities: nil, // No quality options
	}

	req := &Request{Prompt: "test", Count: 1, Quality: "anything"}
	err := cap.Validate(req)
	// Should pass because SupportedQualities is empty
	if err != nil {
		t.Errorf("Validate() error = %v, want nil for empty qualities", err)
	}
}

func TestModelCapabilities_ApplyDefaults(t *testing.T) {
	cap := &ModelCapabilities{
		Name:           "test-model",
		DefaultSize:    "1024x1024",
		DefaultQuality: "standard",
	}

	req := &Request{Prompt: "test", Count: 1}
	cap.ApplyDefaults(req)

	if req.Size != "1024x1024" {
		t.Errorf("ApplyDefaults() Size = %v, want 1024x1024", req.Size)
	}
	if req.Quality != "standard" {
		t.Errorf("ApplyDefaults() Quality = %v, want standard", req.Quality)
	}
	if req.Model != "test-model" {
		t.Errorf("ApplyDefaults() Model = %v, want test-model", req.Model)
	}
}

func TestModelCapabilities_ApplyDefaults_PreservesExisting(t *testing.T) {
	cap := &ModelCapabilities{
		Name:           "test-model",
		DefaultSize:    "1024x1024",
		DefaultQuality: "standard",
	}

	req := &Request{
		Prompt:  "test",
		Model:   "custom-model",
		Size:    "512x512",
		Quality: "hd",
		Count:   1,
	}
	cap.ApplyDefaults(req)

	if req.Size != "512x512" {
		t.Errorf("ApplyDefaults() should preserve Size, got %v", req.Size)
	}
	if req.Quality != "hd" {
		t.Errorf("ApplyDefaults() should preserve Quality, got %v", req.Quality)
	}
	if req.Model != "custom-model" {
		t.Errorf("ApplyDefaults() should preserve Model, got %v", req.Model)
	}
}

func TestModelCapabilities_ApplyDefaults_EmptyQuality(t *testing.T) {
	cap := &ModelCapabilities{
		Name:           "test-model",
		DefaultSize:    "1024x1024",
		DefaultQuality: "", // No default quality
	}

	req := &Request{Prompt: "test", Count: 1}
	cap.ApplyDefaults(req)

	if req.Quality != "" {
		t.Errorf("ApplyDefaults() Quality = %v, want empty", req.Quality)
	}
}

func TestModelRegistry(t *testing.T) {
	r := NewModelRegistry()

	cap := &ModelCapabilities{
		Name:     "test-model",
		Provider: ProviderOpenAI,
	}

	r.Register(cap)

	// Test Get
	got, ok := r.Get("test-model")
	if !ok {
		t.Error("Get() returned false for registered model")
	}
	if got.Name != "test-model" {
		t.Errorf("Get() Name = %v, want test-model", got.Name)
	}

	// Test Get non-existent
	_, ok = r.Get("non-existent")
	if ok {
		t.Error("Get() returned true for non-existent model")
	}

	// Test List
	list := r.List()
	if len(list) != 1 {
		t.Errorf("List() returned %d models, want 1", len(list))
	}
}

func TestModelRegistry_ListByProvider(t *testing.T) {
	r := NewModelRegistry()

	r.Register(&ModelCapabilities{Name: "openai-1", Provider: ProviderOpenAI})
	r.Register(&ModelCapabilities{Name: "openai-2", Provider: ProviderOpenAI})
	r.Register(&ModelCapabilities{Name: "stability-1", Provider: ProviderStability})

	openaiModels := r.ListByProvider(ProviderOpenAI)
	if len(openaiModels) != 2 {
		t.Errorf("ListByProvider(OpenAI) returned %d models, want 2", len(openaiModels))
	}

	stabilityModels := r.ListByProvider(ProviderStability)
	if len(stabilityModels) != 1 {
		t.Errorf("ListByProvider(Stability) returned %d models, want 1", len(stabilityModels))
	}
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()

	// Check OpenAI models
	openaiModels := []string{"gpt-image-1", "dall-e-3", "dall-e-2"}
	for _, model := range openaiModels {
		cap, ok := r.Get(model)
		if !ok {
			t.Errorf("DefaultRegistry() missing model: %s", model)
			continue
		}
		if cap.Provider != ProviderOpenAI {
			t.Errorf("Model %s has provider %v, want %v", model, cap.Provider, ProviderOpenAI)
		}
	}

	// Check Stability models
	stabilityModels := []string{"stable-diffusion-xl", "stable-diffusion-3"}
	for _, model := range stabilityModels {
		cap, ok := r.Get(model)
		if !ok {
			t.Errorf("DefaultRegistry() missing model: %s", model)
			continue
		}
		if cap.Provider != ProviderStability {
			t.Errorf("Model %s has provider %v, want %v", model, cap.Provider, ProviderStability)
		}
	}
}

func TestDefaultRegistry_GPTImage1(t *testing.T) {
	r := DefaultRegistry()
	cap, ok := r.Get("gpt-image-1")
	if !ok {
		t.Fatal("gpt-image-1 not found")
	}

	if cap.MaxImages != 10 {
		t.Errorf("gpt-image-1 MaxImages = %d, want 10", cap.MaxImages)
	}
	if !cap.SupportsTransparency {
		t.Error("gpt-image-1 should support transparency")
	}
	if cap.SupportsStyle {
		t.Error("gpt-image-1 should not support style")
	}
}

func TestDefaultRegistry_DallE3(t *testing.T) {
	r := DefaultRegistry()
	cap, ok := r.Get("dall-e-3")
	if !ok {
		t.Fatal("dall-e-3 not found")
	}

	if cap.MaxImages != 1 {
		t.Errorf("dall-e-3 MaxImages = %d, want 1", cap.MaxImages)
	}
	if cap.SupportsTransparency {
		t.Error("dall-e-3 should not support transparency")
	}
	if !cap.SupportsStyle {
		t.Error("dall-e-3 should support style")
	}
	if len(cap.StyleOptions) != 2 {
		t.Errorf("dall-e-3 StyleOptions length = %d, want 2", len(cap.StyleOptions))
	}
}

func TestDefaultRegistry_DallE2(t *testing.T) {
	r := DefaultRegistry()
	cap, ok := r.Get("dall-e-2")
	if !ok {
		t.Fatal("dall-e-2 not found")
	}

	if cap.MaxImages != 10 {
		t.Errorf("dall-e-2 MaxImages = %d, want 10", cap.MaxImages)
	}
	if cap.DefaultQuality != "" {
		t.Errorf("dall-e-2 should have empty DefaultQuality, got %s", cap.DefaultQuality)
	}
}

func TestProviderType_Constants(t *testing.T) {
	if ProviderOpenAI != "openai" {
		t.Errorf("ProviderOpenAI = %v, want openai", ProviderOpenAI)
	}
	if ProviderStability != "stability" {
		t.Errorf("ProviderStability = %v, want stability", ProviderStability)
	}
}

func TestVideoModelCapabilities_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     *VideoRequest
		cap     *VideoModelCapabilities
		wantErr error
	}{
		{
			name: "valid request",
			req: &VideoRequest{
				Prompt:   "a cat playing",
				Model:    "sora-2",
				Duration: 4,
				Size:     "1280x720",
			},
			cap: &VideoModelCapabilities{
				Name:               "sora-2",
				Provider:           ProviderOpenAI,
				SupportedDurations: []int{4, 8, 12},
				SupportedSizes:     []string{"720x1280", "1280x720", "1024x1792", "1792x1024"},
				DefaultDuration:    4,
				DefaultSize:        "720x1280",
			},
			wantErr: nil,
		},
		{
			name: "empty prompt",
			req: &VideoRequest{
				Prompt: "",
				Model:  "sora-2",
			},
			cap:     &VideoModelCapabilities{Name: "sora-2"},
			wantErr: ErrEmptyPrompt,
		},
		{
			name: "invalid duration",
			req: &VideoRequest{
				Prompt:   "test",
				Duration: 10,
			},
			cap: &VideoModelCapabilities{
				Name:               "sora-2",
				SupportedDurations: []int{4, 8, 12},
			},
			wantErr: ErrInvalidDuration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cap.Validate(tt.req)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestVideoModelCapabilities_Validate_InvalidSize(t *testing.T) {
	cap := &VideoModelCapabilities{
		Name:               "sora-2",
		SupportedSizes:     []string{"720x1280", "1280x720"},
		SupportedDurations: []int{4, 8, 12},
	}

	req := &VideoRequest{
		Prompt: "test prompt",
		Size:   "invalid-size",
	}

	err := cap.Validate(req)
	if !errors.Is(err, ErrInvalidSize) {
		t.Errorf("Validate() error = %v, want %v", err, ErrInvalidSize)
	}
}

func TestVideoModelCapabilities_ApplyDefaults(t *testing.T) {
	cap := &VideoModelCapabilities{
		Name:            "sora-2",
		DefaultDuration: 4,
		DefaultSize:     "720x1280",
	}

	req := &VideoRequest{Prompt: "test prompt"}
	cap.ApplyDefaults(req)

	if req.Duration != 4 {
		t.Errorf("ApplyDefaults() Duration = %v, want 4", req.Duration)
	}
	if req.Size != "720x1280" {
		t.Errorf("ApplyDefaults() Size = %v, want 720x1280", req.Size)
	}
	if req.Model != "sora-2" {
		t.Errorf("ApplyDefaults() Model = %v, want sora-2", req.Model)
	}
}

func TestVideoModelCapabilities_ApplyDefaults_PreservesExisting(t *testing.T) {
	cap := &VideoModelCapabilities{
		Name:            "sora-2",
		DefaultDuration: 4,
		DefaultSize:     "720x1280",
	}

	req := &VideoRequest{
		Prompt:   "test prompt",
		Model:    "sora-2-pro",
		Duration: 8,
		Size:     "1280x720",
	}
	cap.ApplyDefaults(req)

	if req.Duration != 8 {
		t.Errorf("ApplyDefaults() should preserve Duration, got %v", req.Duration)
	}
	if req.Size != "1280x720" {
		t.Errorf("ApplyDefaults() should preserve Size, got %v", req.Size)
	}
	if req.Model != "sora-2-pro" {
		t.Errorf("ApplyDefaults() should preserve Model, got %v", req.Model)
	}
}

func TestNewVideoRequest(t *testing.T) {
	prompt := "a cat playing piano"
	req := NewVideoRequest(prompt)

	if req.Prompt != prompt {
		t.Errorf("NewVideoRequest().Prompt = %v, want %v", req.Prompt, prompt)
	}
	if req.Duration != 0 {
		t.Errorf("NewVideoRequest().Duration = %v, want 0 (unset)", req.Duration)
	}
	if req.Size != "" {
		t.Errorf("NewVideoRequest().Size = %v, want empty", req.Size)
	}
}

func TestModelRegistry_Video(t *testing.T) {
	r := NewModelRegistry()

	cap := &VideoModelCapabilities{
		Name:     "test-video-model",
		Provider: ProviderOpenAI,
	}

	r.RegisterVideo(cap)

	// Test GetVideo
	got, ok := r.GetVideo("test-video-model")
	if !ok {
		t.Error("GetVideo() returned false for registered model")
	}
	if got.Name != "test-video-model" {
		t.Errorf("GetVideo() Name = %v, want test-video-model", got.Name)
	}

	// Test GetVideo non-existent
	_, ok = r.GetVideo("non-existent")
	if ok {
		t.Error("GetVideo() returned true for non-existent model")
	}

	// Test ListVideoModels
	list := r.ListVideoModels()
	if len(list) != 1 {
		t.Errorf("ListVideoModels() returned %d models, want 1", len(list))
	}
}

func TestDefaultRegistry_VideoModels(t *testing.T) {
	r := DefaultRegistry()

	// Check sora-2 is registered
	cap, ok := r.GetVideo("sora-2")
	if !ok {
		t.Fatal("sora-2 not found in registry")
	}
	if cap.Provider != ProviderOpenAI {
		t.Errorf("sora-2 provider = %v, want %v", cap.Provider, ProviderOpenAI)
	}
	if cap.DefaultDuration != 4 {
		t.Errorf("sora-2 default duration = %d, want 4", cap.DefaultDuration)
	}

	// Check sora-2-pro is registered
	cap, ok = r.GetVideo("sora-2-pro")
	if !ok {
		t.Fatal("sora-2-pro not found in registry")
	}

	// Verify ListVideoModels
	models := r.ListVideoModels()
	if len(models) != 2 {
		t.Errorf("ListVideoModels() returned %d models, want 2", len(models))
	}
}
