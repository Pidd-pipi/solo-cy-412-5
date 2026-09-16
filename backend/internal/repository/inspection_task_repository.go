package repository

import (
	"errors"
	"github.com/smartestate/smartestate/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type InspectionTaskRepository struct{ DB *gorm.DB }

func NewInspectionTaskRepository(db *gorm.DB) *InspectionTaskRepository {
	return &InspectionTaskRepository{db}
}

func taskPreloads(q *gorm.DB) *gorm.DB {
	return q.Preload("Facility").Preload("Plan").Preload("Inspector").
		Preload("HazardRepair").Preload("SourceRepair")
}

// Create 新建任务；唯一索引（同设施/周期/期次/种类）冲突时返回 ErrDuplicate。
func (r *InspectionTaskRepository) Create(v *model.InspectionTask) error {
	if e := r.DB.Create(v).Error; e != nil {
		if IsDuplicate(e) {
			return ErrDuplicate
		}
		return e
	}
	return nil
}

// CreateInTx 在给定事务内创建任务（供停用/复检等组合写操作使用）。
func (r *InspectionTaskRepository) CreateInTx(tx *gorm.DB, v *model.InspectionTask) error {
	if e := tx.Create(v).Error; e != nil {
		if IsDuplicate(e) {
			return ErrDuplicate
		}
		return e
	}
	return nil
}

type TaskFilter struct {
	FacilityID  uint
	Status      string
	Kind        string
	StatusNotIn []string
}

func (r *InspectionTaskRepository) List(f TaskFilter) (out []model.InspectionTask, e error) {
	q := taskPreloads(r.DB).Order("due_date asc, created_at desc")
	if f.FacilityID != 0 {
		q = q.Where("facility_id = ?", f.FacilityID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if len(f.StatusNotIn) > 0 {
		q = q.Where("status NOT IN ?", f.StatusNotIn)
	}
	e = q.Find(&out).Error
	return
}

func (r *InspectionTaskRepository) ByID(id uint) (v model.InspectionTask, e error) {
	e = taskPreloads(r.DB).First(&v, id).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = ErrNotFound
	}
	return
}

// ByIDForUpdate 事务内加行锁读取任务。
func (r *InspectionTaskRepository) ByIDForUpdate(tx *gorm.DB, id uint) (v model.InspectionTask, e error) {
	q := tx
	if r.DB.Dialector.Name() != "sqlite" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	e = q.First(&v, id).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = ErrNotFound
	}
	return
}

// Claim 原子接单：仅当任务处于 fromStatus 时把状态置为 claimed 并写入巡检人。
// 返回 rowsAffected==0 表示已被他人接单（或状态已变），实现“多人同时接单只有一个成功”。
func (r *InspectionTaskRepository) Claim(id, inspectorID uint, fromStatus string) (bool, error) {
	res := r.DB.Model(&model.InspectionTask{}).
		Where("id = ? AND status = ?", id, fromStatus).
		Updates(map[string]interface{}{
			"status":       "claimed",
			"inspector_id": inspectorID,
			"updated_at":   time.Now(),
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// Advance 原子推进任务状态（CAS）。仅当当前状态属于 fromStatuses 时更新给定字段，
// 用于提交巡检/复检，保证重复提交或并发提交只有一个结果，终态不可改写。
func (r *InspectionTaskRepository) Advance(id uint, fromStatuses []string, fields map[string]interface{}) (bool, error) {
	res := r.DB.Model(&model.InspectionTask{}).
		Where("id = ? AND status IN ?", id, fromStatuses).
		Updates(fields)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// AdvanceInTx 事务版本的条件推进。
func (r *InspectionTaskRepository) AdvanceInTx(tx *gorm.DB, id uint, fromStatuses []string, fields map[string]interface{}) (bool, error) {
	res := tx.Model(&model.InspectionTask{}).
		Where("id = ? AND status IN ?", id, fromStatuses).
		Updates(fields)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// ExistsSlot 判断同设施/周期/期次/种类的任务是否已存在（生成前快速判重，唯一索引最终兜底）。
func (r *InspectionTaskRepository) ExistsSlot(facilityID uint, kind, cycle, period string) (bool, error) {
	var n int64
	e := r.DB.Model(&model.InspectionTask{}).
		Where("facility_id = ? AND kind = ? AND cycle = ? AND period_value = ?", facilityID, kind, cycle, period).
		Count(&n).Error
	return n > 0, e
}

// RecheckExists 判断某张维修单是否已经安排过复检任务（一单一复检）。
func (r *InspectionTaskRepository) RecheckExists(repairID uint) (bool, error) {
	return r.recheckExists(r.DB, repairID)
}

// RecheckExistsTx 事务内版本，保证与创建在同一事务可见性下判重。
func (r *InspectionTaskRepository) RecheckExistsTx(tx *gorm.DB, repairID uint) (bool, error) {
	return r.recheckExists(tx, repairID)
}

func (r *InspectionTaskRepository) recheckExists(q *gorm.DB, repairID uint) (bool, error) {
	var n int64
	e := q.Model(&model.InspectionTask{}).
		Where("kind = ? AND source_repair_id = ?", "recheck", repairID).
		Count(&n).Error
	return n > 0, e
}

// CountOpenRecheckByFacilityTx 事务内统计某设施仍待处理（pending/claimed）的复检任务数。
// excludeTaskID 用于在“本次复检通过”推进状态前排除当前复检本身，避免把正在关闭的复检计为剩余。
func (r *InspectionTaskRepository) CountOpenRecheckByFacilityTx(tx *gorm.DB, facilityID uint, excludeTaskID uint) (int64, error) {
	var n int64
	q := tx.Model(&model.InspectionTask{}).
		Where("facility_id = ? AND kind = ? AND status IN ?",
			facilityID, "recheck", []string{"pending", "claimed"})
	if excludeTaskID != 0 {
		q = q.Where("id <> ?", excludeTaskID)
	}
	e := q.Count(&n).Error
	return n, e
}

// CountOpenRecheck 全局待处理复检任务数（工作台“未闭环巡检工单”指标，与停用状态保持同步）。
func (r *InspectionTaskRepository) CountOpenRecheck() (int64, error) {
	var n int64
	e := r.DB.Model(&model.InspectionTask{}).
		Where("kind = ? AND status IN ?", "recheck", []string{"pending", "claimed"}).
		Count(&n).Error
	return n, e
}

// CountDue 待巡检数量：处于 pending/claimed 且已到期的常规与复检任务（实时计数，不做累加）。
func (r *InspectionTaskRepository) CountDue(now time.Time) (int64, error) {
	var n int64
	e := r.DB.Model(&model.InspectionTask{}).
		Where("status IN ? AND due_date <= ?",
			[]string{"pending", "claimed"}, now).Count(&n).Error
	return n, e
}

// ListByFacilityAndStatus 取某设施指定状态的任务。
func (r *InspectionTaskRepository) ListByFacilityAndStatus(facilityID uint, statuses []string) (out []model.InspectionTask, e error) {
	e = r.DB.Where("facility_id = ? AND status IN ?", facilityID, statuses).Find(&out).Error
	return
}

// LatestByFacility 取该设施最近一条任务（用于推导复检任务的周期槽位）。
func (r *InspectionTaskRepository) LatestByFacility(facilityID uint) (v model.InspectionTask, e error) {
	e = r.DB.Where("facility_id = ?", facilityID).Order("created_at desc").First(&v).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = ErrNotFound
	}
	return
}
