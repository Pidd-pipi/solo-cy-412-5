package service

import (
	"errors"
	"testing"
	"time"

	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/model"
	"github.com/smartestate/smartestate/internal/repository"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

// 跨多个周期重跑：从起始期补齐到当前期，每期一条；重跑幂等，待巡检数不重复累计。
func TestGenerateDueBackfillsMissedMonths(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "跨月电梯")
	if _, e := fx.planSvc.Create(f.ID, "月度巡检", constants.CycleMonthly, date(2026, 1, 15)); e != nil {
		t.Fatalf("建计划: %v", e)
	}
	n, e := fx.planSvc.GenerateDue(date(2026, 3, 10))
	if e != nil {
		t.Fatalf("补齐生成失败: %v", e)
	}
	if n != 3 {
		t.Fatalf("维修完成口径：应补齐 3 期, 实际 %d", n)
	}
	tasks, _ := fx.tasks.List(repository.TaskFilter{FacilityID: f.ID, Kind: constants.TaskKindRoutine})
	got := periodSet(tasks)
	for _, want := range []string{"2026-01", "2026-02", "2026-03"} {
		if !got[want] {
			t.Fatalf("缺少期次 %s，实际 %v", want, keys(got))
		}
	}
	// 重跑：不新增、不重复累计。
	if n2, _ := fx.planSvc.GenerateDue(date(2026, 3, 20)); n2 != 0 {
		t.Fatalf("重跑不应新增任务, 实际新增 %d", n2)
	}
	if due, _ := fx.taskSvc.DueCount(); due != 3 {
		t.Fatalf("待巡检数量应=3（不重复累计）, 实际 %d", due)
	}
	// 进入下月：仅补新一期。
	if n3, _ := fx.planSvc.GenerateDue(date(2026, 4, 5)); n3 != 1 {
		t.Fatalf("跨到下月应只补 1 期, 实际 %d", n3)
	}
	tasks, _ = fx.tasks.List(repository.TaskFilter{FacilityID: f.ID, Kind: constants.TaskKindRoutine})
	if len(tasks) != 4 {
		t.Fatalf("总任务应=4, 实际 %d", len(tasks))
	}
}

// 已存在（含已完成/隐患停用等终态）的期次不得被改写，只补缺失期。
func TestGenerateDueKeepsTerminalPeriods(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "终态期次电梯")
	plan := model.InspectionPlan{FacilityID: f.ID, Name: "月度", Cycle: constants.CycleMonthly,
		StartDate: date(2026, 1, 1), Active: true}
	if e := fx.db.Create(&plan).Error; e != nil {
		t.Fatalf("建计划: %v", e)
	}
	// 预置 2026-02 为已隐患停用的终态任务。
	pre := model.InspectionTask{FacilityID: f.ID, PlanID: &plan.ID, Kind: constants.TaskKindRoutine,
		Cycle: constants.CycleMonthly, PeriodValue: "2026-02", DueDate: date(2026, 2, 1),
		Status: constants.TaskStatusHazard}
	if e := fx.db.Create(&pre).Error; e != nil {
		t.Fatalf("预置终态任务: %v", e)
	}
	n, e := fx.planSvc.GenerateDue(date(2026, 3, 10))
	if e != nil {
		t.Fatalf("复检提交口径补齐失败: %v", e)
	}
	if n != 2 {
		t.Fatalf("应只补 2026-01、2026-03 两期, 实际 %d", n)
	}
	tasks, _ := fx.tasks.List(repository.TaskFilter{FacilityID: f.ID, Kind: constants.TaskKindRoutine})
	if len(tasks) != 3 {
		t.Fatalf("总任务应=3, 实际 %d", len(tasks))
	}
	var hazardKept bool
	for _, tk := range tasks {
		if tk.PeriodValue == "2026-02" {
			if tk.Status != constants.TaskStatusHazard {
				t.Fatalf("工单关闭口径：终态期次被改写为 %s", tk.Status)
			}
			hazardKept = true
		}
	}
	if !hazardKept {
		t.Fatalf("状态回读：未找到被保留的 2026-02 终态任务")
	}
}

