package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/philippgille/chromem-go"
	_ "modernc.org/sqlite"
)

type config struct {
	EmbeddingAPI       embeddingAPIConfig `json:"embedding_api"`
	SQLitePath         string             `json:"sqlite_path"`
	VectorDBPath       string             `json:"vector_db_path"`
	CollectionName     string             `json:"collection_name"`
	RequestTimeoutSecs int                `json:"request_timeout_seconds"`
	BatchSize          int                `json:"batch_size"`
	Concurrency        int                `json:"concurrency"`
}

type embeddingAPIConfig struct {
	BaseURL           string `json:"base_url"`
	Model             string `json:"model"`
	APIKey            string `json:"api_key"`
	RequestsPerMinute int    `json:"requests_per_minute"`
}

type themeRecord struct {
	ID                      int64
	ThemeID                 sql.NullInt64
	Title                   string
	ProjectDescription      string
	TeacherName             string
	TeacherPinyin           string
	ThemeSubjectArea        string
	ThemeSubjectAreaSub     string
	ThemeType               string
	ThemeProjectType        string
	ThemeSource             string
	ThemeState              string
	DeptName                string
	ThemeUniversity         string
	SourceFile              string
	ThemePrerequisiteSkills string
}

type embeddingClient struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
	limiter <-chan time.Time
}

func main() {
	configPath := flag.String("config", "config/vector-db.example.json", "path to vector database JSON config")
	sqlitePath := flag.String("sqlite", "", "override SQLite input path from config")
	outPath := flag.String("out", "", "override vector DB output directory from config")
	incremental := flag.Bool("incremental", false, "preserve existing vector DB and add only absent vector documents")
	flag.Parse()

	if err := run(*configPath, *sqlitePath, *outPath, *incremental); err != nil {
		log.Fatal(err)
	}
}

func run(configPath, sqliteOverride, outOverride string, incremental bool) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	if sqliteOverride != "" {
		cfg.SQLitePath = sqliteOverride
	}
	if outOverride != "" {
		cfg.VectorDBPath = outOverride
	}
	applyDefaults(&cfg)
	if err := validateConfig(cfg); err != nil {
		return err
	}

	records, err := loadThemes(cfg.SQLitePath)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return errors.New("no themes found in SQLite database")
	}

	if !incremental {
		if err := os.RemoveAll(cfg.VectorDBPath); err != nil {
			return fmt.Errorf("remove existing vector DB directory: %w", err)
		}
	}
	if err := os.MkdirAll(cfg.VectorDBPath, 0o755); err != nil {
		return fmt.Errorf("create vector DB directory: %w", err)
	}

	embedder := newEmbeddingClient(cfg)
	db, err := chromem.NewPersistentDB(cfg.VectorDBPath, false)
	if err != nil {
		return fmt.Errorf("create persistent chromem DB: %w", err)
	}
	metadata := map[string]string{
		"source":          cfg.SQLitePath,
		"embedding_model": cfg.EmbeddingAPI.Model,
	}
	var collection *chromem.Collection
	if incremental {
		collection, err = db.GetOrCreateCollection(cfg.CollectionName, metadata, embedder.embed)
	} else {
		collection, err = db.CreateCollection(cfg.CollectionName, metadata, embedder.embed)
	}
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}

	ctx := context.Background()
	total := 0
	skipped := 0
	for start := 0; start < len(records); start += cfg.BatchSize {
		end := min(start+cfg.BatchSize, len(records))
		docs := make([]chromem.Document, 0, end-start)
		for _, record := range records[start:end] {
			doc := documentFromTheme(record)
			if incremental {
				if _, err := collection.GetByID(ctx, doc.ID); err == nil {
					skipped++
					continue
				}
			}
			docs = append(docs, doc)
		}
		if len(docs) == 0 {
			log.Printf("embedded %d/%d themes (%d skipped)", total, len(records), skipped)
			continue
		}
		if err := collection.AddDocuments(ctx, docs, cfg.Concurrency); err != nil {
			return fmt.Errorf("add documents %d-%d: %w", start+1, end, err)
		}
		total += len(docs)
		log.Printf("embedded %d/%d themes (%d skipped)", total, len(records), skipped)
	}

	if incremental {
		log.Printf("updated vector DB at %s with collection %q: %d embedded, %d skipped", cfg.VectorDBPath, cfg.CollectionName, total, skipped)
	} else {
		log.Printf("created vector DB at %s with collection %q and %d documents", cfg.VectorDBPath, cfg.CollectionName, total)
	}
	return nil
}

