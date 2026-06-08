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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/philippgille/chromem-go"
	_ "modernc.org/sqlite"
)

type apiConfig struct {
	ListenAddr     string             `json:"listen_addr"`
	SQLitePath     string             `json:"sqlite_path"`
	VectorDBPath   string             `json:"vector_db_path"`
	CollectionName string             `json:"collection_name"`
	EmbeddingAPI   apiEmbeddingConfig `json:"embedding_api"`
}

type apiEmbeddingConfig struct {
	BaseURL            string `json:"base_url"`
	Model              string `json:"model"`
	APIKey             string `json:"api_key"`
	RequestsPerMinute  int    `json:"requests_per_minute"`
	RequestTimeoutSecs int    `json:"request_timeout_seconds"`
}

type app struct {
	cfg       apiConfig
	sqlite    *sql.DB
	vector    *chromem.Collection
	vectorErr string
	labels    dictionaryLabels
}

type dictionaryLabels map[string]map[string]dictionaryLabel

type dictionaryLabel struct {
	Label   string `json:"label"`
	LabelEn string `json:"label_en"`
}

type theme struct {
	ID                      int64           `json:"id"`
	SourceFile              string          `json:"source_file"`
	RawJSON                 json.RawMessage `json:"raw_json,omitempty"`
	ThemeSubjectArea        string          `json:"themeSubjectArea"`
	DelFlag                 string          `json:"delFlag"`
	Status                  string          `json:"status"`
	ThemePrerequisiteSkills string          `json:"themePrerequisiteSkills"`
	ThemeProjectDescription string          `json:"themeProjectDescription"`
	ThemeProjectType        string          `json:"themeProjectType"`
	ThemeSourceRemarks      string          `json:"themeSourceRemarks"`
	ThemeSourceSub          string          `json:"themeSourceSub"`
	ThemeSource             string          `json:"themeSource"`
	ThemeSubjectAreaSub     string          `json:"themeSubjectAreaSub"`
	ThemeID                 *int64          `json:"themeId"`
	ThemeProgramme          string          `json:"themeProgramme"`
	ThemeCommitmentCheck    string          `json:"themeCommitmentCheck"`
	ThemeOfficeLocation     string          `json:"themeOfficeLocation"`
	ThemeExpertID           *int64          `json:"themeExpertId"`
	ThemeCheckTime          *int64          `json:"themeCheckTime"`
	ThemeCheckComment       string          `json:"themeCheckComment"`
	ThemeCount              *int64          `json:"themeCount"`
	ThemeTeacherID          *int64          `json:"themeTeacherId"`
	ThemeState              string          `json:"themeState"`
	ThemeIsSelect           string          `json:"themeIsselect"`
	ThemeType               string          `json:"themeType"`
	ThemeTitle              string          `json:"themeTitle"`
	ThemeProID              *int64          `json:"themeProId"`
	ThemeUniversity         string          `json:"themeUniversity"`
	IndStudentID            *int64          `json:"indStudentId"`
	TeacherName             string          `json:"teacherName"`
	TeacherPinyin           string          `json:"teacherPinyin"`
	UserType                string          `json:"userType"`
	StudentName             string          `json:"studentName"`
	StudentPinyin           string          `json:"studentPinyin"`
	GUID                    string          `json:"guid"`
	PhoneNumber             string          `json:"phonenumber"`
	Email                   string          `json:"email"`
	SelectCount             string          `json:"selectCount"`
	CreateBy                string          `json:"createBy"`
	CreateTime              string          `json:"createTime"`
	UpdateBy                string          `json:"updateBy"`
	UpdateTime              string          `json:"updateTime"`
	ThemeSubmitTime         string          `json:"themeSubmitTime"`
	IndResults              string          `json:"indResults"`
	IndState                string          `json:"indState"`
	TeacherEmail            string          `json:"teacherEmail"`
	TeacherChineseEnglish   string          `json:"teacherChineseEnglish"`
	DeptName                string          `json:"deptName"`
	DeptNameEn              string          `json:"deptName_en"`
	JobType                 string          `json:"jobType"`
	ExtendJb                string          `json:"extendJb"`
	ExtendSzyx              string          `json:"extendSzyx"`
	ExtendSzxy              string          `json:"extendSzxy"`
	Labels                  map[string]any  `json:"labels,omitempty"`
}