// 补生成中途失败后重试：已成功期次保留，只继续缺失期次，不重复。
func TestGenerateDueResumesOnlyMissingPeriods(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "续跑电梯")
	plan := model.InspectionPlan{FacilityID: f.ID, Name: "月度", Cycle: constants.CycleMonthly,
		StartDate: date(2026, 1, 1), Active: true}
	if e := fx.db.Create(&plan).Error; e != nil {
		t.Fatalf("建计划: %v", e)
	}
	mk := func(period string, due time.Time, status string) {
		tk := model.InspectionTask{FacilityID: f.ID, PlanID: &plan.ID, Kind: constants.TaskKindRoutine,
			Cycle: constants.CycleMonthly, PeriodValue: period, DueDate: due, Status: status}
		if e := fx.db.Create(&tk).Error; e != nil {
			t.Fatalf("状态回读：预置已成功期次 %s: %v", period, e)
		}
	}
	// 模拟上一次中途失败：2026-01、2026-03 已成功写入，2026-02 缺失。
	mk("2026-01", date(2026, 1, 1), constants.TaskStatusDone)
	mk("2026-03", date(2026, 3, 1), constants.TaskStatusPending)
	n, e := fx.planSvc.GenerateDue(date(2026, 3, 31))
	if e != nil {
		t.Fatalf("维修完成口径续跑失败: %v", e)
	}
	if n != 1 {
		t.Fatalf("重试应只补缺失的 2026-02（1 期）, 实际 %d", n)
	}
	tasks, _ := fx.tasks.List(repository.TaskFilter{FacilityID: f.ID, Kind: constants.TaskKindRoutine})
	if len(tasks) != 3 {
		t.Fatalf("续跑后应恰为 3 期, 实际 %d", len(tasks))
	}
	// 再次重试：0 新增。
	if n2, _ := fx.planSvc.GenerateDue(date(2026, 3, 31)); n2 != 0 {
		t.Fatalf("二次重试不应新增, 实际 %d", n2)
	}
}

// 跨季度与跨年的补齐按同一日历口径枚举。
func TestGenerateDueQuarterlyAndYearly(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "季检设施")
	if _, e := fx.planSvc.Create(f.ID, "季度巡检", constants.CycleQuarterly, date(2025, 10, 1)); e != nil {
		t.Fatalf("建季检计划: %v", e)
	}
	n, _ := fx.planSvc.GenerateDue(date(2026, 5, 1))
	if n != 3 {
		t.Fatalf("季度应补 2025-Q4,2026-Q1,2026-Q2 共 3 期, 实际 %d", n)
	}
	g := fx.createFacility(t, "年检设施")
	if _, e := fx.planSvc.Create(g.ID, "年度巡检", constants.CycleYearly, date(2023, 6, 1)); e != nil {
		t.Fatalf("建年检计划: %v", e)
	}
	ny, _ := fx.planSvc.GenerateDue(date(2026, 1, 1))
	if ny != 4 {
		t.Fatalf("年度应补 2023..2026 共 4 期, 实际 %d", ny)
	}
}

// 开始日在未来：当前无到期任务。
func TestGenerateDueFutureStartNoTask(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "未来计划电梯")
	if _, e := fx.planSvc.Create(f.ID, "月度", constants.CycleMonthly, date(2026, 12, 1)); e != nil {
		t.Fatalf("建计划: %v", e)
	}
	n, e := fx.planSvc.GenerateDue(date(2026, 9, 1))
	if e != nil {
		t.Fatalf("枚举: %v", e)
	}
	if n != 0 {
		t.Fatalf("开始日在未来不应生成任务, 实际 %d", n)
	}
}

// 无效（零值）开始日期必须明确拒绝，不得按当天处理。
func TestCreatePlanRejectsZeroStartDate(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "无效日期电梯")
	_, e := fx.planSvc.Create(f.ID, "月度", constants.CycleMonthly, time.Time{})
	if !errors.Is(e, ErrInvalidState) {
		t.Fatalf("工单关闭口径：零值开始日应返回 ErrInvalidState, 实际 %v", e)
	}
}

func periodSet(tasks []model.InspectionTask) map[string]bool {
	m := map[string]bool{}
	for _, t := range tasks {
		m[t.PeriodValue] = true
	}
	return m
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
