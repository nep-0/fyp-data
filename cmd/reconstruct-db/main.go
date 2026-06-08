package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	_ "modernc.org/sqlite"
)

var themeColumns = []string{
	"themeSubjectArea",
	"delFlag",
	"status",
	"themePrerequisiteSkills",
	"themeProjectDescription",
	"themeProjectType",
	"themeSourceRemarks",
	"themeSourceSub",
	"themeSource",
	"themeSubjectAreaSub",
	"themeId",
	"themeProgramme",
	"themeCommitmentCheck",
	"themeOfficeLocation",
	"themeExpertId",
	"themeCheckTime",
	"themeCheckComment",
	"themeCount",
	"themeTeacherId",
	"themeState",
	"themeIsselect",
	"themeType",
	"themeTitle",
	"themeProId",
	"themeUniversity",
	"indStudentId",
	"teacherName",
	"teacherPinyin",
	"userType",
	"studentName",
	"studentPinyin",
	"guid",
	"phonenumber",
	"email",
	"selectCount",
	"createBy",
	"createTime",
	"updateBy",
	"updateTime",
	"themeSubmitTime",
	"indResults",
	"indState",
	"teacherEmail",
	"teacherChineseEnglish",
	"deptName",
	"deptName_en",
	"jobType",
	"extendJb",
	"extendSzyx",
	"extendSzxy",
}

var dictColumns = []string{
	"dictCode",
	"dictSort",
	"dictLabel",
	"dictLabelEn",
	"label",
	"dictValue",
	"dictType",
	"cssClass",
	"listClass",
	"isDefault",
	"status",
	"remark",
	"createBy",
	"createTime",
	"updateBy",
	"updateTime",
}

var defaultThemeFiles = []string{"p1.json", "p2.json", "p3.json", "p4.json"}

func main() {
	dataDir := flag.String("data", "data", "directory containing source JSON files")
	outPath := flag.String("out", "fyp-data.sqlite", "SQLite database to create")
	includeSmall := flag.Bool("include-small", false, "also import small.json into themes")
	smallOnly := flag.Bool("small-only", false, "import only small.json into themes")
	flag.Parse()

	if err := run(*dataDir, *outPath, *includeSmall, *smallOnly); err != nil {
		log.Fatal(err)
	}
}

func run(dataDir, outPath string, includeSmall, smallOnly bool) error {
	if err := os.Remove(outPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove existing database: %w", err)
	}

	db, err := sql.Open("sqlite", outPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := createSchema(db); err != nil {
		return err
	}

	if _, err := db.Exec("BEGIN"); err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = db.Exec("ROLLBACK")
		}
	}()

	themeFiles := slices.Clone(defaultThemeFiles)
	if smallOnly {
		themeFiles = []string{"small.json"}
	} else if includeSmall {
		themeFiles = append(themeFiles, "small.json")
	}

	totalThemes := 0
	for _, name := range themeFiles {
		n, err := importThemes(db, filepath.Join(dataDir, name), name)
		if err != nil {
			return err
		}
		totalThemes += n
		log.Printf("imported %d theme rows from %s", n, name)
	}

	totalDicts := 0
	dictFiles, err := discoverDictFiles(dataDir)
	if err != nil {
		return err
	}
	for _, path := range dictFiles {
		source := filepath.Base(path)
		n, err := importDictionary(db, path, source)
		if err != nil {
			return err
		}
		totalDicts += n
		log.Printf("imported %d dictionary rows from %s", n, source)
	}

	if _, err := db.Exec("COMMIT"); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	committed = true

	if err := createIndexes(db); err != nil {
		return err
	}
	log.Printf("created %s with %d theme rows and %d dictionary rows", outPath, totalThemes, totalDicts)
	return nil
}

func createSchema(db *sql.DB) error {
	var b strings.Builder
	b.WriteString("CREATE TABLE themes (\n")
	b.WriteString("  id INTEGER PRIMARY KEY AUTOINCREMENT,\n")
	b.WriteString("  source_file TEXT NOT NULL,\n")
	b.WriteString("  raw_json TEXT NOT NULL,\n")
	for i, col := range themeColumns {
		typ := "TEXT"
		switch col {
		case "themeId", "themeExpertId", "themeCheckTime", "themeCount", "themeTeacherId", "themeProId", "indStudentId":
			typ = "INTEGER"
		}
		fmt.Fprintf(&b, "  %q %s", col, typ)
		if i < len(themeColumns)-1 {
			b.WriteString(",")
		}
		b.WriteByte('\n')
	}
	b.WriteString(");\n")

	b.WriteString("CREATE TABLE dictionaries (\n")
	b.WriteString("  id INTEGER PRIMARY KEY AUTOINCREMENT,\n")
	b.WriteString("  source_file TEXT NOT NULL,\n")
	b.WriteString("  raw_json TEXT NOT NULL,\n")
	for i, col := range dictColumns {
		typ := "TEXT"
		switch col {
		case "dictCode", "dictSort", "createBy", "updateBy":
			typ = "INTEGER"
		}
		fmt.Fprintf(&b, "  %q %s", col, typ)
		if i < len(dictColumns)-1 {
			b.WriteString(",")
		}
		b.WriteByte('\n')
	}
	b.WriteString(");\n")

	if _, err := db.Exec(b.String()); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}

