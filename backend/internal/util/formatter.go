package util

import (
	"fmt"
	"github.com/smartestate/smartestate/internal/constants"
	"time"
)

func Date(v time.Time) string { return v.Format("2006-01-02 15:04") }
func Money(v float64) string  { return fmt.Sprintf("¥%.2f", v) }
func StatusText(v string) string {
	m := map[string]string{constants.RepairStatusPending: "待受理", constants.RepairStatusAssigned: "已分派", constants.RepairStatusProcessing: "处理中", constants.RepairStatusDone: "已完成", constants.RepairStatusClosed: "已关闭"}
	return m[v]
}
func RoleText(v string) string {
	m := map[string]string{constants.UserRoleResident: "业主", constants.UserRoleStaff: "物业人员", constants.UserRoleAdmin: "管理员"}
	return m[v]
}

// FacilityStatusText 设施状态中文。
func FacilityStatusText(v string) string {
	m := map[string]string{constants.FacilityStatusAvailable: "可用", constants.FacilityStatusDisabled: "停用"}
	return m[v]
}

// TaskStatusText 巡检任务状态中文。
func TaskStatusText(v string) string {
	m := map[string]string{
		constants.TaskStatusPending:       "待巡检",
		constants.TaskStatusClaimed:       "已接单",
		constants.TaskStatusDone:          "巡检正常",
		constants.TaskStatusHazard:        "发现隐患·停用",
		constants.TaskStatusRecheckFailed: "复检未过·停用",
		constants.TaskStatusRestored:      "复检通过·恢复",
	}
	return m[v]
}

// CycleText 巡检周期中文。
func CycleText(v string) string {
	m := map[string]string{
		constants.CycleDaily: "每日", constants.CycleWeekly: "每周",
		constants.CycleMonthly: "每月", constants.CycleQuarterly: "每季度",
		constants.CycleYearly: "每年",
	}
	return m[v]
}
