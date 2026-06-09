package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"fyp-data/config"
	lru "github.com/hashicorp/golang-lru/v2"
)

type Client struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
	limiter <-chan time.Time
	cache   *lru.Cache[string, []float32]
}

func NewClient(cfg config.Embedding) *Client {
	var cache *lru.Cache[string, []float32]
	if cfg.CacheSize > 0 {
		cache, _ = lru.New[string, []float32](cfg.CacheSize)
	}
	return &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		model:   cfg.Model,
		apiKey:  cfg.APIKey,
		client: &http.Client{
			Timeout: time.Duration(cfg.RequestTimeoutSecs) * time.Second,
		},
		limiter: rateLimiter(cfg.RequestsPerMinute),
		cache:   cache,
	}
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	cacheKey := c.baseURL + "\x00" + c.model + "\x00" + text
	if c.cache != nil {
		if cached, ok := c.cache.Get(cacheKey); ok {
			return slices.Clone(cached), nil
		}
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.limiter:
	}
	body, err := json.Marshal(map[string]string{
		"input": text,
		"model": c.model,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("embedding API returned %s: %s", resp.Status, truncate(string(respBody), 500))
	}
	var parsed struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Data) == 0 || len(parsed.Data[0].Embedding) == 0 {
		return nil, errors.New("embedding API response did not contain an embedding")
	}
	embedding := parsed.Data[0].Embedding
	if c.cache != nil {
		c.cache.Add(cacheKey, slices.Clone(embedding))
	}
	return embedding, nil
}

func rateLimiter(requestsPerMinute int) <-chan time.Time {
	if requestsPerMinute <= 0 {
		requestsPerMinute = 60
	}
	interval := time.Minute / time.Duration(requestsPerMinute)
	if interval <= 0 {
		interval = time.Nanosecond
	}
	return time.Tick(interval)
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
