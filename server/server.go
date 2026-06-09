package server

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"fyp-data/config"
	"fyp-data/embedding"
	"github.com/philippgille/chromem-go"
	_ "modernc.org/sqlite"
)

type App struct {
	cfg       config.API
	sqlite    *sql.DB
	vector    *chromem.Collection
	vectorErr string
	labels    dictionaryLabels
}

func Run(configPath string) error {
	a, err := New(configPath)
	if err != nil {
		return err
	}
	defer a.Close()

	mux := http.NewServeMux()
	a.routes(mux)

	log.Printf("serving API on http://%s", a.cfg.ListenAddr)
	return http.ListenAndServe(a.cfg.ListenAddr, logRequests(mux))
}

func New(configPath string) (*App, error) {
	cfg, err := config.LoadAPI(configPath)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", cfg.SQLitePath)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping SQLite database: %w", err)
	}

	labels, err := loadDictionaryLabels(db)
	if err != nil {
		db.Close()
		return nil, err
	}

	a := &App{cfg: cfg, sqlite: db, labels: labels}
	if cfg.VectorDBPath != "" {
		vectorDB, err := chromem.NewPersistentDB(cfg.VectorDBPath, false)
		if err != nil {
			a.vectorErr = err.Error()
		} else {
			var embed chromem.EmbeddingFunc
			if config.EmbeddingConfigured(cfg.EmbeddingAPI) {
				embed = embedding.NewClient(cfg.EmbeddingAPI).Embed
			}
			a.vector = vectorDB.GetCollection(cfg.CollectionName, embed)
			if a.vector == nil {
				a.vectorErr = "collection not found"
			}
		}
	}
	return a, nil
}

func (a *App) Close() error {
	return a.sqlite.Close()
}

func (a *App) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", a.handleHealth)
	mux.HandleFunc("GET /themes", a.handleThemes)
	mux.HandleFunc("GET /themes/{id}", a.handleTheme)
	mux.HandleFunc("GET /dictionaries", a.handleDictionaries)
	mux.HandleFunc("GET /dictionary-types", a.handleDictionaryTypes)
	mux.HandleFunc("GET /search", a.handleSearch)
	mux.HandleFunc("GET /semantic-search", a.handleSemanticSearch)
	mux.HandleFunc("GET /", handleFrontend)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	counts, err := a.counts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                 true,
		"sqlite_path":        a.cfg.SQLitePath,
		"theme_count":        counts["themes"],
		"dictionary_count":   counts["dictionaries"],
		"vector_db_path":     a.cfg.VectorDBPath,
		"vector_collection":  a.cfg.CollectionName,
		"vector_loaded":      a.vector != nil,
		"vector_load_error":  a.vectorErr,
		"semantic_available": a.vector != nil && config.EmbeddingConfigured(a.cfg.EmbeddingAPI),
	})
}

func (a *App) handleThemes(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := pagination(r, 50, 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	where, args := themeFilters(r)
	total, err := countRows(r.Context(), a.sqlite, "themes", where, args)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	query := "SELECT " + themeSelectColumns + " FROM themes"
	if where != "" {
		query += " WHERE " + where
	}
	query += " ORDER BY id LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := a.sqlite.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	themes, err := scanThemes(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	a.enrichThemes(themes)
	writeJSON(w, http.StatusOK, map[string]any{
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"rows":   themes,
	})
}

func (a *App) handleTheme(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, errors.New("invalid theme id"))
		return
	}
	rows, err := a.sqlite.QueryContext(r.Context(), "SELECT "+themeSelectColumns+" FROM themes WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	themes, err := scanThemes(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if len(themes) == 0 {
		writeError(w, http.StatusNotFound, errors.New("theme not found"))
		return
	}
	a.enrichThemes(themes)
	writeJSON(w, http.StatusOK, themes[0])
}

func (a *App) handleDictionaries(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := pagination(r, 100, 500)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	where := ""
	var args []any
	if dictType := strings.TrimSpace(r.URL.Query().Get("type")); dictType != "" {
		where = "dictType = ?"
		args = append(args, dictType)
	}
	total, err := countRows(r.Context(), a.sqlite, "dictionaries", where, args)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	query := "SELECT " + dictSelectColumns + " FROM dictionaries"
	if where != "" {
		query += " WHERE " + where
	}
	query += " ORDER BY dictType, dictSort, id LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := a.sqlite.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	dicts, err := scanDictionaries(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"rows":   dicts,
	})
}

