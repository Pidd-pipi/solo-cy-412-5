package model

import "time"

// Facility 公共设施。状态（可用/停用）只在巡检复检流程中流转，由后端在事务内维护。
type Facility struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"index;size:100" json:"name"`
	Category  string    `gorm:"size:50" json:"category"` // 消防/电梯/配电/给排水/健身器材 等
	Location  string    `gorm:"size:200" json:"location"`
	Status    string    `gorm:"index;size:20" json:"status"` // available | disabled
	Remark    string    `json:"remark"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
