package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type API struct {
	ListenAddr     string    `json:"listen_addr"`
	SQLitePath     string    `json:"sqlite_path"`
	VectorDBPath   string    `json:"vector_db_path"`
	CollectionName string    `json:"collection_name"`
	EmbeddingAPI   Embedding `json:"embedding_api"`
}

type Embedding struct {
	BaseURL            string `json:"base_url"`
	Model              string `json:"model"`
	APIKey             string `json:"api_key"`
	RequestsPerMinute  int    `json:"requests_per_minute"`
	RequestTimeoutSecs int    `json:"request_timeout_seconds"`
}

func LoadAPI(path string) (API, error) {
	f, err := os.Open(path)
	if err != nil {
		return API{}, fmt.Errorf("open config %s: %w", path, err)
	}
	defer f.Close()

	var cfg API
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return API{}, fmt.Errorf("decode config %s: %w", path, err)
	}
	cfg.ApplyDefaults()
	return cfg, nil
}

func (cfg *API) ApplyDefaults() {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:8080"
	}
	if cfg.SQLitePath == "" {
		cfg.SQLitePath = "fyp-data.sqlite"
	}
	if cfg.CollectionName == "" {
		cfg.CollectionName = "themes"
	}
	cfg.EmbeddingAPI.ApplyDefaults()
}

func (cfg *Embedding) ApplyDefaults() {
	if cfg.RequestsPerMinute == 0 {
		cfg.RequestsPerMinute = 60
	}
	if cfg.RequestTimeoutSecs == 0 {
		cfg.RequestTimeoutSecs = 120
	}
}

func EmbeddingConfigured(cfg Embedding) bool {
	return cfg.BaseURL != "" && cfg.Model != "" && cfg.APIKey != ""
}