type dictionaryRow struct {
	ID          int64           `json:"id"`
	SourceFile  string          `json:"source_file"`
	RawJSON     json.RawMessage `json:"raw_json,omitempty"`
	DictCode    *int64          `json:"dictCode"`
	DictSort    *int64          `json:"dictSort"`
	DictLabel   string          `json:"dictLabel"`
	DictLabelEn string          `json:"dictLabelEn"`
	Label       string          `json:"label"`
	DictValue   string          `json:"dictValue"`
	DictType    string          `json:"dictType"`
	CSSClass    string          `json:"cssClass"`
	ListClass   string          `json:"listClass"`
	IsDefault   string          `json:"isDefault"`
	Status      string          `json:"status"`
	Remark      string          `json:"remark"`
	CreateBy    *int64          `json:"createBy"`
	CreateTime  string          `json:"createTime"`
	UpdateBy    *int64          `json:"updateBy"`
	UpdateTime  string          `json:"updateTime"`
}

func main() {
	configPath := flag.String("config", "config/api.example.json", "path to API JSON config")
	flag.Parse()

	a, err := newApp(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	defer a.sqlite.Close()

	mux := http.NewServeMux()
	a.routes(mux)

	log.Printf("serving API on http://%s", a.cfg.ListenAddr)
	if err := http.ListenAndServe(a.cfg.ListenAddr, logRequests(mux)); err != nil {
		log.Fatal(err)
	}
}

func newApp(configPath string) (*app, error) {
	cfg, err := loadAPIConfig(configPath)
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

	a := &app{cfg: cfg, sqlite: db, labels: labels}
	if cfg.VectorDBPath != "" {
		vectorDB, err := chromem.NewPersistentDB(cfg.VectorDBPath, false)
		if err != nil {
			a.vectorErr = err.Error()
		} else {
			var embed chromem.EmbeddingFunc
			if embeddingConfigured(cfg.EmbeddingAPI) {
				embed = newAPIEmbeddingClient(cfg.EmbeddingAPI).embed
			}
			a.vector = vectorDB.GetCollection(cfg.CollectionName, embed)
			if a.vector == nil {
				a.vectorErr = "collection not found"
			}
		}
	}
	return a, nil
}

func loadAPIConfig(path string) (apiConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return apiConfig{}, fmt.Errorf("open config %s: %w", path, err)
	}
	defer f.Close()

	var cfg apiConfig
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return apiConfig{}, fmt.Errorf("decode config %s: %w", path, err)
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:8080"
	}
	if cfg.SQLitePath == "" {
		cfg.SQLitePath = "fyp-data.sqlite"
	}
	if cfg.CollectionName == "" {
		cfg.CollectionName = "themes"
	}
	if cfg.EmbeddingAPI.RequestsPerMinute == 0 {
		cfg.EmbeddingAPI.RequestsPerMinute = 60
	}
	if cfg.EmbeddingAPI.RequestTimeoutSecs == 0 {
		cfg.EmbeddingAPI.RequestTimeoutSecs = 120
	}
	return cfg, nil
}

func (a *app) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", a.handleHealth)
	mux.HandleFunc("GET /themes", a.handleThemes)
	mux.HandleFunc("GET /themes/{id}", a.handleTheme)
	mux.HandleFunc("GET /dictionaries", a.handleDictionaries)
	mux.HandleFunc("GET /dictionary-types", a.handleDictionaryTypes)
	mux.HandleFunc("GET /search", a.handleSearch)
	mux.HandleFunc("GET /semantic-search", a.handleSemanticSearch)
	mux.HandleFunc("GET /", handleFrontend)
}

