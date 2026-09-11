package anekbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultHuggingFaceBaseURL = "https://router.huggingface.co/v1"
	defaultHuggingFaceModel   = "deepseek-ai/DeepSeek-V4.1-Flash"
	huggingFaceRequestTimeout = 30 * time.Second
)

type hfMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type hfRequest struct {
	Model    string      `json:"model"`
	Messages []hfMessage `json:"messages"`
}

type hfResponse struct {
	Choices []struct {
		Message hfMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type huggingFaceProvider struct {
	client  *http.Client
	baseURL string
	apiKey  string
	model   string
}

func NewHuggingFaceProvider(apiKey, model string) *huggingFaceProvider {
	if model == "" {
		model = defaultHuggingFaceModel
	}
	return &huggingFaceProvider{
		client:  &http.Client{Timeout: huggingFaceRequestTimeout},
		baseURL: defaultHuggingFaceBaseURL,
		apiKey:  apiKey,
		model:   model,
	}
}

func (p *huggingFaceProvider) Name() string {
	return "Hugging Face"
}

func (p *huggingFaceProvider) Ask(ctx context.Context, question string) (string, error) {
	reqBody := hfRequest{
		Model: p.model,
		Messages: []hfMessage{
			{Role: "system", Content: llmSystemPrompt},
			{Role: "user", Content: question},
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var parsed hfResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if parsed.Error != nil {
			return "", fmt.Errorf("huggingface api: %s", parsed.Error.Message)
		}
		return "", fmt.Errorf("huggingface api: unexpected status %d", resp.StatusCode)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("huggingface api: no choices returned")
	}

	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}