func (a *App) handleDictionaryTypes(w http.ResponseWriter, r *http.Request) {
	rows, err := a.sqlite.QueryContext(r.Context(), `SELECT dictType, COUNT(*) FROM dictionaries GROUP BY dictType ORDER BY dictType`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var typ string
		var count int64
		if err := rows.Scan(&typ, &count); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		out = append(out, map[string]any{"dictType": typ, "count": count})
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing q parameter"))
		return
	}
	limit, offset, err := pagination(r, 25, 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	like := "%" + q + "%"
	searchWhere := `(themeTitle LIKE ? OR themeProjectDescription LIKE ? OR teacherName LIKE ? OR deptName LIKE ?)`
	args := []any{like, like, like, like}
	filterWhere, filterArgs := themeFilters(r)
	where := searchWhere
	if filterWhere != "" {
		where += " AND " + filterWhere
		args = append(args, filterArgs...)
	}
	total, err := countRows(r.Context(), a.sqlite, "themes", where, args)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	rows, err := a.sqlite.QueryContext(
		r.Context(),
		"SELECT "+themeSelectColumns+" FROM themes WHERE "+where+" ORDER BY id LIMIT ? OFFSET ?",
		append(args, limit, offset)...,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	themes, err := scanThemes(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	a.enrichThemes(themes)
	writeJSON(w, http.StatusOK, map[string]any{
		"query":  q,
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"rows":   themes,
	})
}

func (a *App) handleSemanticSearch(w http.ResponseWriter, r *http.Request) {
	if a.vector == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("vector DB unavailable: %s", a.vectorErr))
		return
	}
	if !config.EmbeddingConfigured(a.cfg.EmbeddingAPI) {
		writeError(w, http.StatusServiceUnavailable, errors.New("embedding API is not configured for semantic search"))
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing q parameter"))
		return
	}
	limit, err := intParam(r, "limit", 10, 1, 50)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	negative := strings.TrimSpace(r.URL.Query().Get("negative"))
	options := chromem.QueryOptions{
		QueryText: q,
		NResults:  limit,
	}
	if negative != "" {
		options.Negative = chromem.NegativeQueryOptions{
			Mode: chromem.NEGATIVE_MODE_SUBTRACT,
			Text: negative,
		}
	}
	results, err := a.vector.QueryWithOptions(r.Context(), options)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ids := make([]int64, 0, len(results))
	similarities := make(map[int64]float32, len(results))
	for _, result := range results {
		sqliteID, err := strconv.ParseInt(result.Metadata["sqlite_id"], 10, 64)
		if err != nil {
			continue
		}
		ids = append(ids, sqliteID)
		similarities[sqliteID] = result.Similarity
	}
	themes, err := a.themesByIDs(r.Context(), ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	a.enrichThemes(themes)
	rows := make([]map[string]any, 0, len(themes))
	for _, t := range themes {
		rows = append(rows, map[string]any{
			"similarity": similarities[t.ID],
			"theme":      t,
		})
	}
	response := map[string]any{
		"query": q,
		"rows":  rows,
	}
	if negative != "" {
		response["negative"] = negative
		response["negative_mode"] = chromem.NEGATIVE_MODE_SUBTRACT
	}
	writeJSON(w, http.StatusOK, response)
}

func loadDictionaryLabels(db *sql.DB) (dictionaryLabels, error) {
	rows, err := db.Query(`
		SELECT dictType, dictValue, dictLabel, dictLabelEn, label
		FROM dictionaries
		ORDER BY dictType, dictValue
	`)
	if err != nil {
		return nil, fmt.Errorf("load dictionary labels: %w", err)
	}
	defer rows.Close()

	labels := make(dictionaryLabels)
	for rows.Next() {
		var dictType, dictValue sql.NullString
		var dictLabel, dictLabelEn, label sql.NullString
		if err := rows.Scan(&dictType, &dictValue, &dictLabel, &dictLabelEn, &label); err != nil {
			return nil, fmt.Errorf("scan dictionary label: %w", err)
		}
		if !dictType.Valid || !dictValue.Valid {
			continue
		}
		if labels[dictType.String] == nil {
			labels[dictType.String] = make(map[string]dictionaryLabel)
		}
		display := nullString(dictLabel)
		if display == "" {
			display = nullString(label)
		}
		labels[dictType.String][dictValue.String] = dictionaryLabel{
			Label:   display,
			LabelEn: nullString(dictLabelEn),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dictionary labels: %w", err)
	}
	return labels, nil
}

func (a *App) enrichThemes(themes []theme) {
	for i := range themes {
		labels := map[string]any{}
		a.addLabel(labels, "themeSubjectArea", "theme_subject_area", themes[i].ThemeSubjectArea)
		a.addLabel(labels, "themeProjectType", "theme_project_type", themes[i].ThemeProjectType)
		a.addLabel(labels, "themeSource", "theme_source", themes[i].ThemeSource)
		a.addLabel(labels, "themeState", "subject_state", themes[i].ThemeState)
		a.addLabel(labels, "themeType", "theme_type", themes[i].ThemeType)
		a.addLabel(labels, "indResults", "ind_results", themes[i].IndResults)
		a.addLabel(labels, "indState", "subject_state", themes[i].IndState)
		a.addLabel(labels, "extendJb", "extend_jb_dict", themes[i].ExtendJb)
		addMultiLabels(labels, "themeProgramme", a.lookupMany("theme_programme", themes[i].ThemeProgramme))

		if themes[i].ThemeSubjectArea != "" && themes[i].ThemeSubjectAreaSub != "" {
			a.addLabel(labels, "themeSubjectAreaSub", "theme_subject_area_"+themes[i].ThemeSubjectArea, themes[i].ThemeSubjectAreaSub)
		}
		if themes[i].ThemeSource != "" && themes[i].ThemeSourceSub != "" {
			a.addLabel(labels, "themeSourceSub", "theme_source_"+themes[i].ThemeSource, themes[i].ThemeSourceSub)
		}
		if len(labels) > 0 {
			themes[i].Labels = labels
		}
	}
}

func (a *App) lookup(dictType, value string) (dictionaryLabel, bool) {
	if value == "" {
		return dictionaryLabel{}, false
	}
	byValue := a.labels[dictType]
	if byValue == nil {
		return dictionaryLabel{}, false
	}
	label, ok := byValue[value]
	return label, ok
}

func (a *App) lookupMany(dictType, raw string) []dictionaryLabel {
	var out []dictionaryLabel
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if label, ok := a.lookup(dictType, part); ok {
			out = append(out, label)
		}
	}
	return out
}

func (a *App) addLabel(labels map[string]any, field, dictType, value string) {
	if label, ok := a.lookup(dictType, value); ok {
		labels[field] = label
	}
}

func addMultiLabels(labels map[string]any, field string, values []dictionaryLabel) {
	if len(values) > 0 {
		labels[field] = values
	}
}
