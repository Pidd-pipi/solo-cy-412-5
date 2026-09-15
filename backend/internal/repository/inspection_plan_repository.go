package repository

import (
	"errors"
	"github.com/smartestate/smartestate/internal/model"
	"gorm.io/gorm"
)

type InspectionPlanRepository struct{ DB *gorm.DB }

func NewInspectionPlanRepository(db *gorm.DB) *InspectionPlanRepository {
	return &InspectionPlanRepository{db}
}

func (r *InspectionPlanRepository) Create(v *model.InspectionPlan) error {
	if e := r.DB.Create(v).Error; e != nil {
		if IsDuplicate(e) {
			return ErrDuplicate
		}
		return e
	}
	return nil
}

func (r *InspectionPlanRepository) List(active *bool) (out []model.InspectionPlan, e error) {
	q := r.DB.Preload("Facility").Order("created_at desc")
	if active != nil {
		q = q.Where("active = ?", *active)
	}
	e = q.Find(&out).Error
	return
}

func (r *InspectionPlanRepository) ByID(id uint) (v model.InspectionPlan, e error) {
	e = r.DB.Preload("Facility").First(&v, id).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = ErrNotFound
	}
	return
}

// ByFacilityAndCycle 同设施同周期最多一条计划。
func (r *InspectionPlanRepository) ByFacilityAndCycle(facilityID uint, cycle string) (v model.InspectionPlan, e error) {
	e = r.DB.Where("facility_id = ? AND cycle = ?", facilityID, cycle).First(&v).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = ErrNotFound
	}
	return
}

func (r *InspectionPlanRepository) Update(v *model.InspectionPlan) error { return r.DB.Save(v).Error }

// ListActiveDue 到期生成任务时使用：取所有启用计划，由服务层统一重跑。
func (r *InspectionPlanRepository) ListActiveDue() (out []model.InspectionPlan, e error) {
	e = r.DB.Preload("Facility").Where("active = ?", true).Find(&out).Error
	return
}
