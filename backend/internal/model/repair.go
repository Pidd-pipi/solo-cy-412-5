package model

import "time"

type Repair struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	UserID      uint   `gorm:"index" json:"user_id"`
	User        User   `json:"user"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Images      string `json:"images"`
	Status      string `gorm:"index;size:20" json:"status"`
	HandlerID   *uint  `json:"handler_id"`
	Handler     *User  `gorm:"foreignKey:HandlerID" json:"handler,omitempty"`
	// FacilityID 非空表示该工单由巡检隐患/复检未通过自动生成。
	FacilityID *uint     `gorm:"index" json:"facility_id"`
	Facility   *Facility `gorm:"foreignKey:FacilityID" json:"facility,omitempty"`
	// SourceTaskID 触发生成该工单的巡检任务，一对一，避免重复生成工单。
	SourceTaskID *uint     `gorm:"uniqueIndex" json:"source_task_id"`
	Rating       int       `json:"rating"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
