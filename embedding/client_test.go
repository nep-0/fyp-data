package embedding

import (
	"path/filepath"
	"slices"
	"testing"

	"fyp-data/config"
)

func TestClientCachePersistence(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "embedding-cache.gob")
	cfg := config.Embedding{
		BaseURL:            "https://example.test/v1",
		Model:              "test-model",
		APIKey:             "secret",
		RequestTimeoutSecs: 1,
		RequestsPerMinute:  60,
		CacheSize:          2,
		CachePath:          cachePath,
	}

	original := NewClient(cfg)
	original.cache.Add(original.baseURL+"\x00"+original.model+"\x00"+"hello", []float32{1, 2, 3})
	original.cache.Add(original.baseURL+"\x00"+original.model+"\x00"+"world", []float32{4, 5, 6})

	if err := original.SaveCache(); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	reloaded := NewClient(cfg)
	got, ok := reloaded.cache.Get(reloaded.baseURL + "\x00" + reloaded.model + "\x00" + "hello")
	if !ok {
		t.Fatal("expected persisted entry to be loaded")
	}
	if !slices.Equal(got, []float32{1, 2, 3}) {
		t.Fatalf("unexpected embedding: got %v", got)
	}
}
