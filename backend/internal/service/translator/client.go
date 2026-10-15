package translator

// Машинный перевод сегмента через OpenAI-совместимый Chat Completions API (по умолчанию DeepSeek).
// Ключ API только на сервере.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
)

// Model переводит один сегмент; в тестах — сценарная модель.
type Model interface {
	Configured() bool
	Name() string
	Translate(ctx context.Context, system, user string) (string, error)
}

const (
	requestTimeout = 60 * time.Second
	maxAttempts    = 4 // повторы при 429, 5xx и тайм-ауте: через 2, 4, 8 с
)

type Client struct {
	apiKey, baseURL, model string
	httpClient             *http.Client
	retryDelay             time.Duration
}

func NewClient(apiKey, baseURL, model string) *Client {
	return &Client{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), model: model, httpClient: &http.Client{}, retryDelay: 2 * time.Second}
}

func (c *Client) Configured() bool { return c.apiKey != "" && c.model != "" }
func (c *Client) Name() string     { return c.model }

func (c *Client) Translate(ctx context.Context, system, user string) (string, error) {
	delay := c.retryDelay
	for attempt := 1; ; attempt++ {
		text, err := c.send(ctx, system, user)
		retry := errors.Is(err, ai.ErrAIRateLimited) || errors.Is(err, ai.ErrAIUnavailable) || errors.Is(err, ai.ErrAITimeout)
		if !retry || attempt == maxAttempts {
			return text, err
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
		delay *= 2
	}
}

func (c *Client) send(ctx context.Context, system, user string) (string, error) {
	request := map[string]any{
		"model":       c.model,
		"messages":    []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
		"max_tokens":  4096,
		"temperature": 0.2,
	}
	if strings.Contains(c.baseURL, "deepseek.com") {
		// Отдельный режим рассуждений DeepSeek медленный и для перевода не нужен.
		request["thinking"] = map[string]string{"type": "disabled"}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return "", ai.ErrAIBadResponse
	}
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", ai.ErrAIBadResponse
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			return "", ai.ErrAITimeout
		}
		return "", ai.ErrAIUnavailable
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusPaymentRequired:
		return "", ai.ErrAIUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests:
		return "", ai.ErrAIRateLimited
	case resp.StatusCode == http.StatusBadRequest, resp.StatusCode == http.StatusUnprocessableEntity:
		return "", ai.ErrAIRequestRejected
	case resp.StatusCode == http.StatusNotFound:
		return "", ai.ErrAIModelUnavailable
	case resp.StatusCode == http.StatusRequestTimeout:
		return "", ai.ErrAITimeout
	case resp.StatusCode >= 500:
		return "", ai.ErrAIUnavailable
	case resp.StatusCode < 200 || resp.StatusCode >= 300 || readErr != nil:
		return "", ai.ErrAIBadResponse
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(data, &parsed) != nil || len(parsed.Choices) == 0 {
		return "", ai.ErrAIBadResponse
	}
	text := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if text == "" {
		return "", ai.ErrAIBadResponse
	}
	return text, nil
}
