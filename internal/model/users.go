package model

// 通用多语言字段结构
type I18nName struct {
	DefaultValue string            `json:"default_value"`
	I18nValue    map[string]string `json:"i18n_value,omitempty"`
}

// 部门统计信息
type DepartmentCount struct {
	RecursiveMembersCount               string `json:"recursive_members_count"`
	DirectMembersCount                  string `json:"direct_members_count"`
	RecursiveMembersCountExcludeLeaders string `json:"recursive_members_count_exclude_leaders"`
	RecursiveDepartmentsCount           string `json:"recursive_departments_count"`
	DirectDepartmentsCount              string `json:"direct_departments_count"`
}

// 部门领导
type DepartmentLeader struct {
	LeaderType string `json:"leader_type"`
	LeaderID   string `json:"leader_id"`
}

// 自定义字段的值
type CustomFieldValue struct {
	FieldKey   string      `json:"field_key"`
	FieldType  string      `json:"field_type"`
	TextValue  *I18nName   `json:"text_value,omitempty"`
	URLValue   *URLValue   `json:"url_value,omitempty"`
	EnumValue  *EnumValue  `json:"enum_value,omitempty"`
	UserValues []UserValue `json:"user_values,omitempty"`
}

// URL值
type URLValue struct {
	LinkText *I18nName `json:"link_text"`
	URL      string    `json:"url"`
	PCURL    string    `json:"pcurl,omitempty"`
}

// 枚举值
type EnumValue struct {
	EnumIDs  []string `json:"enum_ids"`
	EnumName string   `json:"enum_name"`
	EnumType string   `json:"enum_type"`
}

// 用户值
type UserValue struct {
	IDs      []string `json:"ids"`
	UserType string   `json:"user_type"`
}

// 部门路径信息
type DepartmentPathInfo struct {
	DepartmentID   string   `json:"department_id"`
	DepartmentName I18nName `json:"department_name"`
}

// 部门信息
type Department struct {
	DepartmentID        string               `json:"department_id"`
	DepartmentCount     DepartmentCount      `json:"department_count"`
	HasChild            bool                 `json:"has_child"`
	Leaders             []DepartmentLeader   `json:"leaders,omitempty"`
	HRBPs               []string             `json:"hrbps,omitempty"`
	ParentDepartmentID  string               `json:"parent_department_id"`
	Name                I18nName             `json:"name"`
	OrderWeight         string               `json:"order_weight"`
	CustomFieldValues   []CustomFieldValue   `json:"custom_field_values,omitempty"`
	DepartmentPathInfos []DepartmentPathInfo `json:"department_path_infos,omitempty"`
	DataSource          int                  `json:"data_source"`
}

// 部门内排序信息
type EmployeeOrderInDepartment struct {
	DepartmentID                string `json:"department_id"`
	OrderWeightInDepartment     string `json:"order_weight_in_department"`
	OrderWeightAmongDepartments string `json:"order_weight_among_deparments"`
}

// 头像信息
type Avatar struct {
	Avatar72     string `json:"avatar_72"`
	Avatar240    string `json:"avatar_240"`
	Avatar640    string `json:"avatar_640"`
	AvatarOrigin string `json:"avatar_origin"`
}

// 工作地点信息
type Workplace struct {
	PlaceID     string   `json:"place_id"`
	PlaceName   I18nName `json:"place_name"`
	IsEnabled   bool     `json:"is_enabled"`
	Description I18nName `json:"description"`
}

// 职位信息
type JobTitle struct {
	JobTitleID   string   `json:"job_title_id"`
	JobTitleName I18nName `json:"job_title_name"`
	IsEnabled    bool     `json:"is_enabled"`
	Description  I18nName `json:"description"`
}

// 职位系列
type JobFamily struct {
	Description       I18nName `json:"description"`
	IsEnabled         bool     `json:"is_enabled"`
	JobFamilyID       string   `json:"job_family_id"`
	JobFamilyName     I18nName `json:"job_family_name"`
	ParentJobFamilyID string   `json:"parent_job_family_id"`
}

// 职位
type Position struct {
	PositionCode       string `json:"position_code"`
	PositionName       string `json:"position_name"`
	LeaderID           string `json:"leader_id"`
	LeaderPositionCode string `json:"leader_position_code"`
	IsMainPosition     bool   `json:"is_main_position"`
	DepartmentID       string `json:"department_id"`
}

// 员工基础信息
type BaseInfo struct {
	EmployeeID string `json:"employee_id"`
	Name       struct {
		Name        I18nName `json:"name"`
		AnotherName string   `json:"another_name"`
	} `json:"name"`
	Mobile                     string                      `json:"mobile"`
	Email                      string                      `json:"email"`
	EnterpriseEmail            string                      `json:"enterprise_email"`
	Gender                     int                         `json:"gender"`
	Departments                []Department                `json:"departments"`
	EmployeeOrderInDepartments []EmployeeOrderInDepartment `json:"employee_order_in_departments"`
	Description                string                      `json:"description"`
	ActiveStatus               int                         `json:"active_status"`
	IsResigned                 bool                        `json:"is_resigned"`
	LeaderID                   string                      `json:"leader_id"`
	DottedLineLeaderIDs        []string                    `json:"dotted_line_leader_ids"`
	IsPrimaryAdmin             bool                        `json:"is_primary_admin"`
	EnterpriseEmailAliases     []string                    `json:"enterprise_email_aliases"`
	CustomFieldValues          []CustomFieldValue          `json:"custom_field_values,omitempty"`
	DepartmentPathInfos        [][]DepartmentPathInfo      `json:"department_path_infos"`
	ResignTime                 string                      `json:"resign_time"`
	Avatar                     Avatar                      `json:"avatar"`
	BackgroundImage            string                      `json:"background_image"`
	IsAdmin                    bool                        `json:"is_admin"`
	DataSource                 int                         `json:"data_source"`
	GeoName                    string                      `json:"geo_name"`
	SubscriptionIDs            []string                    `json:"subscription_ids"`
}

// 员工工作信息
type WorkInfo struct {
	WorkCountryOrRegion string     `json:"work_country_or_region"`
	WorkPlace           Workplace  `json:"work_place"`
	WorkStation         I18nName   `json:"work_station"`
	JobNumber           string     `json:"job_number"`
	ExtensionNumber     string     `json:"extension_number"`
	JoinDate            string     `json:"join_date"`
	EmploymentType      int        `json:"employment_type"`
	StaffStatus         int        `json:"staff_status"`
	Positions           []Position `json:"positions"`
	JobTitle            JobTitle   `json:"job_title"`
	JobFamily           JobFamily  `json:"job_family"`
}

// 员工完整信息
type Employee struct {
	BaseInfo BaseInfo `json:"base_info"`
	WorkInfo WorkInfo `json:"work_info"`
}

// 异常信息中的字段错误
type FieldErrors struct {
	BaseInfoMobile int `json:"base_info.mobile"`
}

// 异常信息
type Abnormal struct {
	RowError    int         `json:"row_error"`
	FieldErrors FieldErrors `json:"field_errors"`
	ID          string      `json:"id"`
}

// 分页响应
type PageResponse struct {
	HasMore   bool   `json:"has_more"`
	PageToken string `json:"page_token"`
}

// 主响应结构
type GetUserInfoAPIResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Employees    []Employee   `json:"employees"`
		PageResponse PageResponse `json:"page_response"`
		Abnormals    []Abnormal   `json:"abnormals"`
	} `json:"data"`
}
