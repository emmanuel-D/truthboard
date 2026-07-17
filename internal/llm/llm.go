// Package llm gives the binary two optional LLM-backed workflows — drafting
// a backlog from a concept and narrating a sprint review — without pulling
// in any SDK. A provider is configured purely by environment: an Anthropic
// key or a local Ollama host. Nothing in this package runs unless the user
// explicitly invokes truthboard draft or truthboard review; the rest of the
// binary never touches it.
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Provider turns one prompt into one completion. Both workflows need
// nothing fancier.
type Provider interface {
	Complete(prompt string) (string, error)
	Name() string
}

// FromEnv picks a provider: ANTHROPIC_API_KEY wins, then OLLAMA_HOST.
// TRUTHBOARD_LLM_MODEL overrides the default model for either.
func FromEnv() (Provider, error) {
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		model := os.Getenv("TRUTHBOARD_LLM_MODEL")
		if model == "" {
			model = "claude-sonnet-5"
		}
		return &anthropic{key: key, model: model, base: "https://api.anthropic.com"}, nil
	}
	if host := os.Getenv("OLLAMA_HOST"); host != "" {
		model := os.Getenv("TRUTHBOARD_LLM_MODEL")
		if model == "" {
			model = "llama3.1"
		}
		if !strings.Contains(host, "://") {
			host = "http://" + host
		}
		return &ollama{host: strings.TrimRight(host, "/"), model: model}, nil
	}
	return nil, fmt.Errorf("no LLM configured — set ANTHROPIC_API_KEY (Anthropic API) or OLLAMA_HOST (local Ollama); TRUTHBOARD_LLM_MODEL overrides the model")
}

var httpClient = &http.Client{Timeout: 120 * time.Second}

type anthropic struct {
	key, model, base string
}

func (a *anthropic) Name() string { return "anthropic/" + a.model }

func (a *anthropic) Complete(prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":      a.model,
		"max_tokens": 4096,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
	})
	req, err := http.NewRequest(http.MethodPost, a.base+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.key)
	req.Header.Set("anthropic-version", "2023-06-01")
	var out struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := do(req, &out); err != nil {
		return "", err
	}
	if out.Error != nil {
		return "", fmt.Errorf("anthropic: %s", out.Error.Message)
	}
	if len(out.Content) == 0 {
		return "", fmt.Errorf("anthropic: empty response")
	}
	return out.Content[0].Text, nil
}

type ollama struct {
	host, model string
}

func (o *ollama) Name() string { return "ollama/" + o.model }

func (o *ollama) Complete(prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model": o.model, "prompt": prompt, "stream": false,
	})
	req, err := http.NewRequest(http.MethodPost, o.host+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var out struct {
		Response string `json:"response"`
		Error    string `json:"error"`
	}
	if err := do(req, &out); err != nil {
		return "", err
	}
	if out.Error != "" {
		return "", fmt.Errorf("ollama: %s", out.Error)
	}
	return out.Response, nil
}

func do(req *http.Request, into any) error {
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		// Decode anyway — both APIs put the useful message in the body.
		if json.Unmarshal(raw, into) == nil {
			return nil
		}
		return fmt.Errorf("%s: HTTP %d: %.300s", req.URL.Host, resp.StatusCode, raw)
	}
	return json.Unmarshal(raw, into)
}