func handleFrontend(w http.ResponseWriter, r *http.Request) {
	dist := filepath.Join("frontend", "dist")
	assetPath := strings.TrimPrefix(filepath.Clean(r.URL.Path), string(os.PathSeparator))
	requested := filepath.Join(dist, assetPath)
	if info, err := os.Stat(requested); err == nil && !info.IsDir() {
		http.ServeFile(w, r, requested)
		return
	}

	index := filepath.Join(dist, "index.html")
	if _, err := os.Stat(index); err != nil {
		writeError(w, http.StatusNotFound, errors.New("frontend build not found; run npm run build in frontend"))
		return
	}
	http.ServeFile(w, r, index)
}

func (a *app) handleHealth(w http.ResponseWriter, r *http.Request) {
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
		"semantic_available": a.vector != nil && embeddingConfigured(a.cfg.EmbeddingAPI),
	})
}

func (a *app) handleThemes(w http.ResponseWriter, r *http.Request) {
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
	themes, err := scanThemes(rows, false)
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

func (a *app) handleTheme(w http.ResponseWriter, r *http.Request) {
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
	themes, err := scanThemes(rows, true)
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

func (a *app) handleDictionaries(w http.ResponseWriter, r *http.Request) {
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
	dicts, err := scanDictionaries(rows, false)
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

func (a *app) handleDictionaryTypes(w http.ResponseWriter, r *http.Request) {
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

func (a *app) handleSearch(w http.ResponseWriter, r *http.Request) {
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
	themes, err := scanThemes(rows, false)
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

func (a *app) handleSemanticSearch(w http.ResponseWriter, r *http.Request) {
	if a.vector == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("vector DB unavailable: %s", a.vectorErr))
		return
	}
	if !embeddingConfigured(a.cfg.EmbeddingAPI) {
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

func (a *app) counts(ctx context.Context) (map[string]int64, error) {
	out := make(map[string]int64)
	for _, table := range []string{"themes", "dictionaries"} {
		var count int64
		if err := a.sqlite.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return nil, err
		}
		out[table] = count
	}
	return out, nil
}

func (a *app) themesByIDs(ctx context.Context, ids []int64) ([]theme, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	byID := make(map[int64]theme, len(ids))
	for _, id := range ids {
		rows, err := a.sqlite.QueryContext(ctx, "SELECT "+themeSelectColumns+" FROM themes WHERE id = ?", id)
		if err != nil {
			return nil, err
		}
		themes, err := scanThemes(rows, false)
		rows.Close()
		if err != nil {
			return nil, err
		}
		if len(themes) > 0 {
			byID[id] = themes[0]
		}
	}
	ordered := make([]theme, 0, len(ids))
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			ordered = append(ordered, t)
		}
	}
	return ordered, nil
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

func (a *app) enrichThemes(themes []theme) {
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

func (a *app) lookup(dictType, value string) (dictionaryLabel, bool) {
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

func (a *app) lookupMany(dictType, raw string) []dictionaryLabel {
	var out []dictionaryLabel
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if label, ok := a.lookup(dictType, part); ok {
			out = append(out, label)
		}
	}
	return out
}

func (a *app) addLabel(labels map[string]any, field, dictType, value string) {
	if label, ok := a.lookup(dictType, value); ok {
		labels[field] = label
	}
}

func addMultiLabels(labels map[string]any, field string, values []dictionaryLabel) {
	if len(values) > 0 {
		labels[field] = values
	}
}

const themeSelectColumns = `
	id, source_file, raw_json, themeSubjectArea, delFlag, status, themePrerequisiteSkills,
	themeProjectDescription, themeProjectType, themeSourceRemarks, themeSourceSub, themeSource,
	themeSubjectAreaSub, themeId, themeProgramme, themeCommitmentCheck, themeOfficeLocation,
	themeExpertId, themeCheckTime, themeCheckComment, themeCount, themeTeacherId, themeState,
	themeIsselect, themeType, themeTitle, themeProId, themeUniversity, indStudentId, teacherName,
	teacherPinyin, userType, studentName, studentPinyin, guid, phonenumber, email, selectCount,
	createBy, createTime, updateBy, updateTime, themeSubmitTime, indResults, indState, teacherEmail,
	teacherChineseEnglish, deptName, deptName_en, jobType, extendJb, extendSzyx, extendSzxy`

const dictSelectColumns = `
	id, source_file, raw_json, dictCode, dictSort, dictLabel, dictLabelEn, label, dictValue,
	dictType, cssClass, listClass, isDefault, status, remark, createBy, createTime, updateBy, updateTime`

func scanThemes(rows *sql.Rows, includeRaw bool) ([]theme, error) {
	var themes []theme
	for rows.Next() {
		var t theme
		var raw string
		var themeID, themeExpertID, themeCheckTime, themeCount, themeTeacherID, themeProID, indStudentID sql.NullInt64
		fields := []*string{
			&t.ThemeSubjectArea, &t.DelFlag, &t.Status, &t.ThemePrerequisiteSkills,
			&t.ThemeProjectDescription, &t.ThemeProjectType, &t.ThemeSourceRemarks,
			&t.ThemeSourceSub, &t.ThemeSource, &t.ThemeSubjectAreaSub, &t.ThemeProgramme,
			&t.ThemeCommitmentCheck, &t.ThemeOfficeLocation, &t.ThemeCheckComment, &t.ThemeState,
			&t.ThemeIsSelect, &t.ThemeType, &t.ThemeTitle, &t.ThemeUniversity, &t.TeacherName,
			&t.TeacherPinyin, &t.UserType, &t.StudentName, &t.StudentPinyin, &t.GUID,
			&t.PhoneNumber, &t.Email, &t.SelectCount, &t.CreateBy, &t.CreateTime, &t.UpdateBy,
			&t.UpdateTime, &t.ThemeSubmitTime, &t.IndResults, &t.IndState, &t.TeacherEmail,
			&t.TeacherChineseEnglish, &t.DeptName, &t.DeptNameEn, &t.JobType, &t.ExtendJb,
			&t.ExtendSzyx, &t.ExtendSzxy,
		}
		nulls := make([]sql.NullString, len(fields))
		dest := []any{
			&t.ID, &t.SourceFile, &raw, &nulls[0], &nulls[1], &nulls[2], &nulls[3],
			&nulls[4], &nulls[5], &nulls[6], &nulls[7], &nulls[8], &nulls[9],
			&themeID, &nulls[10], &nulls[11], &nulls[12], &themeExpertID, &themeCheckTime,
			&nulls[13], &themeCount, &themeTeacherID, &nulls[14], &nulls[15], &nulls[16],
			&nulls[17], &themeProID, &nulls[18], &indStudentID, &nulls[19], &nulls[20],
			&nulls[21], &nulls[22], &nulls[23], &nulls[24], &nulls[25], &nulls[26],
			&nulls[27], &nulls[28], &nulls[29], &nulls[30], &nulls[31], &nulls[32],
			&nulls[33], &nulls[34], &nulls[35], &nulls[36], &nulls[37], &nulls[38],
			&nulls[39], &nulls[40], &nulls[41], &nulls[42],
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		t.SourceFile = nullString(sql.NullString{String: t.SourceFile, Valid: t.SourceFile != ""})
		for i := range fields {
			*fields[i] = nullString(nulls[i])
		}
		t.ThemeID = nullInt(themeID)
		t.ThemeExpertID = nullInt(themeExpertID)
		t.ThemeCheckTime = nullInt(themeCheckTime)
		t.ThemeCount = nullInt(themeCount)
		t.ThemeTeacherID = nullInt(themeTeacherID)
		t.ThemeProID = nullInt(themeProID)
		t.IndStudentID = nullInt(indStudentID)
		if includeRaw && raw != "" {
			t.RawJSON = json.RawMessage(raw)
		}
		themes = append(themes, t)
	}
	return themes, rows.Err()
}

func scanDictionaries(rows *sql.Rows, includeRaw bool) ([]dictionaryRow, error) {
	var out []dictionaryRow
	for rows.Next() {
		var d dictionaryRow
		var raw string
		var dictCode, dictSort, createBy, updateBy sql.NullInt64
		var dictLabel, dictLabelEn, label, dictValue, dictType, cssClass, listClass, isDefault, status, remark, createTime, updateTime sql.NullString
		if err := rows.Scan(
			&d.ID, &d.SourceFile, &raw, &dictCode, &dictSort, &dictLabel, &dictLabelEn,
			&label, &dictValue, &dictType, &cssClass, &listClass, &isDefault, &status,
			&remark, &createBy, &createTime, &updateBy, &updateTime,
		); err != nil {
			return nil, err
		}
		d.DictCode = nullInt(dictCode)
		d.DictSort = nullInt(dictSort)
		d.DictLabel = nullString(dictLabel)
		d.DictLabelEn = nullString(dictLabelEn)
		d.Label = nullString(label)
		d.DictValue = nullString(dictValue)
		d.DictType = nullString(dictType)
		d.CSSClass = nullString(cssClass)
		d.ListClass = nullString(listClass)
		d.IsDefault = nullString(isDefault)
		d.Status = nullString(status)
		d.Remark = nullString(remark)
		d.CreateBy = nullInt(createBy)
		d.CreateTime = nullString(createTime)
		d.UpdateBy = nullInt(updateBy)
		d.UpdateTime = nullString(updateTime)
		if includeRaw && raw != "" {
			d.RawJSON = json.RawMessage(raw)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func themeFilters(r *http.Request) (string, []any) {
	var clauses []string
	var args []any
	for param, column := range map[string]string{
		"state":        "themeState",
		"subject_area": "themeSubjectArea",
		"teacher_id":   "themeTeacherId",
		"project_type": "themeProjectType",
		"theme_type":   "themeType",
	} {
		if v := strings.TrimSpace(r.URL.Query().Get(param)); v != "" {
			clauses = append(clauses, column+" = ?")
			args = append(args, v)
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("programme")); v != "" {
		clauses = append(clauses, "',' || themeProgramme || ',' LIKE ?")
		args = append(args, "%,"+v+",%")
	}
	return strings.Join(clauses, " AND "), args
}

func countRows(ctx context.Context, db *sql.DB, table, where string, args []any) (int64, error) {
	query := "SELECT COUNT(*) FROM " + table
	if where != "" {
		query += " WHERE " + where
	}
	var count int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func pagination(r *http.Request, defaultLimit, maxLimit int) (int, int, error) {
	limit, err := intParam(r, "limit", defaultLimit, 1, maxLimit)
	if err != nil {
		return 0, 0, err
	}
	offset, err := intParam(r, "offset", 0, 0, 1_000_000)
	if err != nil {
		return 0, 0, err
	}
	return limit, offset, nil
}

func intParam(r *http.Request, name string, def, minValue, maxValue int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < minValue || n > maxValue {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minValue, maxValue)
	}
	return n, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.RequestURI(), time.Since(start).Round(time.Millisecond))
	})
}

func nullString(s sql.NullString) string {
	if !s.Valid {
		return ""
	}
	return s.String
}

func nullInt(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	return &n.Int64
}

func embeddingConfigured(cfg apiEmbeddingConfig) bool {
	return cfg.BaseURL != "" && cfg.Model != "" && cfg.APIKey != ""
}

type apiEmbeddingClient struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
	limiter <-chan time.Time
}

func newAPIEmbeddingClient(cfg apiEmbeddingConfig) *apiEmbeddingClient {
	return &apiEmbeddingClient{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		model:   cfg.Model,
		apiKey:  cfg.APIKey,
		client: &http.Client{
			Timeout: time.Duration(cfg.RequestTimeoutSecs) * time.Second,
		},
		limiter: rateLimiter(cfg.RequestsPerMinute),
	}
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

func (c *apiEmbeddingClient) embed(ctx context.Context, text string) ([]float32, error) {
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
	return parsed.Data[0].Embedding, nil
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
