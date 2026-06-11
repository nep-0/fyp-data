package embedding

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"fyp-data/config"
	lru "github.com/hashicorp/golang-lru/v2"
)

type Client struct {
	baseURL   string
	model     string
	apiKey    string
	client    *http.Client
	limiter   <-chan time.Time
	cache     *lru.Cache[string, []float32]
	cachePath string
}

func NewClient(cfg config.Embedding) *Client {
	var cache *lru.Cache[string, []float32]
	if cfg.CacheSize > 0 {
		cache, _ = lru.New[string, []float32](cfg.CacheSize)
	}
	c := &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		model:   cfg.Model,
		apiKey:  cfg.APIKey,
		client: &http.Client{
			Timeout: time.Duration(cfg.RequestTimeoutSecs) * time.Second,
		},
		limiter:   rateLimiter(cfg.RequestsPerMinute),
		cache:     cache,
		cachePath: cfg.CachePath,
	}
	if err := c.LoadCache(); err != nil {
		// Cache load failure should not disable semantic search.
		fmt.Fprintf(os.Stderr, "embedding cache load failed: %v\n", err)
	}
	return c
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

type persistedCache struct {
	Version int
	BaseURL string
	Model   string
	Entries []persistedCacheEntry
}

type persistedCacheEntry struct {
	Key       string
	Embedding []float32
}

func (c *Client) LoadCache() error {
	if c.cache == nil || strings.TrimSpace(c.cachePath) == "" {
		return nil
	}
	f, err := os.Open(c.cachePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open %s: %w", c.cachePath, err)
	}
	defer f.Close()

	var persisted persistedCache
	if err := gob.NewDecoder(f).Decode(&persisted); err != nil {
		return fmt.Errorf("decode %s: %w", c.cachePath, err)
	}
	if persisted.Version != 1 {
		return fmt.Errorf("unsupported cache version %d in %s", persisted.Version, c.cachePath)
	}
	if persisted.BaseURL != "" && persisted.BaseURL != c.baseURL {
		return nil
	}
	if persisted.Model != "" && persisted.Model != c.model {
		return nil
	}
	for _, entry := range persisted.Entries {
		c.cache.Add(entry.Key, slices.Clone(entry.Embedding))
	}
	return nil
}

func (c *Client) SaveCache() error {
	if c.cache == nil || strings.TrimSpace(c.cachePath) == "" {
		return nil
	}

	entries := make([]persistedCacheEntry, 0, c.cache.Len())
	for _, key := range c.cache.Keys() {
		value, ok := c.cache.Peek(key)
		if !ok {
			continue
		}
		entries = append(entries, persistedCacheEntry{
			Key:       key,
			Embedding: slices.Clone(value),
		})
	}

	dir := filepath.Dir(c.cachePath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create cache directory %s: %w", dir, err)
		}
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(c.cachePath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp cache file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	persisted := persistedCache{
		Version: 1,
		BaseURL: c.baseURL,
		Model:   c.model,
		Entries: entries,
	}
	if err := gob.NewEncoder(tmp).Encode(&persisted); err != nil {
		tmp.Close()
		return fmt.Errorf("encode cache %s: %w", c.cachePath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp cache file %s: %w", tmpPath, err)
	}
	if err := os.Remove(c.cachePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove old cache file %s: %w", c.cachePath, err)
	}
	if err := os.Rename(tmpPath, c.cachePath); err != nil {
		return fmt.Errorf("replace cache file %s: %w", c.cachePath, err)
	}
	return nil
}
