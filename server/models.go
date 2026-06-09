package server

type dictionaryLabels map[string]map[string]dictionaryLabel

type dictionaryLabel struct {
	Label   string `json:"label"`
	LabelEn string `json:"label_en"`
}

type theme struct {
	ID                      int64          `json:"id"`
	SourceFile              string         `json:"source_file"`
	ThemeSubjectArea        string         `json:"themeSubjectArea"`
	DelFlag                 string         `json:"delFlag"`
	Status                  string         `json:"status"`
	ThemePrerequisiteSkills string         `json:"themePrerequisiteSkills"`
	ThemeProjectDescription string         `json:"themeProjectDescription"`
	ThemeProjectType        string         `json:"themeProjectType"`
	ThemeSourceRemarks      string         `json:"themeSourceRemarks"`
	ThemeSourceSub          string         `json:"themeSourceSub"`
	ThemeSource             string         `json:"themeSource"`
	ThemeSubjectAreaSub     string         `json:"themeSubjectAreaSub"`
	ThemeID                 *int64         `json:"themeId"`
	ThemeProgramme          string         `json:"themeProgramme"`
	ThemeCommitmentCheck    string         `json:"themeCommitmentCheck"`
	ThemeOfficeLocation     string         `json:"themeOfficeLocation"`
	ThemeExpertID           *int64         `json:"themeExpertId"`
	ThemeCheckTime          *int64         `json:"themeCheckTime"`
	ThemeCheckComment       string         `json:"themeCheckComment"`
	ThemeCount              *int64         `json:"themeCount"`
	ThemeTeacherID          *int64         `json:"themeTeacherId"`
	ThemeState              string         `json:"themeState"`
	ThemeIsSelect           string         `json:"themeIsselect"`
	ThemeType               string         `json:"themeType"`
	ThemeTitle              string         `json:"themeTitle"`
	ThemeProID              *int64         `json:"themeProId"`
	ThemeUniversity         string         `json:"themeUniversity"`
	IndStudentID            *int64         `json:"indStudentId"`
	TeacherName             string         `json:"teacherName"`
	TeacherPinyin           string         `json:"teacherPinyin"`
	UserType                string         `json:"userType"`
	StudentName             string         `json:"studentName"`
	StudentPinyin           string         `json:"studentPinyin"`
	GUID                    string         `json:"guid"`
	PhoneNumber             string         `json:"phonenumber"`
	Email                   string         `json:"email"`
	SelectCount             string         `json:"selectCount"`
	CreateBy                string         `json:"createBy"`
	CreateTime              string         `json:"createTime"`
	UpdateBy                string         `json:"updateBy"`
	UpdateTime              string         `json:"updateTime"`
	ThemeSubmitTime         string         `json:"themeSubmitTime"`
	IndResults              string         `json:"indResults"`
	IndState                string         `json:"indState"`
	TeacherEmail            string         `json:"teacherEmail"`
	TeacherChineseEnglish   string         `json:"teacherChineseEnglish"`
	DeptName                string         `json:"deptName"`
	DeptNameEn              string         `json:"deptName_en"`
	JobType                 string         `json:"jobType"`
	ExtendJb                string         `json:"extendJb"`
	ExtendSzyx              string         `json:"extendSzyx"`
	ExtendSzxy              string         `json:"extendSzxy"`
	Labels                  map[string]any `json:"labels,omitempty"`
}

type dictionaryRow struct {
	ID          int64  `json:"id"`
	SourceFile  string `json:"source_file"`
	DictCode    *int64 `json:"dictCode"`
	DictSort    *int64 `json:"dictSort"`
	DictLabel   string `json:"dictLabel"`
	DictLabelEn string `json:"dictLabelEn"`
	Label       string `json:"label"`
	DictValue   string `json:"dictValue"`
	DictType    string `json:"dictType"`
	CSSClass    string `json:"cssClass"`
	ListClass   string `json:"listClass"`
	IsDefault   string `json:"isDefault"`
	Status      string `json:"status"`
	Remark      string `json:"remark"`
	CreateBy    *int64 `json:"createBy"`
	CreateTime  string `json:"createTime"`
	UpdateBy    *int64 `json:"updateBy"`
	UpdateTime  string `json:"updateTime"`
}
