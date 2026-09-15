package repository

import (
	"errors"
	"github.com/smartestate/smartestate/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RepairRepository struct{ DB *gorm.DB }

func NewRepairRepository(db *gorm.DB) *RepairRepository  { return &RepairRepository{db} }
func (r *RepairRepository) Create(v *model.Repair) error { return r.DB.Create(v).Error }

// CreateInTx 在事务内创建巡检关联维修工单；source_task_id 唯一索引保证一张巡检任务只生成一张工单。
func (r *RepairRepository) CreateInTx(tx *gorm.DB, v *model.Repair) error {
	if e := tx.Create(v).Error; e != nil {
		if IsDuplicate(e) {
			return ErrDuplicate
		}
		return e
	}
	return nil
}

func (r *RepairRepository) List(status string) (out []model.Repair, e error) {
	q := r.DB.Preload("User").Preload("Handler").Preload("Facility").Order("created_at desc")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	e = q.Find(&out).Error
	return
}
func (r *RepairRepository) ByID(id uint) (v model.Repair, e error) {
	e = r.DB.Preload("User").Preload("Handler").Preload("Facility").First(&v, id).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = ErrNotFound
	}
	return
}

// ByIDForUpdate 事务内加行锁读取工单，供“维修完成 → 生成复检任务”原子使用。
func (r *RepairRepository) ByIDForUpdate(tx *gorm.DB, id uint) (v model.Repair, e error) {
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

func (r *RepairRepository) Update(v *model.Repair) error { return r.DB.Save(v).Error }

// AdvanceStatusInTx 条件更新工单状态（CAS）：仅当当前状态属于 fromStatuses 时生效。
func (r *RepairRepository) AdvanceStatusInTx(tx *gorm.DB, id uint, fromStatuses []string, status string, fields map[string]interface{}) (bool, error) {
	if fields == nil {
		fields = map[string]interface{}{}
	}
	fields["status"] = status
	res := tx.Model(&model.Repair{}).Where("id = ? AND status IN ?", id, fromStatuses).Updates(fields)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// ListByFacility 设施详情中的处置进度：与该设施关联的巡检维修工单（最新在前）。
func (r *RepairRepository) ListByFacility(facilityID uint) (out []model.Repair, e error) {
	e = r.DB.Preload("Handler").
		Where("facility_id = ?", facilityID).
		Order("created_at desc").Find(&out).Error
	return
}

func (r *RepairRepository) CountOpen() (int64, error) {
	var n int64
	e := r.DB.Model(&model.Repair{}).Where("status NOT IN ?", []string{"done", "closed"}).Count(&n).Error
	return n, e
}

// CountUnclosedFacility 巡检停用处置中尚未闭环（done/closed）的关联维修工单数。
func (r *RepairRepository) CountUnclosedFacility() (int64, error) {
	var n int64
	e := r.DB.Model(&model.Repair{}).
		Where("facility_id IS NOT NULL AND status NOT IN ?", []string{"done", "closed"}).
		Count(&n).Error
	return n, e
}
