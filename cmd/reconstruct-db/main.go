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

type stringList []string

func (l *stringList) String() string {
	return strings.Join(*l, ",")
}

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func main() {
	dataDir := flag.String("data", "data", "directory containing source JSON files")
	outPath := flag.String("out", "fyp-data.sqlite", "SQLite database to create")
	includeSmall := flag.Bool("include-small", false, "also import small.json into themes")
	smallOnly := flag.Bool("small-only", false, "import only small.json into themes")
	incremental := flag.Bool("incremental", false, "preserve existing database and update/add rows from imported source files")
	var themeFiles stringList
	flag.Var(&themeFiles, "theme-file", "theme page JSON file to import; repeat to import multiple files")
	flag.Parse()

	if err := run(*dataDir, *outPath, *includeSmall, *smallOnly, *incremental, themeFiles); err != nil {
		log.Fatal(err)
	}
}

func run(dataDir, outPath string, includeSmall, smallOnly, incremental bool, themeFiles []string) error {
	if !incremental {
		if err := os.Remove(outPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove existing database: %w", err)
		}
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

	themeFiles = resolveThemeFiles(themeFiles, includeSmall, smallOnly)

	totalThemes := 0
	seenThemeIDs := make([]int64, 0)
	for _, name := range themeFiles {
		n, seen, err := importThemes(db, filepath.Join(dataDir, name), name, incremental)
		if err != nil {
			return err
		}
		totalThemes += n
		seenThemeIDs = append(seenThemeIDs, seen...)
		log.Printf("imported %d theme rows from %s", n, name)
	}
	if incremental {
		if err := markMissingThemes(db, seenThemeIDs); err != nil {
			return err
		}
	}

	totalDicts := 0
	dictFiles, err := discoverDictFiles(dataDir)
	if err != nil {
		return err
	}
	for _, path := range dictFiles {
		source := filepath.Base(path)
		n, err := importDictionary(db, path, source, incremental)
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
	if incremental {
		log.Printf("updated %s with %d theme rows and %d dictionary rows", outPath, totalThemes, totalDicts)
	} else {
		log.Printf("created %s with %d theme rows and %d dictionary rows", outPath, totalThemes, totalDicts)
	}
	return nil
}

func resolveThemeFiles(files []string, includeSmall, smallOnly bool) []string {
	if len(files) > 0 {
		return slices.Clone(files)
	}
	themeFiles := slices.Clone(defaultThemeFiles)
	if smallOnly {
		return []string{"small.json"}
	}
	if includeSmall {
		themeFiles = append(themeFiles, "small.json")
	}
	return themeFiles
}

func markMissingThemes(db *sql.DB, seenIDs []int64) error {
	if _, err := db.Exec(`CREATE TEMP TABLE IF NOT EXISTS seen_theme_ids (id INTEGER PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create seen theme temp table: %w", err)
	}
	if _, err := db.Exec(`DELETE FROM seen_theme_ids`); err != nil {
		return fmt.Errorf("clear seen theme temp table: %w", err)
	}
	stmt, err := db.Prepare(`INSERT OR IGNORE INTO seen_theme_ids(id) VALUES (?)`)
	if err != nil {
		return fmt.Errorf("prepare seen theme insert: %w", err)
	}
	for _, id := range seenIDs {
		if _, err := stmt.Exec(id); err != nil {
			stmt.Close()
			return fmt.Errorf("insert seen theme id %d: %w", id, err)
		}
	}
	if err := stmt.Close(); err != nil {
		return fmt.Errorf("close seen theme insert: %w", err)
	}
	if _, err := db.Exec(`
		UPDATE themes
		SET missing = CASE
			WHEN EXISTS (SELECT 1 FROM seen_theme_ids WHERE seen_theme_ids.id = themes.id) THEN 0
			ELSE 1
		END
	`); err != nil {
		return fmt.Errorf("mark missing themes: %w", err)
	}
	return nil
}

func createSchema(db *sql.DB) error {
	var b strings.Builder
	b.WriteString("CREATE TABLE IF NOT EXISTS themes (\n")
	b.WriteString("  id INTEGER PRIMARY KEY AUTOINCREMENT,\n")
	b.WriteString("  source_file TEXT NOT NULL,\n")
	b.WriteString("  raw_json TEXT NOT NULL,\n")
	b.WriteString("  missing INTEGER NOT NULL DEFAULT 0,\n")
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

	b.WriteString("CREATE TABLE IF NOT EXISTS dictionaries (\n")
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
	if err := ensureColumn(db, "themes", "missing", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := migrateOldMissingColumn(db); err != nil {
		return err
	}
	return nil
}

func migrateOldMissingColumn(db *sql.DB) error {
	if !columnExists(db, "themes", "missing_from_latest") {
		return nil
	}
	if _, err := db.Exec(`UPDATE themes SET missing = missing_from_latest WHERE missing_from_latest IS NOT NULL`); err != nil {
		return fmt.Errorf("migrate missing_from_latest to missing: %w", err)
	}
	return nil
}

func ensureColumn(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("scan %s schema: %w", table, err)
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate %s schema: %w", table, err)
	}
	if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, definition)); err != nil {
		return fmt.Errorf("add %s.%s: %w", table, column, err)
	}
	return nil
}

func columnExists(db *sql.DB, table, column string) bool {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false
		}
		if name == column {
			return true
		}
	}
	return false
}

func createIndexes(db *sql.DB) error {
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_themes_theme_id ON themes(themeId)`,
		`CREATE INDEX IF NOT EXISTS idx_themes_teacher_id ON themes(themeTeacherId)`,
		`CREATE INDEX IF NOT EXISTS idx_themes_state ON themes(themeState)`,
		`CREATE INDEX IF NOT EXISTS idx_themes_subject_area ON themes(themeSubjectArea, themeSubjectAreaSub)`,
		`CREATE INDEX IF NOT EXISTS idx_dictionaries_type_value ON dictionaries(dictType, dictValue)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("create index %q: %w", stmt, err)
		}
	}
	return nil
}

func importThemes(db *sql.DB, path, source string, incremental bool) (int, []int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.UseNumber()
	if err := advanceToRows(dec); err != nil {
		return 0, nil, fmt.Errorf("find rows in %s: %w", path, err)
	}
	insertStmt, err := db.Prepare(insertSQL("themes", []string{"source_file", "raw_json", "missing"}, themeColumns))
	if err != nil {
		return 0, nil, fmt.Errorf("prepare theme insert: %w", err)
	}
	defer insertStmt.Close()

	var updateStmt *sql.Stmt
	var selectStmt *sql.Stmt
	if incremental {
		updateStmt, err = db.Prepare(updateSQL("themes", []string{"source_file", "raw_json", "missing"}, themeColumns))
		if err != nil {
			return 0, nil, fmt.Errorf("prepare theme update: %w", err)
		}
		defer updateStmt.Close()

		selectStmt, err = db.Prepare(`SELECT id FROM themes WHERE source_file = ? AND themeId = ? LIMIT 1`)
		if err != nil {
			return 0, nil, fmt.Errorf("prepare theme lookup: %w", err)
		}
		defer selectStmt.Close()
	}

	count := 0
	seen := make([]int64, 0)
	for dec.More() {
		var row map[string]any
		if err := dec.Decode(&row); err != nil {
			return count, seen, fmt.Errorf("decode row %d in %s: %w", count+1, path, err)
		}
		args, err := values(source, themeColumns, row)
		if err != nil {
			return count, seen, fmt.Errorf("prepare row %d from %s: %w", count+1, path, err)
		}
		args = append([]any{args[0], args[1], 0}, args[2:]...)
		if incremental {
			themeID := normalize(row["themeId"])
			if themeID != nil {
				var id int64
				err := selectStmt.QueryRow(source, themeID).Scan(&id)
				if err == nil {
					updateArgs := append(slices.Clone(args), id)
					if _, err := updateStmt.Exec(updateArgs...); err != nil {
						return count, seen, fmt.Errorf("update row %d from %s: %w", count+1, path, err)
					}
					seen = append(seen, id)
					count++
					continue
				}
				if !errors.Is(err, sql.ErrNoRows) {
					return count, seen, fmt.Errorf("lookup row %d from %s: %w", count+1, path, err)
				}
			}
		}
		result, err := insertStmt.Exec(args...)
		if err != nil {
			return count, seen, fmt.Errorf("insert row %d from %s: %w", count+1, path, err)
		}
		if id, err := result.LastInsertId(); err == nil {
			seen = append(seen, id)
		}
		count++
	}
	return count, seen, nil
}

func importDictionary(db *sql.DB, path, source string, incremental bool) (int, error) {
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
	insertStmt, err := db.Prepare(insertSQL("dictionaries", []string{"source_file", "raw_json"}, dictColumns))
	if err != nil {
		return 0, fmt.Errorf("prepare dictionary insert: %w", err)
	}
	defer insertStmt.Close()

	var updateStmt *sql.Stmt
	var selectStmt *sql.Stmt
	if incremental {
		updateStmt, err = db.Prepare(updateSQL("dictionaries", []string{"source_file", "raw_json"}, dictColumns))
		if err != nil {
			return 0, fmt.Errorf("prepare dictionary update: %w", err)
		}
		defer updateStmt.Close()

		selectStmt, err = db.Prepare(`SELECT id FROM dictionaries WHERE source_file = ? AND dictType = ? AND dictValue = ? LIMIT 1`)
		if err != nil {
			return 0, fmt.Errorf("prepare dictionary lookup: %w", err)
		}
		defer selectStmt.Close()
	}

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
		if incremental {
			dictType := normalize(row["dictType"])
			dictValue := normalize(row["dictValue"])
			if dictType != nil && dictValue != nil {
				var id int64
				err := selectStmt.QueryRow(source, dictType, dictValue).Scan(&id)
				if err == nil {
					updateArgs := append(slices.Clone(args), id)
					if _, err := updateStmt.Exec(updateArgs...); err != nil {
						return count, fmt.Errorf("update row %d from %s: %w", count+1, path, err)
					}
					count++
					continue
				}
				if !errors.Is(err, sql.ErrNoRows) {
					return count, fmt.Errorf("lookup row %d from %s: %w", count+1, path, err)
				}
			}
		}
		if _, err := insertStmt.Exec(args...); err != nil {
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

func updateSQL(table string, leadingColumns, columns []string) string {
	allColumns := append(slices.Clone(leadingColumns), columns...)
	assignments := make([]string, len(allColumns))
	for i, col := range allColumns {
		assignments[i] = fmt.Sprintf("%q = ?", col)
	}
	return fmt.Sprintf(
		"UPDATE %s SET %s WHERE id = ?",
		table,
		strings.Join(assignments, ", "),
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
