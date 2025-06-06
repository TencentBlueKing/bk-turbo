/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package disttask

import (
	"fmt"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	commonMySQL "github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/mysql"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/server/pkg/engine"
	selfMetric "github.com/TencentBlueKing/bk-turbo/src/backend/booster/server/pkg/metric"

	"github.com/jinzhu/gorm"
)

// MySQL describe the full operations to mysql databases need by engine.
type MySQL interface {
	// get db operator
	GetDB() *gorm.DB

	ListTask(opts commonMySQL.ListOptions) ([]*TableTask, int64, error)
	GetTask(taskID string) (*TableTask, error)
	PutTask(task *TableTask) error
	UpdateTask(taskID string, task map[string]interface{}) error
	UpdateTaskPart(taskID string, task *TableTask) error
	DeleteTask(taskID string) error

	ListProject(opts commonMySQL.ListOptions) ([]*CombinedProject, int64, error)

	ListProjectInfo(opts commonMySQL.ListOptions) ([]*TableProjectInfo, int64, error)
	GetProjectInfo(projectID string) (*TableProjectInfo, error)
	PutProjectInfo(projectInfo *TableProjectInfo) error
	UpdateProjectInfo(projectID string, projectInfo map[string]interface{}) error
	DeleteProjectInfo(projectID string) error
	AddProjectInfoStats(projectID string, delta DeltaInfoStats) error

	ListProjectSetting(opts commonMySQL.ListOptions) ([]*TableProjectSetting, int64, error)
	GetProjectSetting(projectID string) (*TableProjectSetting, error)
	PutProjectSetting(projectSetting *TableProjectSetting) error
	UpdateProjectSetting(projectID string, projectSetting map[string]interface{}) error
	DeleteProjectSetting(projectID string) error
	CreateOrUpdateProjectSetting(projectSetting *TableProjectSetting, projectSettingRaw map[string]interface{}) error

	ListWhitelist(opts commonMySQL.ListOptions) ([]*TableWhitelist, int64, error)
	GetWhitelist(key engine.WhiteListKey) (*TableWhitelist, error)
	PutWhitelist(wll []*TableWhitelist) error
	UpdateWhitelist(key engine.WhiteListKey, wll []map[string]interface{}) error
	DeleteWhitelist(keys []*engine.WhiteListKey) error

	ListWorker(opts commonMySQL.ListOptions) ([]*TableWorker, int64, error)
	GetWorker(version, scene string) (*TableWorker, error)
	PutWorker(worker *TableWorker) error
	UpdateWorker(version, scene string, worker map[string]interface{}) error
	DeleteWorker(version, scene string) error

	ListWorkStats(opts commonMySQL.ListOptions) ([]*TableWorkStats, int64, error)
	// GetWorkStats(id int) (*TableWorkStats, error)
	GetWorkStats(taskID, workID string) (*TableWorkStats, error)
	PutWorkStats(stats *TableWorkStats) error
	UpdateWorkStats(id int, stats map[string]interface{}) error
	DeleteWorkStats(id int) error

	SummaryTaskRecords(opts commonMySQL.ListOptions) ([]*SummaryResult, int64, error)
	SummaryTaskRecordsByUser(opts commonMySQL.ListOptions) ([]*SummaryResultByUser, int64, error)
}

// NewMySQL get new mysql instance with connected orm operator.
func NewMySQL(conf engine.MySQLConf) (MySQL, error) {
	if conf.Charset == "" {
		conf.Charset = "utf8"
	}

	blog.Info("get a new engine(%s) mysql: %s, db: %s, user: %s",
		EngineName, conf.MySQLStorage, conf.MySQLDatabase, conf.MySQLUser)
	source := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=%s&parseTime=True&loc=Local"+
		"&timeout=30s&readTimeout=30s&writeTimeout=30s",
		conf.MySQLUser, conf.MySQLPwd, conf.MySQLStorage, conf.MySQLDatabase, conf.Charset)
	db, err := gorm.Open("mysql", source)
	if err != nil {
		blog.Errorf("engine(%s) connect to mysql %s failed: %v", EngineName, conf.MySQLStorage, err)
		return nil, err
	}

	if conf.MySQLDebug {
		db = db.Debug()
	}

	m := &mysql{
		db: db,
	}
	if err = m.ensureTables(
		&TableTask{}, &TableProjectSetting{}, &TableProjectInfo{},
		&TableWhitelist{}, &TableWorker{}, &TableWorkStats{}); err != nil {
		blog.Errorf("engine(%s) mysql ensure tables failed: %v", EngineName, err)
		return nil, err
	}

	return m, nil
}

type mysql struct {
	db *gorm.DB
}