func loadConfig(path string) (config, error) {
	f, err := os.Open(path)
	if err != nil {
		return config{}, fmt.Errorf("open config %s: %w", path, err)
	}
	defer f.Close()

	var cfg config
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return config{}, fmt.Errorf("decode config %s: %w", path, err)
	}
	return cfg, nil
}

func applyDefaults(cfg *config) {
	if cfg.SQLitePath == "" {
		cfg.SQLitePath = "fyp-data.sqlite"
	}
	if cfg.VectorDBPath == "" {
		cfg.VectorDBPath = "theme-vectors"
	}
	if cfg.CollectionName == "" {
		cfg.CollectionName = "themes"
	}
	if cfg.RequestTimeoutSecs == 0 {
		cfg.RequestTimeoutSecs = 120
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 16
	}
	if cfg.Concurrency == 0 {
		cfg.Concurrency = 1
	}
}

func validateConfig(cfg config) error {
	var missing []string
	if cfg.EmbeddingAPI.BaseURL == "" {
		missing = append(missing, "embedding_api.base_url")
	}
	if cfg.EmbeddingAPI.Model == "" {
		missing = append(missing, "embedding_api.model")
	}
	if cfg.EmbeddingAPI.APIKey == "" {
		missing = append(missing, "embedding_api.api_key")
	}
	if cfg.EmbeddingAPI.RequestsPerMinute <= 0 {
		missing = append(missing, "embedding_api.requests_per_minute")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing or invalid config fields: %s", strings.Join(missing, ", "))
	}
	if cfg.EmbeddingAPI.BaseURL == "https://api.example.com/v1" ||
		cfg.EmbeddingAPI.Model == "text-embedding-model-name" ||
		cfg.EmbeddingAPI.APIKey == "replace-with-your-api-key" {
		return errors.New("config/vector-db.example.json contains placeholder embedding API values; copy it or edit it with real base_url, model, and api_key values")
	}
	if cfg.BatchSize < 1 {
		return errors.New("batch_size must be at least 1")
	}
	if cfg.Concurrency < 1 {
		return errors.New("concurrency must be at least 1")
	}
	if cfg.RequestTimeoutSecs < 1 {
		return errors.New("request_timeout_seconds must be at least 1")
	}
	return nil
}

