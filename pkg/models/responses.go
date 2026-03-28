package models

type ResponsesRequest struct {
	Model              string
	Prompt             string
	PreviousResponseID string
	ImagePaths         []string
	ImageURLs          []string
	ImageData          [][]byte
	Action             string // "auto", "generate", "edit"
	Size               string
	Quality            string
	Background         string
	OutputFormat       OutputFormat
	InputFidelity      string
	OutputCompression  *int   // 0-100, nil means not set (server default)
	Moderation         string // "low" or "auto"
	ImageModel         string // tool-level model override (gpt-image-1, gpt-image-1-mini, gpt-image-1.5)
}

func NewResponsesRequest(prompt string) *ResponsesRequest {
	return &ResponsesRequest{
		Prompt:       prompt,
		Action:       "auto",
		OutputFormat: FormatPNG,
	}
}

type ResponsesResponse struct {
	ID            string
	Images        []GeneratedImage
	Text          string
	Cost          *CostInfo
	RevisedPrompt string
}