// ensureTables makes sure that the tables exist in database and runs migrations. In G-ORM, the migration
// will be save and the columns deleting will not be executed.
func (m *mysql) ensureTables(tables ...interface{}) error {
	for _, table := range tables {
		if !m.db.HasTable(table) {
			if err := m.db.CreateTable(table).Error; err != nil {
				return err
			}
		}
		if err := m.db.AutoMigrate(table).Error; err != nil {
			return err
		}
	}
	return nil
}

// GetDB get db operator.
func (m *mysql) GetDB() *gorm.DB {
	return m.db
}

// ListTask list task from db, return list and total num.
// list cut by offset and limit, but total num describe the true num.
func (m *mysql) ListTask(opts commonMySQL.ListOptions) ([]*TableTask, int64, error) {
	defer timeMetricRecord("list_task")()
	defer logSlowFunc(time.Now().Unix(), "ListTask", 2)

	var tl []*TableTask
	db := opts.AddWhere(m.db.Model(&TableTask{})).Where("disabled = ?", false)

	var length int64
	if err := db.Count(&length).Error; err != nil {
		blog.Errorf("engine(%s) mysql count task failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	db = opts.AddOffsetLimit(db)
	db = opts.AddSelector(db)
	db = opts.AddOrder(db)

	if err := db.Find(&tl).Error; err != nil {
		blog.Errorf("engine(%s) mysql list task failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	return tl, length, nil
}

// GetTask get task.
func (m *mysql) GetTask(taskID string) (*TableTask, error) {
	defer timeMetricRecord("get_task")()
	defer logSlowFunc(time.Now().Unix(), "GetTask", 2)

	opts := commonMySQL.NewListOptions()
	opts.Limit(1)
	opts.Equal("task_id", taskID)

	// tl, _, err := m.ListTask(opts)
	// if err != nil {
	// 	blog.Errorf("engine(%s) mysql get task(%s) failed: %v", EngineName, taskID, err)
	// 	return nil, err
	// }

	// if len(tl) < 1 {
	// 	err = engine.ErrorTaskNoFound
	// 	return nil, err
	// }

	var tl []*TableTask
	db := opts.AddWhere(m.db.Model(&TableTask{})).Where("disabled = ?", false)
	db = opts.AddOffsetLimit(db)
	db = opts.AddSelector(db)
	db = opts.AddOrder(db)

	if err := db.Find(&tl).Error; err != nil {
		blog.Errorf("engine(%s) mysql get task failed opts(%v): %v", EngineName, opts, err)
		return nil, err
	}

	if len(tl) < 1 {
		return nil, engine.ErrorTaskNoFound
	}

	return tl[0], nil
}

// PutTask put task with full fields.
func (m *mysql) PutTask(task *TableTask) error {
	defer timeMetricRecord("put_task")()
	defer logSlowFunc(time.Now().Unix(), "PutTask", 2)

	if err := m.db.Model(&TableTask{}).Save(task).Error; err != nil {
		blog.Errorf("engine(%s) mysql put task(%s) failed: %v", EngineName, task.TaskID, err)
		return err
	}
	return nil
}

// UpdateTask update task with given fields.
func (m *mysql) UpdateTask(taskID string, task map[string]interface{}) error {
	defer timeMetricRecord("update_task")()
	defer logSlowFunc(time.Now().Unix(), "UpdateTask", 2)

	task["disabled"] = false

	if err := m.db.Model(&TableTask{}).Where("task_id = ?", taskID).Updates(task).Error; err != nil {
		blog.Errorf("engine(%s) mysql update task(%s)(%+v) failed: %v", EngineName, taskID, task, err)
		return err
	}

	return nil
}

// UpdateTaskPart update task part with given fields by struct , not update null fields
func (m *mysql) UpdateTaskPart(taskID string, task *TableTask) error {
	defer timeMetricRecord("update_task_part")()
	defer logSlowFunc(time.Now().Unix(), "UpdateTaskPart", 2)

	if err := m.db.Model(&TableTask{}).Where("task_id = ?", taskID).Updates(task).Error; err != nil {
		blog.Errorf("engine(%s) mysql update task(%s)(%+v) part failed: %v", EngineName, taskID, task, err)
		return err
	}

	return nil
}

// DeleteTask delete task from db. Just set the disabled to true instead of real deletion.
func (m *mysql) DeleteTask(taskID string) error {
	defer timeMetricRecord("delete_task")()
	defer logSlowFunc(time.Now().Unix(), "DeleteTask", 2)

	if err := m.db.Model(&TableTask{}).Where("task_id = ?", taskID).
		Update("disabled", true).Error; err != nil {
		blog.Errorf("engine(%s) mysql delete task(%s) failed: %v", EngineName, taskID, err)
		return err
	}
	return nil
}

// ListProject join the two tables "project_settings" and "project_records".
// List all project_settings, then list project_records with those project_ids, then combine them together
// and return.
// TODO: consider to use mysql "join" command to do this
func (m *mysql) ListProject(opts commonMySQL.ListOptions) ([]*CombinedProject, int64, error) {
	defer timeMetricRecord("list_project")()
	defer logSlowFunc(time.Now().Unix(), "ListProject", 2)

	var settingProjectList []*TableProjectSetting
	db := opts.AddWhere(m.db.Model(&TableProjectSetting{})).Where("disabled = ?", false)

	var length int64
	if err := db.Count(&length).Error; err != nil {
		blog.Errorf("engine(%s) mysql count project setting failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	db = opts.AddOffsetLimit(db)
	db = opts.AddSelector(db)
	db = opts.AddOrder(db)

	if err := db.Find(&settingProjectList).Error; err != nil {
		blog.Errorf("engine(%s) mysql list project setting failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	combinedProjectList := make([]*CombinedProject, 0, 1000)
	if len(settingProjectList) == 0 {
		return combinedProjectList, 0, nil
	}

	projectIDList := make([]string, 0, 1000)
	for _, settingProject := range settingProjectList {
		projectIDList = append(projectIDList, settingProject.ProjectID)
	}

	var recordProjectList []*TableProjectInfo
	secondOpts := commonMySQL.NewListOptions()
	secondOpts.In("project_id", projectIDList)
	if err := secondOpts.AddWhere(m.db.Model(&TableProjectInfo{})).Find(&recordProjectList).Error; err != nil {
		blog.Errorf("engine(%s) mysql list project info failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	recordProjectMap := make(map[string]*TableProjectInfo, 1000)
	for _, recordProject := range recordProjectList {
		recordProjectMap[recordProject.ProjectID] = recordProject
	}

	for _, settingProject := range settingProjectList {

		combinedProjectList = append(combinedProjectList, &CombinedProject{
			TableProjectSetting: settingProject,
			TableProjectInfo:    recordProjectMap[settingProject.ProjectID],
		})

	}

	return combinedProjectList, length, nil
}

// ListProjectInfo list project info from db, return list and total num.
// list cut by offset and limit, but total num describe the true num.
func (m *mysql) ListProjectInfo(opts commonMySQL.ListOptions) ([]*TableProjectInfo, int64, error) {
	defer timeMetricRecord("list_project_info")()
	defer logSlowFunc(time.Now().Unix(), "ListProjectInfo", 2)

	var pl []*TableProjectInfo
	db := opts.AddWhere(m.db.Model(&TableProjectInfo{})).Where("disabled = ?", false)

	var length int64
	if err := db.Count(&length).Error; err != nil {
		blog.Errorf("engine(%s) mysql count project info failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	db = opts.AddOffsetLimit(db)
	db = opts.AddSelector(db)
	db = opts.AddOrder(db)

	if err := db.Find(&pl).Error; err != nil {
		blog.Errorf("engine(%s) mysql list project info failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	return pl, length, nil
}

// GetProjectInfo get project info.
func (m *mysql) GetProjectInfo(projectID string) (*TableProjectInfo, error) {
	defer timeMetricRecord("get_project_info")()
	defer logSlowFunc(time.Now().Unix(), "GetProjectInfo", 2)

	opts := commonMySQL.NewListOptions()
	opts.Limit(1)
	opts.Equal("project_id", projectID)

	// pl, _, err := m.ListProjectInfo(opts)
	// if err != nil {
	// 	blog.Errorf("engine(%s) mysql get project info(%s) failed: %v", EngineName, projectID, err)
	// 	return nil, err
	// }

	// if len(pl) < 1 {
	// 	err = engine.ErrorProjectNoFound
	// 	blog.Errorf("engine(%s) mysql get project info(%s) failed: %v", EngineName, projectID, err)
	// 	return nil, err
	// }

	var pl []*TableProjectInfo
	db := opts.AddWhere(m.db.Model(&TableProjectInfo{})).Where("disabled = ?", false)
	db = opts.AddOffsetLimit(db)
	db = opts.AddSelector(db)
	db = opts.AddOrder(db)

	if err := db.Find(&pl).Error; err != nil {
		blog.Errorf("engine(%s) mysql get project info failed opts(%v): %v", EngineName, opts, err)
		return nil, err
	}

	if len(pl) < 1 {
		return nil, engine.ErrorProjectNoFound
	}

	return pl[0], nil
}

// PutProjectInfo put project info with full fields.
func (m *mysql) PutProjectInfo(projectInfo *TableProjectInfo) error {
	defer timeMetricRecord("put_project_info")()
	defer logSlowFunc(time.Now().Unix(), "PutProjectInfo", 2)

	if err := m.db.Model(&TableProjectInfo{}).Save(projectInfo).Error; err != nil {
		blog.Errorf("engine(%s) mysql put project info(%s) failed: %v", EngineName, projectInfo.ProjectID, err)
		return err
	}
	return nil
}

// UpdateProjectInfo update project info with given fields.
func (m *mysql) UpdateProjectInfo(projectID string, projectInfo map[string]interface{}) error {
	defer timeMetricRecord("update_project_info")()
	defer logSlowFunc(time.Now().Unix(), "UpdateProjectInfo", 2)

	projectInfo["disabled"] = false

	if err := m.db.Model(&TableProjectInfo{}).Where("project_id = ?", projectID).
		Updates(projectInfo).Error; err != nil {
		blog.Errorf("engine(%s) mysql update project(%s) info(%+v) failed: %v", EngineName, projectID, projectInfo, err)
		return err
	}

	return nil
}

// DeleteProjectInfo delete project info from db. Just set the disabled to true instead of real deletion.
func (m *mysql) DeleteProjectInfo(projectID string) error {
	defer timeMetricRecord("delete_project_info")()
	defer logSlowFunc(time.Now().Unix(), "DeleteProjectInfo", 2)

	if err := m.db.Model(&TableProjectInfo{}).Where("project_id = ?", projectID).
		Update("disabled", true).Error; err != nil {
		blog.Errorf("engine(%s) mysql delete project info(%s) failed: %v", EngineName, projectID, err)
		return err
	}
	return nil
}

// AddProjectInfoStats add project info stats with given delta data, will lock the row and update some data.
func (m *mysql) AddProjectInfoStats(projectID string, delta DeltaInfoStats) error {
	defer timeMetricRecord("add_project_info_stats")()
	defer logSlowFunc(time.Now().Unix(), "AddProjectInfoStats", 2)

	tx := m.db.Begin()

	var pi TableProjectInfo
	pi.ProjectID = projectID
	if err := tx.Set("gorm:query_option", "FOR UPDATE").FirstOrCreate(&pi).Error; err != nil {
		tx.Rollback()
		blog.Errorf("engine(%s) mysql add project(%s) info stats, first or create failed: %v",
			EngineName, projectID, err)
		return err
	}

	pi.CompileFilesOK += delta.CompileFilesOK
	pi.CompileFilesErr += delta.CompileFilesErr
	pi.CompileFilesTimeout += delta.CompileFilesTimeout
	pi.CompileUnits += delta.CompileUnits

	if err := tx.Save(&pi).Error; err != nil {
		tx.Rollback()
		blog.Errorf("engine(%s) mysql add project(%s) info stats, save failed: %v", EngineName, projectID, err)
		return err
	}

	tx.Commit()
	return nil
}

// DeltaInfoStats describe the project info delta data.
type DeltaInfoStats struct {
	CompileFilesOK      int64
	CompileFilesErr     int64
	CompileFilesTimeout int64
	CompileUnits        float64
}

// ListProjectSetting list project setting from db, return list and total num.
// list cut by offset and limit, but total num describe the true num.
func (m *mysql) ListProjectSetting(opts commonMySQL.ListOptions) ([]*TableProjectSetting, int64, error) {
	defer timeMetricRecord("list_project_setting")()
	defer logSlowFunc(time.Now().Unix(), "ListProjectSetting", 2)

	var pl []*TableProjectSetting
	db := opts.AddWhere(m.db.Model(&TableProjectSetting{})).Where("disabled = ?", false)

	var length int64
	if err := db.Count(&length).Error; err != nil {
		blog.Errorf("engine(%s) mysql count project setting failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	db = opts.AddOffsetLimit(db)
	db = opts.AddSelector(db)
	db = opts.AddOrder(db)

	if err := db.Find(&pl).Error; err != nil {
		blog.Errorf("engine(%s) mysql list project setting failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	return pl, length, nil
}

// GetProjectSetting get project setting.
func (m *mysql) GetProjectSetting(projectID string) (*TableProjectSetting, error) {
	defer timeMetricRecord("get_project_setting")()
	defer logSlowFunc(time.Now().Unix(), "GetProjectSetting", 2)

	opts := commonMySQL.NewListOptions()
	opts.Limit(1)
	opts.Equal("project_id", projectID)

	// pl, _, err := m.ListProjectSetting(opts)
	// if err != nil {
	// 	blog.Errorf("engine(%s) mysql get project setting(%s) failed: %v", EngineName, projectID, err)
	// 	return nil, err
	// }

	// if len(pl) < 1 {
	// 	err = engine.ErrorProjectNoFound
	// 	blog.Errorf("engine(%s) mysql get project setting(%s) failed: %v", EngineName, projectID, err)
	// 	return nil, err
	// }

	var pl []*TableProjectSetting
	db := opts.AddWhere(m.db.Model(&TableProjectSetting{})).Where("disabled = ?", false)
	db = opts.AddOffsetLimit(db)
	db = opts.AddSelector(db)
	db = opts.AddOrder(db)

	if err := db.Find(&pl).Error; err != nil {
		blog.Errorf("engine(%s) mysql list project setting failed opts(%v): %v", EngineName, opts, err)
		return nil, err
	}

	if len(pl) < 1 {
		return nil, engine.ErrorProjectNoFound
	}

	return pl[0], nil
}

// PutProjectSetting put project setting with full fields.
func (m *mysql) PutProjectSetting(projectSetting *TableProjectSetting) error {
	defer timeMetricRecord("put_project_setting")()
	defer logSlowFunc(time.Now().Unix(), "PutProjectSetting", 2)

	if err := m.db.Model(&TableProjectSetting{}).Save(projectSetting).Error; err != nil {
		blog.Errorf("engine(%s) mysql put project setting(%s) failed: %v",
			EngineName, projectSetting.ProjectID, err)
		return err
	}
	return nil
}

// UpdateProjectSetting update project setting with given fields.
func (m *mysql) UpdateProjectSetting(projectID string, projectSetting map[string]interface{}) error {
	defer timeMetricRecord("update_project_setting")()
	defer logSlowFunc(time.Now().Unix(), "UpdateProjectSetting", 2)

	projectSetting["disabled"] = false

	if err := m.db.Model(&TableProjectSetting{}).Where("project_id = ?", projectID).
		Updates(projectSetting).Error; err != nil {
		blog.Errorf("engine(%s) mysql update project(%s) setting(%+v) failed: %v", EngineName,
			projectID, projectSetting, err)
		return err
	}

	return nil
}

// DeleteProjectSetting delete project setting from db. Just set the disabled to true instead of real deletion.
func (m *mysql) DeleteProjectSetting(projectID string) error {
	defer timeMetricRecord("delete_project_setting")()
	defer logSlowFunc(time.Now().Unix(), "DeleteProjectSetting", 2)

	if err := m.db.Model(&TableProjectSetting{}).Where("project_id = ?", projectID).
		Update("disabled", true).Error; err != nil {
		blog.Errorf("engine(%s) mysql delete project setting(%s) failed: %v", EngineName, projectID, err)
		return err
	}
	return nil
}

// CreateOrUpdateProjectSetting create a new project with struct or update a exist project setting with given fields.
func (m *mysql) CreateOrUpdateProjectSetting(
	projectSetting *TableProjectSetting,
	projectSettingRaw map[string]interface{}) error {
	defer timeMetricRecord("create_or_update_project_setting")()
	defer logSlowFunc(time.Now().Unix(), "CreateOrUpdateProjectSetting", 2)

	projectID, _ := projectSettingRaw["project_id"]
	projectSettingRaw["disabled"] = false

	if projectSetting != nil {
		if err := m.db.Table(TableProjectSetting{}.TableName()).Create(projectSetting).Error; err == nil {
			return nil
		}
	}

	if err := m.db.Model(&TableProjectSetting{}).Updates(projectSettingRaw).Error; err != nil {
		blog.Errorf("engine(%s) mysql create or update project setting failed ID(%s): %v", EngineName, projectID, err)
		return err
	}

	return nil
}

// ListWhitelist list whitelist from db, return list and total num.
// list cut by offset and limit, but total num describe the true num.
func (m *mysql) ListWhitelist(opts commonMySQL.ListOptions) ([]*TableWhitelist, int64, error) {
	defer timeMetricRecord("list_whitelist")()
	defer logSlowFunc(time.Now().Unix(), "timeMetricRecord", 2)

	var wll []*TableWhitelist
	db := opts.AddWhere(m.db.Model(&TableWhitelist{})).Where("disabled = ?", false)

	var length int64
	if err := db.Count(&length).Error; err != nil {
		blog.Errorf("engine(%s) count whitelist failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	db = opts.AddOffsetLimit(db)
	db = opts.AddSelector(db)
	db = opts.AddOrder(db)

	if err := db.Find(&wll).Error; err != nil {
		blog.Errorf("engine(%s) list whitelist failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	return wll, length, nil
}

// GetWhitelist get whitelist.
func (m *mysql) GetWhitelist(key engine.WhiteListKey) (*TableWhitelist, error) {
	defer timeMetricRecord("get_whitelist")()
	defer logSlowFunc(time.Now().Unix(), "GetWhitelist", 2)

	opts := commonMySQL.NewListOptions()
	opts.Limit(-1)
	opts.Equal("project_id", key.ProjectID)
	opts.Equal("ip", key.IP)

	// wll, _, err := m.ListWhitelist(opts)
	// if err != nil {
	// 	blog.Errorf("engine(%s) mysql get whitelist(%+v) failed: %v", EngineName, key, err)
	// 	return nil, err
	// }

	// if len(wll) < 1 {
	// 	err = engine.ErrorWhitelistNoFound
	// 	blog.Errorf("engine(%s) mysql get whitelist(%+v) failed: %v", EngineName, key, err)
	// 	return nil, err
	// }

	var wll []*TableWhitelist
	db := opts.AddWhere(m.db.Model(&TableWhitelist{})).Where("disabled = ?", false)
	db = opts.AddOffsetLimit(db)
	db = opts.AddSelector(db)
	db = opts.AddOrder(db)

	if err := db.Find(&wll).Error; err != nil {
		blog.Errorf("engine(%s) list whitelist failed opts(%v): %v", EngineName, opts, err)
		return nil, err
	}

	if len(wll) < 1 {
		return nil, engine.ErrorWhitelistNoFound
	}

	return wll[0], nil
}

// PutWhitelist put whitelist with full fields.
func (m *mysql) PutWhitelist(wll []*TableWhitelist) error {
	defer timeMetricRecord("put_whitelist")()
	defer logSlowFunc(time.Now().Unix(), "PutWhitelist", 2)

	tx := m.db.Begin()
	for _, wl := range wll {
		if err := tx.Model(&TableWhitelist{}).Save(wl).Error; err != nil {
			blog.Errorf("engine(%s) mysql put whitelist(%+v) failed: %v", EngineName, wl, err)
			tx.Rollback()
			return err
		}
	}
	tx.Commit()
	return nil
}

// UpdateWhitelist update whitelist with given fields.
func (m *mysql) UpdateWhitelist(key engine.WhiteListKey, wll []map[string]interface{}) error {
	defer timeMetricRecord("update_whitelist")()
	defer logSlowFunc(time.Now().Unix(), "UpdateWhitelist", 2)

	tx := m.db.Begin()
	for _, wl := range wll {
		wl["disabled"] = false
		if err := tx.Model(&TableWhitelist{}).Where("project_id = ?", key.ProjectID).
			Where("ip = ?", key.IP).Updates(wl).Error; err != nil {
			blog.Errorf("engine(%s) mysql update whitelist(%+v) failed: %v", EngineName, wl, err)
			tx.Rollback()
			return err
		}
	}
	tx.Commit()
	return nil
}

// DeleteWhitelist delete whitelist from db. Just set the disabled to true instead of real deletion.
func (m *mysql) DeleteWhitelist(keys []*engine.WhiteListKey) error {
	defer timeMetricRecord("delete_whitelist")()
	defer logSlowFunc(time.Now().Unix(), "DeleteWhitelist", 2)

	tx := m.db.Begin()
	for _, key := range keys {
		if err := tx.Model(&TableWhitelist{}).Where("ip = ?", key.IP).
			Where("project_id = ?", key.ProjectID).Update("disabled", true).Error; err != nil {
			blog.Errorf("engine(%s) mysql delete whitelist failed IP(%s) projectID(%s): %v",
				EngineName, key.IP, key.ProjectID, err)
			tx.Rollback()
			return err
		}
	}
	tx.Commit()
	return nil
}

// ListWorker list worker version from db, return list and total num.
// list cut by offset and limit, but total num describe the true num.
func (m *mysql) ListWorker(opts commonMySQL.ListOptions) ([]*TableWorker, int64, error) {
	defer timeMetricRecord("list_worker")()
	defer logSlowFunc(time.Now().Unix(), "ListWorker", 2)

	var gl []*TableWorker
	db := opts.AddWhere(m.db.Model(&TableWorker{})).Where("disabled = ?", false)

	var length int64
	if err := db.Count(&length).Error; err != nil {
		blog.Errorf("engine(%s) count worker failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	db = opts.AddOffsetLimit(db)
	db = opts.AddSelector(db)
	db = opts.AddOrder(db)

	if err := db.Find(&gl).Error; err != nil {
		blog.Errorf("engine(%s) list worker failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	return gl, length, nil
}

// GetWorker get worker.
func (m *mysql) GetWorker(version, scene string) (*TableWorker, error) {
	defer timeMetricRecord("get_worker")()
	defer logSlowFunc(time.Now().Unix(), "GetWorker", 2)

	opts := commonMySQL.NewListOptions()
	opts.Limit(1)
	opts.Equal("worker_version", version)
	opts.Equal("scene", scene)

	// gl, _, err := m.ListWorker(opts)
	// if err != nil {
	// 	blog.Errorf("engine(%s) mysql get worker(%s) failed: %v", EngineName, version, err)
	// 	return nil, err
	// }

	// if len(gl) < 1 {
	// 	err = fmt.Errorf("worker no found")
	// 	blog.Errorf("engine(%s) mysql get worker(%s) failed: %v", EngineName, version, err)
	// 	return nil, err
	// }

	var gl []*TableWorker
	db := opts.AddWhere(m.db.Model(&TableWorker{})).Where("disabled = ?", false)
	db = opts.AddOffsetLimit(db)
	db = opts.AddSelector(db)
	db = opts.AddOrder(db)

	if err := db.Find(&gl).Error; err != nil {
		blog.Errorf("engine(%s) list worker failed opts(%v): %v", EngineName, opts, err)
		return nil, err
	}

	if len(gl) < 1 {
		return nil, fmt.Errorf("worker no found")
	}

	return gl[0], nil
}

// PutWorker put worker with full fields.
func (m *mysql) PutWorker(worker *TableWorker) error {
	defer timeMetricRecord("put_worker")()
	defer logSlowFunc(time.Now().Unix(), "PutWorker", 2)

	if err := m.db.Model(&TableWorker{}).Save(worker).Error; err != nil {
		blog.Errorf("engine(%s) mysql put worker(%s) failed: %v", EngineName, worker.WorkerVersion, err)
		return err
	}
	return nil
}

// UpdateWorker update worker with given fields.
func (m *mysql) UpdateWorker(version, scene string, worker map[string]interface{}) error {
	defer timeMetricRecord("update_worker")()
	defer logSlowFunc(time.Now().Unix(), "UpdateWorker", 2)

	worker["disabled"] = false

	if err := m.db.Model(&TableWorker{}).Where("worker_version = ?", version).
		Where("scene = ?", scene).Updates(worker).Error; err != nil {
		blog.Errorf("engine(%s) mysql update worker(%s)(%+v) failed: %v", EngineName, version, worker, err)
		return err
	}

	return nil
}

// DeleteWorker delete worker from db. Just set the disabled to true instead of real deletion.
func (m *mysql) DeleteWorker(version, scene string) error {
	defer timeMetricRecord("delete_worker")()
	defer logSlowFunc(time.Now().Unix(), "DeleteWorker", 2)

	if err := m.db.Model(&TableWorker{}).Where("worker_version = ?", version).
		Where("scene = ?", scene).Update("disabled", true).Error; err != nil {
		blog.Errorf("engine(%s) mysql delete worker(%s) failed: %v", EngineName, version, err)
		return err
	}
	return nil
}

// CombinedProject generate project_settings and project_records
type CombinedProject struct {
	*TableProjectSetting
	*TableProjectInfo
}

// ListWorkStats list work stats from db, return list and total num.
// list cut by offset and limit, but total num describe the true num.
func (m *mysql) ListWorkStats(opts commonMySQL.ListOptions) ([]*TableWorkStats, int64, error) {
	defer timeMetricRecord("list_work_stats")()
	defer logSlowFunc(time.Now().Unix(), "ListWorkStats", 2)

	var tl []*TableWorkStats
	db := opts.AddWhere(m.db.Model(&TableWorkStats{})).Where("disabled = ?", false)

	var length int64
	if err := db.Count(&length).Error; err != nil {
		blog.Errorf("engine(%s) mysql count work stats failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	db = opts.AddOffsetLimit(db)
	db = opts.AddSelector(db)
	db = opts.AddOrder(db)

	if err := db.Find(&tl).Error; err != nil {
		blog.Errorf("engine(%s) mysql list work stats failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	return tl, length, nil
}

// GetWorkStats get work stats.
func (m *mysql) GetWorkStats(taskID, workID string) (*TableWorkStats, error) {
	defer timeMetricRecord("get_work_stats")()
	defer logSlowFunc(time.Now().Unix(), "GetWorkStats", 2)

	opts := commonMySQL.NewListOptions()
	opts.Limit(1)
	// opts.Equal("id", id)
	opts.Equal("task_id", taskID)
	opts.Equal("work_id", workID)

	// tl, _, err := m.ListWorkStats(opts)
	// if err != nil {
	// 	blog.Errorf("engine(%s) mysql get work stats(%d) failed: %v", EngineName, id, err)
	// 	return nil, err
	// }

	// if len(tl) < 1 {
	// 	err = fmt.Errorf("work stats no found")
	// 	blog.Errorf("engine(%s) mysql get work stats(%d) failed: %v", EngineName, id, err)
	// 	return nil, err
	// }

	var tl []*TableWorkStats
	db := opts.AddWhere(m.db.Model(&TableWorkStats{})).Where("disabled = ?", false)
	db = opts.AddOffsetLimit(db)
	db = opts.AddSelector(db)
	db = opts.AddOrder(db)

	if err := db.Find(&tl).Error; err != nil {
		blog.Errorf("engine(%s) mysql list work stats failed opts(%v): %v", EngineName, opts, err)
		return nil, err
	}

	if len(tl) < 1 {
		// do not return error, same as ListWorkStats
		return nil, nil
	}

	return tl[0], nil
}

// PutWorkStats put work stats with full fields.
func (m *mysql) PutWorkStats(stats *TableWorkStats) error {
	defer timeMetricRecord("put_work_stats")()
	defer logSlowFunc(time.Now().Unix(), "PutWorkStats", 2)

	if stats.ID > 0 {
		// 增加更新条件，只有新的JobStats 长度不小于老的JobStats长度才更新
		newstatlen := len(stats.JobStats)
		if err := m.db.Model(&TableWorkStats{}).Where("id = ?", stats.ID).
			Where("? >= CHAR_LENGTH(job_stats)", newstatlen).
			Updates(stats).Error; err != nil {
			blog.Errorf("engine(%s) mysql put work stats(%d)(%+v) failed: %v", EngineName, stats.ID, stats, err)
			return err
		}
	} else {
		if err := m.db.Model(&TableWorkStats{}).Save(stats).Error; err != nil {
			blog.Errorf("engine(%s) mysql put work stats(%s) failed: %v", EngineName, stats.TaskID, err)
			return err
		}
	}
	return nil
}

// UpdateWorkStats update work stats with given fields.
func (m *mysql) UpdateWorkStats(id int, stats map[string]interface{}) error {
	defer timeMetricRecord("update_work_stats")()
	defer logSlowFunc(time.Now().Unix(), "UpdateWorkStats", 2)

	stats["disabled"] = false

	if err := m.db.Model(&TableWorkStats{}).Where("id = ?", id).Updates(stats).Error; err != nil {
		blog.Errorf("engine(%s) mysql update work stats(%d)(%+v) failed: %v", EngineName, id, stats, err)
		return err
	}

	return nil
}

// DeleteWorkStats delete work stats from db. Just set the disabled to true instead of real deletion.
func (m *mysql) DeleteWorkStats(id int) error {
	defer timeMetricRecord("delete_task")()
	defer logSlowFunc(time.Now().Unix(), "DeleteWorkStats", 2)

	if err := m.db.Model(&TableWorkStats{}).Where("id = ?", id).
		Update("disabled", true).Error; err != nil {
		blog.Errorf("engine(%s) mysql delete work stats(%d) failed: %v", EngineName, id, err)
		return err
	}
	return nil
}

// SummaryTaskRecords summary task records.
func (m *mysql) SummaryTaskRecords(opts commonMySQL.ListOptions) ([]*SummaryResult, int64, error) {
	defer timeMetricRecord("summary_task_records")()
	defer logSlowFunc(time.Now().Unix(), "summary_task_records", 2)

	var tl []*SummaryResult
	// db := opts.AddWhere(m.db.Model(&TableTask{})).Where("disabled = ?", false)
	db := m.db.Table("task_records").Where("disabled = ?", false)

	var length int64
	// if err := db.Count(&length).Error; err != nil {
	// 	blog.Errorf("engine(%s) mysql summary task failed opts(%v): %v", EngineName, opts, err)
	// 	return nil, 0, err
	// }

	db = opts.AddSelector(db)
	db = opts.AddWhere(db)
	db = opts.AddGroup(db)

	if err := db.Find(&tl).Error; err != nil {
		blog.Errorf("engine(%s) mysql summary task failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	return tl, length, nil
}

// SummaryTaskRecordsByUser summary task records group by user.
func (m *mysql) SummaryTaskRecordsByUser(opts commonMySQL.ListOptions) ([]*SummaryResultByUser, int64, error) {
	defer timeMetricRecord("summary_task_records")()
	defer logSlowFunc(time.Now().Unix(), "summary_task_records", 2)

	var tl []*SummaryResultByUser
	// db := opts.AddWhere(m.db.Model(&TableTask{})).Where("disabled = ?", false)
	db := m.db.Table("task_records").Where("disabled = ?", false)

	var length int64
	// if err := db.Count(&length).Error; err != nil {
	// 	blog.Errorf("engine(%s) mysql summary task failed opts(%v): %v", EngineName, opts, err)
	// 	return nil, 0, err
	// }

	db = opts.AddSelector(db)
	db = opts.AddWhere(db)
	db = opts.AddGroup(db)

	if err := db.Find(&tl).Error; err != nil {
		blog.Errorf("engine(%s) mysql summary task failed opts(%v): %v", EngineName, opts, err)
		return nil, 0, err
	}

	return tl, length, nil
}

// SummaryResult generate summary data
type SummaryResult struct {
	Day               string  `json:"day"`
	ProjectID         string  `json:"project_id"`
	TotalTime         float64 `json:"total_time"`
	TotalTimeWithCPU  float64 `json:"total_time_with_cpu"`
	TotalRecordNumber int     `json:"total_record_number"`
}

// SummaryResultByUser generate summary data group by user
type SummaryResultByUser struct {
	Day               string  `json:"day"`
	ProjectID         string  `json:"project_id"`
	User              string  `json:"user"`
	TotalTime         float64 `json:"total_time"`
	TotalTimeWithCPU  float64 `json:"total_time_with_cpu"`
	TotalRecordNumber int     `json:"total_record_number"`
}

func timeMetricRecord(operation string) func() {
	return selfMetric.TimeMetricRecord(fmt.Sprintf("%s_%s", EngineName, operation))
}

func logSlowFunc(start int64, funcname string, threshold int64) {
	now := time.Now().Unix()
	if now-start > threshold {
		blog.Warnf("engine(%s) mysql func(%s) too long %d seconds", EngineName, funcname, now-start)
	}
}