func loadThemes(sqlitePath string) ([]themeRecord, error) {
	db, err := sql.Open("sqlite", sqlitePath)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT
			id,
			themeId,
			themeTitle,
			themeProjectDescription,
			teacherName,
			teacherPinyin,
			themeSubjectArea,
			themeSubjectAreaSub,
			themeType,
			themeProjectType,
			themeSource,
			themeState,
			deptName,
			themeUniversity,
			source_file,
			themePrerequisiteSkills
		FROM themes
		WHERE missing = 0
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("query themes: %w", err)
	}
	defer rows.Close()

	var records []themeRecord
	for rows.Next() {
		var r themeRecord
		var title sql.NullString
		var projectDescription sql.NullString
		var teacherName sql.NullString
		var teacherPinyin sql.NullString
		var themeSubjectArea sql.NullString
		var themeSubjectAreaSub sql.NullString
		var themeType sql.NullString
		var themeProjectType sql.NullString
		var themeSource sql.NullString
		var themeState sql.NullString
		var deptName sql.NullString
		var themeUniversity sql.NullString
		var sourceFile sql.NullString
		var themePrerequisiteSkills sql.NullString
		if err := rows.Scan(
			&r.ID,
			&r.ThemeID,
			&title,
			&projectDescription,
			&teacherName,
			&teacherPinyin,
			&themeSubjectArea,
			&themeSubjectAreaSub,
			&themeType,
			&themeProjectType,
			&themeSource,
			&themeState,
			&deptName,
			&themeUniversity,
			&sourceFile,
			&themePrerequisiteSkills,
		); err != nil {
			return nil, fmt.Errorf("scan theme: %w", err)
		}
		r.Title = nullString(title)
		r.ProjectDescription = nullString(projectDescription)
		r.TeacherName = nullString(teacherName)
		r.TeacherPinyin = nullString(teacherPinyin)
		r.ThemeSubjectArea = nullString(themeSubjectArea)
		r.ThemeSubjectAreaSub = nullString(themeSubjectAreaSub)
		r.ThemeType = nullString(themeType)
		r.ThemeProjectType = nullString(themeProjectType)
		r.ThemeSource = nullString(themeSource)
		r.ThemeState = nullString(themeState)
		r.DeptName = nullString(deptName)
		r.ThemeUniversity = nullString(themeUniversity)
		r.SourceFile = nullString(sourceFile)
		r.ThemePrerequisiteSkills = nullString(themePrerequisiteSkills)
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate themes: %w", err)
	}
	return records, nil
}

func documentFromTheme(r themeRecord) chromem.Document {
	themeID := ""
	if r.ThemeID.Valid {
		themeID = fmt.Sprint(r.ThemeID.Int64)
	}
	content := strings.TrimSpace(r.Title + "\n\n" + r.ProjectDescription)
	return chromem.Document{
		ID: fmt.Sprintf("theme:%d", r.ID),
		Metadata: map[string]string{
			"sqlite_id":                   fmt.Sprint(r.ID),
			"theme_id":                    themeID,
			"title":                       r.Title,
			"teacher_name":                r.TeacherName,
			"teacher_pinyin":              r.TeacherPinyin,
			"theme_subject_area":          r.ThemeSubjectArea,
			"theme_subject_area_sub":      r.ThemeSubjectAreaSub,
			"theme_type":                  r.ThemeType,
			"theme_project_type":          r.ThemeProjectType,
			"theme_source":                r.ThemeSource,
			"theme_state":                 r.ThemeState,
			"dept_name":                   r.DeptName,
			"theme_university":            r.ThemeUniversity,
			"source_file":                 r.SourceFile,
			"theme_prerequisite_skills":   r.ThemePrerequisiteSkills,
			"embedded_fields":             "themeTitle,themeProjectDescription",
			"embedding_content_separator": "blank_line",
		},
		Content: content,
	}
}

func newEmbeddingClient(cfg config) *embeddingClient {
	return &embeddingClient{
		baseURL: strings.TrimRight(cfg.EmbeddingAPI.BaseURL, "/"),
		model:   cfg.EmbeddingAPI.Model,
		apiKey:  cfg.EmbeddingAPI.APIKey,
		client: &http.Client{
			Timeout: time.Duration(cfg.RequestTimeoutSecs) * time.Second,
		},
		limiter: rateLimiter(cfg.EmbeddingAPI.RequestsPerMinute),
	}
}

func rateLimiter(requestsPerMinute int) <-chan time.Time {
	interval := time.Minute / time.Duration(requestsPerMinute)
	if interval <= 0 {
		interval = time.Nanosecond
	}
	return time.Tick(interval)
}

func (c *embeddingClient) embed(ctx context.Context, text string) ([]float32, error) {
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
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send embedding request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embedding response: %w", err)
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
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	if len(parsed.Data) == 0 || len(parsed.Data[0].Embedding) == 0 {
		return nil, errors.New("embedding API response did not contain an embedding")
	}
	return parsed.Data[0].Embedding, nil
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func nullString(s sql.NullString) string {
	if !s.Valid {
		return ""
	}
	return s.String
}

func init() {
	log.SetFlags(log.LstdFlags)
}
