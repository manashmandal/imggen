package models

import "testing"

func TestNewResponsesRequest(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
	}{
		{"simple prompt", "a red car"},
		{"empty prompt", ""},
		{"long prompt", "a very detailed scene with mountains, rivers, trees, and a sunset in the background"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := NewResponsesRequest(tt.prompt)

			if req.Prompt != tt.prompt {
				t.Errorf("Prompt = %q, want %q", req.Prompt, tt.prompt)
			}
			if req.Action != "auto" {
				t.Errorf("Action = %q, want %q", req.Action, "auto")
			}
			if req.OutputFormat != FormatPNG {
				t.Errorf("OutputFormat = %q, want %q", req.OutputFormat, FormatPNG)
			}
		})
	}
}

func TestNewResponsesRequest_ZeroValueDefaults(t *testing.T) {
	req := NewResponsesRequest("test")

	if req.Model != "" {
		t.Errorf("Model = %q, want empty", req.Model)
	}
	if req.PreviousResponseID != "" {
		t.Errorf("PreviousResponseID = %q, want empty", req.PreviousResponseID)
	}
	if req.ImagePaths != nil {
		t.Errorf("ImagePaths = %v, want nil", req.ImagePaths)
	}
	if req.ImageURLs != nil {
		t.Errorf("ImageURLs = %v, want nil", req.ImageURLs)
	}
	if req.ImageData != nil {
		t.Errorf("ImageData = %v, want nil", req.ImageData)
	}
	if req.Size != "" {
		t.Errorf("Size = %q, want empty", req.Size)
	}
	if req.Quality != "" {
		t.Errorf("Quality = %q, want empty", req.Quality)
	}
	if req.Background != "" {
		t.Errorf("Background = %q, want empty", req.Background)
	}
	if req.InputFidelity != "" {
		t.Errorf("InputFidelity = %q, want empty", req.InputFidelity)
	}
}

func TestResponsesRequest_FieldsSettable(t *testing.T) {
	req := NewResponsesRequest("edit this image")

	req.Model = "gpt-image-1.5"
	req.PreviousResponseID = "resp_abc123"
	req.ImagePaths = []string{"/tmp/image1.png", "/tmp/image2.png"}
	req.ImageURLs = []string{"https://example.com/img.png"}
	req.ImageData = [][]byte{{0x89, 0x50, 0x4E, 0x47}}
	req.Action = "edit"
	req.Size = "1024x1024"
	req.Quality = "high"
	req.Background = "transparent"
	req.OutputFormat = FormatWebP
	req.InputFidelity = "high"

	if req.Model != "gpt-image-1.5" {
		t.Errorf("Model = %q, want %q", req.Model, "gpt-image-1.5")
	}
	if req.PreviousResponseID != "resp_abc123" {
		t.Errorf("PreviousResponseID = %q, want %q", req.PreviousResponseID, "resp_abc123")
	}
	if len(req.ImagePaths) != 2 {
		t.Errorf("ImagePaths length = %d, want 2", len(req.ImagePaths))
	}
	if len(req.ImageURLs) != 1 {
		t.Errorf("ImageURLs length = %d, want 1", len(req.ImageURLs))
	}
	if len(req.ImageData) != 1 {
		t.Errorf("ImageData length = %d, want 1", len(req.ImageData))
	}
	if req.Action != "edit" {
		t.Errorf("Action = %q, want %q", req.Action, "edit")
	}
	if req.Size != "1024x1024" {
		t.Errorf("Size = %q, want %q", req.Size, "1024x1024")
	}
	if req.Quality != "high" {
		t.Errorf("Quality = %q, want %q", req.Quality, "high")
	}
	if req.Background != "transparent" {
		t.Errorf("Background = %q, want %q", req.Background, "transparent")
	}
	if req.OutputFormat != FormatWebP {
		t.Errorf("OutputFormat = %q, want %q", req.OutputFormat, FormatWebP)
	}
	if req.InputFidelity != "high" {
		t.Errorf("InputFidelity = %q, want %q", req.InputFidelity, "high")
	}
}

func TestResponsesResponse_ZeroValue(t *testing.T) {
	var resp ResponsesResponse

	if resp.ID != "" {
		t.Errorf("ID = %q, want empty", resp.ID)
	}
	if resp.Images != nil {
		t.Errorf("Images = %v, want nil", resp.Images)
	}
	if resp.Text != "" {
		t.Errorf("Text = %q, want empty", resp.Text)
	}
	if resp.Cost != nil {
		t.Errorf("Cost = %v, want nil", resp.Cost)
	}
	if resp.RevisedPrompt != "" {
		t.Errorf("RevisedPrompt = %q, want empty", resp.RevisedPrompt)
	}
}

func TestResponsesResponse_FieldsSettable(t *testing.T) {
	resp := ResponsesResponse{
		ID: "resp_xyz789",
		Images: []GeneratedImage{
			{Data: []byte{0xFF}, Index: 0, Filename: "output.png"},
		},
		Text:          "Here is the generated image",
		Cost:          &CostInfo{PerImage: 0.04, Total: 0.04, Currency: "USD"},
		RevisedPrompt: "a detailed red car on a highway",
	}

	if resp.ID != "resp_xyz789" {
		t.Errorf("ID = %q, want %q", resp.ID, "resp_xyz789")
	}
	if len(resp.Images) != 1 {
		t.Errorf("Images length = %d, want 1", len(resp.Images))
	}
	if resp.Images[0].Filename != "output.png" {
		t.Errorf("Images[0].Filename = %q, want %q", resp.Images[0].Filename, "output.png")
	}
	if resp.Text != "Here is the generated image" {
		t.Errorf("Text = %q, want %q", resp.Text, "Here is the generated image")
	}
	if resp.Cost.Total != 0.04 {
		t.Errorf("Cost.Total = %f, want 0.04", resp.Cost.Total)
	}
	if resp.RevisedPrompt != "a detailed red car on a highway" {
		t.Errorf("RevisedPrompt = %q, want %q", resp.RevisedPrompt, "a detailed red car on a highway")
	}
}
