package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"goon/internal/config"
)

// Chat abstracts one completion, so tests can inject a fake.
type Chat interface {
	Complete(prompt string) (string, error)
}

// Client is an OpenAI-compatible chat completions client.
type Client struct {
	cfg config.LLM
	hc  *http.Client
}

// Chat is satisfied by *Client.
var _ Chat = (*Client)(nil)

func New(cfg config.LLM) *Client { return &Client{cfg: cfg, hc: http.DefaultClient} }

type msg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (c *Client) Complete(prompt string) (string, error) {
	key := os.Getenv(c.cfg.APIKeyEnv)
	if key == "" {
		return "", fmt.Errorf("env %s not set for llm key", c.cfg.APIKeyEnv)
	}
	payload, _ := json.Marshal(map[string]any{
		"model":    c.cfg.Model,
		"messages": []msg{{Role: "user", Content: prompt}},
	})
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("llm %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		Choices []struct {
			Message msg `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}
