package repository

import (
	"errors"
	"github.com/smartestate/smartestate/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FacilityRepository struct{ DB *gorm.DB }

func NewFacilityRepository(db *gorm.DB) *FacilityRepository { return &FacilityRepository{db} }

func (r *FacilityRepository) Create(v *model.Facility) error { return r.DB.Create(v).Error }

func (r *FacilityRepository) List(status string) (out []model.Facility, e error) {
	q := r.DB.Order("created_at desc")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	e = q.Find(&out).Error
	return
}

func (r *FacilityRepository) ByID(id uint) (v model.Facility, e error) {
	e = r.DB.First(&v, id).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = ErrNotFound
	}
	return
}

// ByIDForUpdate 在事务内加行锁读取设施，保证停用/恢复与任务推进原子且互斥。
// MySQL 使用 SELECT ... FOR UPDATE；SQLite 不支持该子句，依靠其写锁串行化。
func (r *FacilityRepository) ByIDForUpdate(tx *gorm.DB, id uint) (v model.Facility, e error) {
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

func (r *FacilityRepository) Update(v *model.Facility) error { return r.DB.Save(v).Error }

// UpdateStatusInTx 在事务内按条件更新设施状态（GORM 自动维护 updated_at）。
func (r *FacilityRepository) UpdateStatusInTx(tx *gorm.DB, id uint, status string) error {
	return tx.Model(&model.Facility{}).Where("id = ?", id).
		Update("status", status).Error
}

func (r *FacilityRepository) CountByStatus(status string) (int64, error) {
	var n int64
	e := r.DB.Model(&model.Facility{}).Where("status = ?", status).Count(&n).Error
	return n, e
}
