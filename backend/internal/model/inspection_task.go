package model

import "time"

// InspectionTask 巡检任务。
// 同一设施、同一周期、同一期次、同一种类只能有一条任务（唯一索引 idx_task_slot 兜底），
// 因此计划重跑/到期重算不会产生重复任务。
// kind=routine 为常规巡检（PeriodValue 为周期键，如 2026-09）；
// kind=recheck 为维修复检（PeriodValue 形如 recheck-<repair_id>）。
type InspectionTask struct {
	ID             uint            `gorm:"primaryKey" json:"id"`
	FacilityID     uint            `gorm:"index;uniqueIndex:idx_task_slot" json:"facility_id"`
	Facility       Facility        `json:"facility"`
	PlanID         *uint           `gorm:"index" json:"plan_id"`
	Plan           *InspectionPlan `gorm:"foreignKey:PlanID" json:"plan,omitempty"`
	Kind           string          `gorm:"size:10;index;uniqueIndex:idx_task_slot" json:"kind"`
	Cycle          string          `gorm:"size:10;uniqueIndex:idx_task_slot" json:"cycle"`
	PeriodValue    string          `gorm:"size:40;uniqueIndex:idx_task_slot" json:"period_value"`
	DueDate        time.Time       `gorm:"index" json:"due_date"`
	Status         string          `gorm:"index;size:20" json:"status"` // pending|claimed|done|hazard|recheck_failed|restored
	InspectorID    *uint           `json:"inspector_id"`
	Inspector      *User           `gorm:"foreignKey:InspectorID" json:"inspector,omitempty"`
	Result         string          `gorm:"size:20" json:"result"` // normal|hazard|pass|fail
	Finding        string          `json:"finding"`
	SourceRepairID *uint           `gorm:"uniqueIndex" json:"source_repair_id"`
	SourceRepair   *Repair         `gorm:"foreignKey:SourceRepairID" json:"source_repair,omitempty"`
	HazardRepairID *uint           `gorm:"uniqueIndex" json:"hazard_repair_id"`
	HazardRepair   *Repair         `gorm:"foreignKey:HazardRepairID" json:"hazard_repair,omitempty"`
	ReviewedAt     *time.Time      `json:"reviewed_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}
