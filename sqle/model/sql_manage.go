package model

import (
	e "errors"
	"fmt"
	"strings"
	"time"

	"github.com/actiontech/sqle/sqle/errors"
	"github.com/jinzhu/gorm"
)

const (
	SQLManageStatusUnhandled = "unhandled"
	SQLManageStatusSolved    = "solved"
	SQLManageStatusIgnored   = "ignored"

	SQLManageSourceAuditPlan      = "audit_plan"
	SQLManageSourceSqlAuditRecord = "sql_audit_record"
)

type SqlManage struct {
	Model
	SqlFingerprint            string       `json:"sql_fingerprint" gorm:"type:mediumtext;not null"`
	ProjFpSourceInstSchemaMd5 string       `json:"proj_fp_source_inst_schema_md5" gorm:"unique_index:proj_fp_source_inst_schema_md5;not null"`
	SqlText                   string       `json:"sql_text" gorm:"type:mediumtext;not null"`
	Source                    string       `json:"source"`
	AuditLevel                string       `json:"audit_level"`
	AuditResults              AuditResults `json:"audit_results" gorm:"type:json"`
	FpCount                   uint64       `json:"fp_count"`
	FirstAppearTimestamp      *time.Time   `json:"first_appear_timestamp"`
	LastReceiveTimestamp      *time.Time   `json:"last_receive_timestamp"`
	InstanceName              string       `json:"instance_name"`
	SchemaName                string       `json:"schema_name"`

	Assignees []*User `gorm:"many2many:sql_manage_assignees;"`
	Status    string  `json:"status" gorm:"default:\"unhandled\""`
	Remark    string  `json:"remark"`

	ProjectId uint     `json:"project_id"`
	Project   *Project `gorm:"foreignkey:ProjectId"`

	AuditPlanId uint       `json:"audit_plan_id"`
	AuditPlan   *AuditPlan `gorm:"foreignkey:AuditPlanId"`

	SqlAuditRecordId uint            `json:"sql_audit_record_id"`
	SqlAuditRecord   *SQLAuditRecord `gorm:"foreignkey:SqlAuditRecordId"`
}

func (s *Storage) GetSqlManageByFingerprintSourceInstNameSchemaMd5(projFpSourceInstSchemaMd5 string) (*SqlManage, bool, error) {
	sqlManage := &SqlManage{}
	err := s.db.Where("proj_fp_source_inst_schema_md5 = ?", projFpSourceInstSchemaMd5).Find(sqlManage).Error
	if e.Is(err, gorm.ErrRecordNotFound) {
		return sqlManage, false, nil
	}

	return sqlManage, true, errors.New(errors.ConnectStorageError, err)
}

type SqlManageResp struct {
	SqlManageList         []*SqlManageDetail
	SqlManageTotalNum     uint64 `json:"sql_manage_total_num"`
	SqlManageBadNum       uint64 `json:"sql_manage_bad_num"`
	SqlManageOptimizedNum uint64 `json:"sql_manage_optimized_num"`
}

type SqlManageDetail struct {
	ID                   uint         `json:"id"`
	SqlFingerprint       string       `json:"sql_fingerprint"`
	SqlText              string       `json:"sql_text"`
	Source               string       `json:"source"`
	AuditLevel           string       `json:"audit_level"`
	AuditResults         AuditResults `json:"audit_results"`
	FpCount              uint64       `json:"fp_count"`
	AppearTimestamp      *time.Time   `json:"first_appear_timestamp"`
	LastReceiveTimestamp *time.Time   `json:"last_receive_timestamp"`
	InstanceName         string       `json:"instance_name"`
	SchemaName           string       `json:"schema_name"`
	Status               string       `json:"status"`
	Remark               string       `json:"remark"`
	Assignees            RowList      `json:"assignees"`
	ApName               *string      `json:"ap_name"`
	SqlAuditRecordID     *string      `json:"sql_audit_record_id"`
}

var sqlManageQueryTpl = `
SELECT 
	sm.id,
	sm.sql_fingerprint,
	sm.sql_text,
	sm.source,
	sm.audit_level,
	sm.audit_results,
	sm.fp_count,
    sm.first_appear_timestamp,
	sm.last_receive_timestamp,
	sm.instance_name,
	sm.schema_name,
	sm.status,
	sm.remark,
	GROUP_CONCAT(u.login_name) as assignees,
	ap.name as ap_name,
	sar.audit_record_id as sql_audit_record_id

{{- template "body" . -}} 

GROUP BY sm.id
ORDER BY sm.id desc

{{- if .limit }}
LIMIT :limit OFFSET :offset
{{- end -}}
`

