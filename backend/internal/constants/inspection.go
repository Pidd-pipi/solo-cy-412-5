package constants

// 巡检周期
const (
	CycleDaily     = "daily"
	CycleWeekly    = "weekly"
	CycleMonthly   = "monthly"
	CycleQuarterly = "quarterly"
	CycleYearly    = "yearly"
)

// ValidCycles 合法巡检周期
var ValidCycles = map[string]bool{
	CycleDaily: true, CycleWeekly: true, CycleMonthly: true,
	CycleQuarterly: true, CycleYearly: true,
}

// 设施状态
const (
	FacilityStatusAvailable = "available"
	FacilityStatusDisabled  = "disabled"
)

// ValidFacilityStatuses 合法设施状态
var ValidFacilityStatuses = map[string]bool{
	FacilityStatusAvailable: true, FacilityStatusDisabled: true,
}

// 巡检任务种类
const (
	TaskKindRoutine = "routine"
	TaskKindRecheck = "recheck"
)

// 巡检任务状态
const (
	TaskStatusPending       = "pending"
	TaskStatusClaimed       = "claimed"
	TaskStatusDone          = "done"           // 常规巡检无隐患（终态）
	TaskStatusHazard        = "hazard"         // 巡检发现隐患，设施停用、已建工单（终态）
	TaskStatusRecheckFailed = "recheck_failed" // 复检未通过，保持停用、已续建工单（终态）
	TaskStatusRestored      = "restored"       // 复检通过，设施恢复可用（终态）
)

// ValidTaskStatuses 合法任务状态
var ValidTaskStatuses = map[string]bool{
	TaskStatusPending: true, TaskStatusClaimed: true, TaskStatusDone: true,
	TaskStatusHazard: true, TaskStatusRecheckFailed: true, TaskStatusRestored: true,
}

// TerminalTaskStatuses 终态任务，结果不允许被后续调整改写
var TerminalTaskStatuses = map[string]bool{
	TaskStatusDone: true, TaskStatusHazard: true,
	TaskStatusRecheckFailed: true, TaskStatusRestored: true,
}

// 巡检结果
const (
	ResultNormal = "normal" // 常规巡检正常
	ResultHazard = "hazard" // 常规巡检发现隐患
	ResultPass   = "pass"   // 复检通过
	ResultFail   = "fail"   // 复检未通过
)
