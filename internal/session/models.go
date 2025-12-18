package session

import (
	"encoding/json"
	"time"
)

type Session struct {
	ID                 string
	Name               string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CurrentIterationID string
	Model              string
}

type Iteration struct {
	ID            string
	SessionID     string
	ParentID      string
	Operation     string // "generate" or "edit"
	Prompt        string
	RevisedPrompt string
	Model         string
	ImagePath     string
	Timestamp     time.Time
	Metadata      IterationMetadata
}

type IterationMetadata struct {
	Size        string  `json:"size,omitempty"`
	Quality     string  `json:"quality,omitempty"`
	Format      string  `json:"format,omitempty"`
	Transparent bool    `json:"transparent,omitempty"`
	Cost        float64 `json:"cost,omitempty"`
	Provider    string  `json:"provider,omitempty"`
}

func (m *IterationMetadata) ToJSON() string {
	data, _ := json.Marshal(m)
	return string(data)
}

func ParseIterationMetadata(data string) IterationMetadata {
	var m IterationMetadata
	if data != "" {
		json.Unmarshal([]byte(data), &m)
	}
	return m
}
