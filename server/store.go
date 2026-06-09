package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const themeSelectColumns = `
	id, source_file, raw_json, missing, themeSubjectArea, delFlag, status, themePrerequisiteSkills,
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

func (a *App) counts(ctx context.Context) (map[string]int64, error) {
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

func (a *App) themesByIDs(ctx context.Context, ids []int64) ([]theme, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	byID := make(map[int64]theme, len(ids))
	for _, id := range ids {
		rows, err := a.sqlite.QueryContext(ctx, "SELECT "+themeSelectColumns+" FROM themes WHERE id = ?", id)
		if err != nil {
			return nil, err
		}
		themes, err := scanThemes(rows)
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

func scanThemes(rows *sql.Rows) ([]theme, error) {
	var themes []theme
	for rows.Next() {
		var t theme
		var raw string
		var missing int
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
			&t.ID, &t.SourceFile, &raw, &missing, &nulls[0], &nulls[1], &nulls[2], &nulls[3],
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
		t.Missing = missing != 0
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
		themes = append(themes, t)
	}
	return themes, rows.Err()
}

func scanDictionaries(rows *sql.Rows) ([]dictionaryRow, error) {
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
	if v := firstQueryValue(r, "missing"); v != "" {
		clauses = append(clauses, "missing = ?")
		args = append(args, missingFilterValue(v))
	}
	return strings.Join(clauses, " AND "), args
}

func firstQueryValue(r *http.Request, names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(r.URL.Query().Get(name)); v != "" {
			return v
		}
	}
	return ""
}

func missingFilterValue(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y":
		return 1
	default:
		return 0
	}
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