func createIndexes(db *sql.DB) error {
	stmts := []string{
		`CREATE INDEX idx_themes_theme_id ON themes(themeId)`,
		`CREATE INDEX idx_themes_teacher_id ON themes(themeTeacherId)`,
		`CREATE INDEX idx_themes_state ON themes(themeState)`,
		`CREATE INDEX idx_themes_subject_area ON themes(themeSubjectArea, themeSubjectAreaSub)`,
		`CREATE INDEX idx_dictionaries_type_value ON dictionaries(dictType, dictValue)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("create index %q: %w", stmt, err)
		}
	}
	return nil
}

func importThemes(db *sql.DB, path, source string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.UseNumber()
	if err := advanceToRows(dec); err != nil {
		return 0, fmt.Errorf("find rows in %s: %w", path, err)
	}

	stmt, err := db.Prepare(insertSQL("themes", []string{"source_file", "raw_json"}, themeColumns))
	if err != nil {
		return 0, fmt.Errorf("prepare theme insert: %w", err)
	}
	defer stmt.Close()

	count := 0
	for dec.More() {
		var row map[string]any
		if err := dec.Decode(&row); err != nil {
			return count, fmt.Errorf("decode row %d in %s: %w", count+1, path, err)
		}
		args, err := values(source, themeColumns, row)
		if err != nil {
			return count, fmt.Errorf("prepare row %d from %s: %w", count+1, path, err)
		}
		if _, err := stmt.Exec(args...); err != nil {
			return count, fmt.Errorf("insert row %d from %s: %w", count+1, path, err)
		}
		count++
	}
	return count, nil
}

func importDictionary(db *sql.DB, path, source string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.UseNumber()
	if err := advanceToDataArray(dec); err != nil {
		return 0, fmt.Errorf("find data array in %s: %w", path, err)
	}

	stmt, err := db.Prepare(insertSQL("dictionaries", []string{"source_file", "raw_json"}, dictColumns))
	if err != nil {
		return 0, fmt.Errorf("prepare dictionary insert: %w", err)
	}
	defer stmt.Close()

	count := 0
	for dec.More() {
		var row map[string]any
		if err := dec.Decode(&row); err != nil {
			return count, fmt.Errorf("decode row %d in %s: %w", count+1, path, err)
		}
		args, err := values(source, dictColumns, row)
		if err != nil {
			return count, fmt.Errorf("prepare row %d from %s: %w", count+1, path, err)
		}
		if _, err := stmt.Exec(args...); err != nil {
			return count, fmt.Errorf("insert row %d from %s: %w", count+1, path, err)
		}
		count++
	}
	return count, nil
}

func advanceToRows(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim('{') {
		return fmt.Errorf("expected root object, got %v", tok)
	}

	for dec.More() {
		key, err := stringToken(dec)
		if err != nil {
			return err
		}
		if key != "data" {
			if err := skipValue(dec); err != nil {
				return err
			}
			continue
		}
		return advanceToRowsInData(dec)
	}
	return errors.New("missing data.rows array")
}

func advanceToRowsInData(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim('{') {
		return fmt.Errorf("expected data object, got %v", tok)
	}
	for dec.More() {
		key, err := stringToken(dec)
		if err != nil {
			return err
		}
		if key != "rows" {
			if err := skipValue(dec); err != nil {
				return err
			}
			continue
		}
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if tok != json.Delim('[') {
			return fmt.Errorf("expected rows array, got %v", tok)
		}
		return nil
	}
	return errors.New("missing rows array")
}

func advanceToDataArray(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim('{') {
		return fmt.Errorf("expected root object, got %v", tok)
	}

	for dec.More() {
		key, err := stringToken(dec)
		if err != nil {
			return err
		}
		if key != "data" {
			if err := skipValue(dec); err != nil {
				return err
			}
			continue
		}
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if tok != json.Delim('[') {
			return fmt.Errorf("expected data array, got %v", tok)
		}
		return nil
	}
	return errors.New("missing data array")
}

func skipValue(dec *json.Decoder) error {
	var v any
	return dec.Decode(&v)
}

func stringToken(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	s, ok := tok.(string)
	if !ok {
		return "", fmt.Errorf("expected object key, got %v", tok)
	}
	return s, nil
}

func insertSQL(table string, leadingColumns, columns []string) string {
	allColumns := append(slices.Clone(leadingColumns), columns...)
	quoted := make([]string, len(allColumns))
	placeholders := make([]string, len(allColumns))
	for i, col := range allColumns {
		quoted[i] = fmt.Sprintf("%q", col)
		placeholders[i] = "?"
	}
	return fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(quoted, ", "),
		strings.Join(placeholders, ", "),
	)
}

func values(source string, columns []string, row map[string]any) ([]any, error) {
	rawJSON, err := json.Marshal(row)
	if err != nil {
		return nil, err
	}
	args := make([]any, 0, len(columns)+1)
	args = append(args, source)
	args = append(args, string(rawJSON))
	for _, col := range columns {
		args = append(args, normalize(row[col]))
	}
	return args, nil
}

func normalize(v any) any {
	switch v := v.(type) {
	case nil, string, bool:
		return v
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		return v.String()
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	}
}

func discoverDictFiles(dataDir string) ([]string, error) {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, fmt.Errorf("read data directory: %w", err)
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".json" || name == "small.json" || slices.Contains(defaultThemeFiles, name) {
			continue
		}
		path := filepath.Join(dataDir, name)
		if looksLikeDictionary(path) {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	return paths, nil
}

func looksLikeDictionary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	dec := json.NewDecoder(io.LimitReader(f, 64*1024))
	dec.UseNumber()
	if err := advanceToDataArray(dec); err != nil {
		return false
	}
	if !dec.More() {
		return true
	}
	var row map[string]any
	if err := dec.Decode(&row); err != nil {
		return false
	}
	_, hasDictType := row["dictType"]
	_, hasDictValue := row["dictValue"]
	return hasDictType && hasDictValue
}
