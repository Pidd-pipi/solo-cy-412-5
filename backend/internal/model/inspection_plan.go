package model

import "time"

// InspectionPlan 巡检计划：同一设施同一周期只能有一条计划（唯一索引兜底）。
type InspectionPlan struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	FacilityID uint      `gorm:"uniqueIndex:idx_plan_facility_cycle" json:"facility_id"`
	Facility   Facility  `json:"facility"`
	Name       string    `gorm:"size:100" json:"name"`
	Cycle      string    `gorm:"size:10;uniqueIndex:idx_plan_facility_cycle" json:"cycle"` // daily | weekly | monthly | quarterly | yearly
	StartDate  time.Time `json:"start_date"`
	Active     bool      `gorm:"index" json:"active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