var sqlManageBodyTpl = `
{{ define "body" }}

FROM sql_manages sm
         LEFT JOIN sql_audit_records sar ON sm.sql_audit_record_id = sar.id
         LEFT JOIN audit_plans ap ON ap.id = sm.audit_plan_id
         LEFT JOIN projects p ON p.id = sm.project_id
         LEFT JOIN sql_manage_assignees sma ON sma.sql_manage_id = sm.id
         LEFT JOIN users u ON u.id = sma.user_id

WHERE p.name = :project_name
  AND sm.deleted_at IS NULL

{{- if .fuzzy_search_sql_fingerprint }}
AND sm.sql_fingerprint LIKE '%{{ .fuzzy_search_sql_fingerprint }}%'
{{- end }}

{{- if .filter_assignee }}
AND u.login_name = :filter_assignee
{{- end }}

{{- if .filter_instance_name }}
AND sm.instance_name = :filter_instance_name
{{- end }}

{{- if .filter_source }}
AND sm.source = :filter_source
{{- end }}

{{- if .filter_audit_level }}
AND sm.audit_level = :filter_audit_level
{{- end }}

{{- if .filter_last_audit_start_time_from }}
AND sm.last_receive_timestamp >= :filter_last_audit_start_time_from
{{- end }}

{{- if .filter_last_audit_start_time_to }}
AND sm.last_receive_timestamp <= :filter_last_audit_start_time_to
{{- end }}

{{- if .filter_status }}
AND sm.status = :filter_status
{{- end }}

{{ end }}
`

func (s *Storage) GetSqlManageListByReq(data map[string]interface{}) (list *SqlManageResp, err error) {
	sqlManageList := make([]*SqlManageDetail, 0)
	err = s.getListResult(sqlManageQueryTpl, sqlManageBodyTpl, data, &sqlManageList)
	if err != nil {
		return nil, err
	}

	totalCount := len(sqlManageList)

	var badSqlCount uint64
	var solvedCount uint64
	for _, sqlManage := range sqlManageList {
		if sqlManage.AuditLevel != "" && sqlManage.Status != SQLManageStatusSolved {
			badSqlCount += 1
		}

		if sqlManage.Status == SQLManageStatusSolved {
			solvedCount += 1
		}
	}

	return &SqlManageResp{
		SqlManageList:         sqlManageList,
		SqlManageTotalNum:     uint64(totalCount),
		SqlManageBadNum:       badSqlCount,
		SqlManageOptimizedNum: solvedCount,
	}, nil
}

func (s *Storage) GetAllSqlManageList() ([]*SqlManage, error) {
	sqlManageList := make([]*SqlManage, 0)
	err := s.db.Model(&SqlManage{}).Find(&sqlManageList).Error
	if err != nil {
		return nil, errors.New(errors.ConnectStorageError, err)
	}
	return sqlManageList, nil
}

func (s *Storage) InsertOrUpdateSqlManage(sqlManageList []*SqlManage) error {
	args := make([]interface{}, 0, len(sqlManageList))
	pattern := make([]string, 0, len(sqlManageList))
	for _, sqlManage := range sqlManageList {
		pattern = append(pattern, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		args = append(args, sqlManage.SqlFingerprint, sqlManage.ProjFpSourceInstSchemaMd5, sqlManage.SqlText,
			sqlManage.Source, sqlManage.AuditLevel, sqlManage.AuditResults, sqlManage.FpCount, sqlManage.FirstAppearTimestamp,
			sqlManage.LastReceiveTimestamp, sqlManage.InstanceName, sqlManage.SchemaName, sqlManage.Status, sqlManage.Remark,
			sqlManage.AuditPlanId, sqlManage.ProjectId, sqlManage.SqlAuditRecordId)
	}

	raw := fmt.Sprintf(`
INSERT INTO sql_manages (sql_fingerprint, proj_fp_source_inst_schema_md5, sql_text, source, audit_level, audit_results,
                         fp_count, first_appear_timestamp, last_receive_timestamp, instance_name, schema_name, status,
                         remark, audit_plan_id, project_id, sql_audit_record_id)
		VALUES %s
		ON DUPLICATE KEY UPDATE sql_text       = VALUES(sql_text),
                        audit_plan_id          = VALUES(audit_plan_id),
                        sql_audit_record_id    = VALUES(sql_audit_record_id),
                        audit_level            = VALUES(audit_level),
                        audit_results          = VALUES(audit_results),
                        fp_count 			   = VALUES(fp_count),
                        first_appear_timestamp = VALUES(first_appear_timestamp),
                        last_receive_timestamp = VALUES(last_receive_timestamp);`,
		strings.Join(pattern, ", "))

	return errors.New(errors.ConnectStorageError, s.db.Exec(raw, args...).Error)
}