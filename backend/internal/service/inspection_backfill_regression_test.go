package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/model"
	"github.com/smartestate/smartestate/internal/repository"
)

// 本文件只补“巡检计划补生成”的集成测试，不改变任何业务行为。
// 持久化与事务链路全部真实执行：复用 newIntegrationHarness —— 文件型 SQLite（t.TempDir）、
// 连接池 8 连接、WAL + busy_timeout + txlock=immediate；不使用内存替身、模拟对象、单连接；
// 并发场景用真实 goroutine + start-gate 同时放行，不做串行请求。
//
// 失败信息按阶段标注：[日期解析] / [期次枚举] / [任务生成] / [状态回读] / [工作台计数]。
// 每个用例都核对：本次新增数量、任务期次键、到期日（due_date）以及工作台待巡检计数。

// routinePeriods 回读某设施常规任务，按 period_value 索引。
func routinePeriods(h *integrationHarness, fid uint) map[string]model.InspectionTask {
	list, e := h.tasks.List(repository.TaskFilter{FacilityID: fid, Kind: constants.TaskKindRoutine})
	if e != nil {
		h.t.Fatalf("[状态回读] 读取设施 %d 常规任务失败: %v", fid, e)
	}
	m := make(map[string]model.InspectionTask, len(list))
	for _, tk := range list {
		m[tk.PeriodValue] = tk
	}
	return m
}

func assertPlanCreatedCount(t *testing.T, phase string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("[任务生成] %s：本次新增数量=%d，期望 %d", phase, got, want)
	}
}

// 场景 1：从起始期到当前期的全部期次一次性补齐（跨月），逐期核对键与到期日。
func TestBackfill_AllPeriodsFromStartToCurrent(t *testing.T) {
	h := newIntegrationHarness(t)
	f := h.createFacility("全期补齐电梯")
	plan, e := h.planSvc.Create(f.ID, "月度巡检", constants.CycleMonthly, date(2026, 1, 15))
	if e != nil {
		t.Fatalf("[日期解析] 合法开始日期建计划被拒: %v", e)
	}
	now := date(2026, 5, 10)

	n, e := h.planSvc.GenerateDue(now)
	if e != nil {
		t.Fatalf("[期次枚举] 跨期补齐返回错误: %v", e)
	}
	assertPlanCreatedCount(t, "起始期到当前期", n, 5)

	want := []struct {
		key string
		due time.Time // 首期到期=开始日；其余为该月 1 号
	}{
		{"2026-01", date(2026, 1, 15)},
		{"2026-02", date(2026, 2, 1)},
		{"2026-03", date(2026, 3, 1)},
		{"2026-04", date(2026, 4, 1)},
		{"2026-05", date(2026, 5, 1)},
	}
	got := routinePeriods(h, f.ID)
	if len(got) != len(want) {
		t.Fatalf("[状态回读] 任务键总数=%d，期望 %d（%v）", len(got), len(want), periodKeysOf(got))
	}
	for _, w := range want {
		tk, ok := got[w.key]
		if !ok {
			t.Fatalf("[状态回读] 缺少期次键 %s，实际 %v", w.key, periodKeysOf(got))
		}
		if !tk.DueDate.Equal(w.due) {
			t.Fatalf("[状态回读] 期次 %s 到期日=%v，期望 %v", w.key, tk.DueDate, w.due)
		}
		if tk.Status != constants.TaskStatusPending {
			t.Fatalf("[状态回读] 期次 %s 状态=%s，期望 pending", w.key, tk.Status)
		}
		if tk.PlanID == nil || *tk.PlanID != plan.ID {
			t.Fatalf("[状态回读] 期次 %s 未正确关联计划 %d", w.key, plan.ID)
		}
	}
	// 工作台：5 期到期日均 <= now，全部计入待巡检。
	due, _ := h.tasks.CountDue(now)
	if due != 5 {
		t.Fatalf("[工作台计数] 待巡检=%d，期望 5", due)
	}
}

// 场景 2：当前期次内，计划开始日尚未到达 —— 仍生成当前期一条，但其到期日是“未来的开始日”，
// 因而不计入待巡检；到达开始日后才计入。
func TestBackfill_StartDayWithinCurrentPeriodNotYetReached(t *testing.T) {
	h := newIntegrationHarness(t)
	f := h.createFacility("月中生效电梯")
	// 计划在当前月（2026-09）的 20 号生效，重跑基准时间为 16 号：开始日尚未到达。
	if _, e := h.planSvc.Create(f.ID, "月度巡检", constants.CycleMonthly, date(2026, 9, 20)); e != nil {
		t.Fatalf("[日期解析] 合法未来开始日（当月）建计划被拒: %v", e)
	}
	before := date(2026, 9, 16)

	n, e := h.planSvc.GenerateDue(before)
	if e != nil {
		t.Fatalf("[期次枚举] 当前期补齐返回错误: %v", e)
	}
	assertPlanCreatedCount(t, "当前期开始日未到", n, 1)

	got := routinePeriods(h, f.ID)
	tk, ok := got["2026-09"]
	if !ok {
		t.Fatalf("[状态回读] 应生成当前期 2026-09，实际键 %v", periodKeysOf(got))
	}
	if !tk.DueDate.Equal(date(2026, 9, 20)) {
		t.Fatalf("[状态回读] 当前期到期日=%v，期望为未到达的开始日 2026-09-20", tk.DueDate)
	}
	// 开始日未到：不计入待巡检。
	if due, _ := h.tasks.CountDue(before); due != 0 {
		t.Fatalf("[工作台计数] 开始日未到时待巡检应为 0，实际 %d", due)
	}
	// 到达开始日后：同一任务计入待巡检，且不重复生成。
	after := date(2026, 9, 20)
	if due, _ := h.tasks.CountDue(after); due != 1 {
		t.Fatalf("[工作台计数] 开始日到达后待巡检应为 1，实际 %d", due)
	}
	if n2, _ := h.planSvc.GenerateDue(after); n2 != 0 {
		t.Fatalf("[任务生成] 开始日到达后重跑不应新增，实际 %d", n2)
	}
}

// 场景 3：未来开始期 —— 开始日所在期次晚于当前期，不生成任何任务。
func TestBackfill_FutureStartPeriodGeneratesNothing(t *testing.T) {
	h := newIntegrationHarness(t)
	f := h.createFacility("未来季度设施")
	if _, e := h.planSvc.Create(f.ID, "季度巡检", constants.CycleQuarterly, date(2026, 12, 1)); e != nil {
		t.Fatalf("[日期解析] 合法未来开始日建计划被拒: %v", e)
	}
	n, e := h.planSvc.GenerateDue(date(2026, 9, 16))
	if e != nil {
		t.Fatalf("[期次枚举] 未来开始期不应报错: %v", e)
	}
	assertPlanCreatedCount(t, "未来开始期", n, 0)
	if len(routinePeriods(h, f.ID)) != 0 {
		t.Fatalf("[状态回读] 未来开始期不应存在任务，实际 %v", periodKeysOf(routinePeriods(h, f.ID)))
	}
	if due, _ := h.tasks.CountDue(date(2026, 9, 16)); due != 0 {
		t.Fatalf("[工作台计数] 未来开始期待巡检应为 0，实际 %d", due)
	}

	// 进入开始季度后：只补起始期一条。
	n2, _ := h.planSvc.GenerateDue(date(2026, 12, 20))
	assertPlanCreatedCount(t, "进入开始季度", n2, 1)
	got := routinePeriods(h, f.ID)
	if _, ok := got["2026-Q4"]; !ok {
		t.Fatalf("[状态回读] 进入开始季度后应生成 2026-Q4，实际 %v", periodKeysOf(got))
	}
}

// 场景 4：无效开始日期 —— 零值开始日必须被明确拒绝（service 层校验），不落任何任务。
func TestBackfill_InvalidStartDateRejected(t *testing.T) {
	h := newIntegrationHarness(t)
	f := h.createFacility("无效日期电梯")
	if _, e := h.planSvc.Create(f.ID, "月度", constants.CycleMonthly, time.Time{}); e == nil {
		t.Fatalf("[日期解析] 零值开始日期必须被拒绝，却成功建计划")
	}
	// 拒绝后补齐不应为该设施产生任何任务。
	if _, e := h.planSvc.GenerateDue(date(2026, 9, 16)); e != nil {
		t.Fatalf("[期次枚举] 补齐其他计划时不应受无效计划影响: %v", e)
	} else if len(routinePeriods(h, f.ID)) != 0 {
		t.Fatalf("[状态回读] 无效开始日期不应生成任务，实际 %d 条", len(routinePeriods(h, f.ID)))
	}
}

// 场景 5：已有终态期次不得被改写；补齐只新增缺失期。
func TestBackfill_TerminalPeriodsAreNotRewritten(t *testing.T) {
	h := newIntegrationHarness(t)
	f := h.createFacility("终态保留电梯")
	plan := model.InspectionPlan{FacilityID: f.ID, Name: "月度", Cycle: constants.CycleMonthly,
		StartDate: date(2026, 1, 1), Active: true}
	if e := h.db.Create(&plan).Error; e != nil {
		t.Fatalf("[任务生成] 预置计划失败: %v", e)
	}
	// 预置 2026-02 为“发现隐患·停用”的终态任务，并记录其关键字段用于事后比对。
	pre := model.InspectionTask{FacilityID: f.ID, PlanID: &plan.ID, Kind: constants.TaskKindRoutine,
		Cycle: constants.CycleMonthly, PeriodValue: "2026-02", DueDate: date(2026, 2, 1),
		Status: constants.TaskStatusHazard, Result: constants.ResultHazard, Finding: "既有隐患记录"}
	if e := h.db.Create(&pre).Error; e != nil {
		t.Fatalf("[任务生成] 预置终态任务失败: %v", e)
	}

	n, e := h.planSvc.GenerateDue(date(2026, 3, 10))
	if e != nil {
		t.Fatalf("[期次枚举] 含终态期的补齐报错: %v", e)
	}
	// 只补 1 月、3 月两条；2 月已存在（终态）跳过。
	assertPlanCreatedCount(t, "终态期跳过后补缺", n, 2)

	got := routinePeriods(h, f.ID)
	if len(got) != 3 {
		t.Fatalf("[状态回读] 应恰有 3 个期次，实际 %d", len(got))
	}
	kept := got["2026-02"]
	if kept.Status != constants.TaskStatusHazard || kept.Result != constants.ResultHazard || kept.Finding != "既有隐患记录" {
		t.Fatalf("[状态回读] 终态期被改写: status=%s result=%s finding=%q",
			kept.Status, kept.Result, kept.Finding)
	}
	if kept.ID != pre.ID {
		t.Fatalf("[状态回读] 终态期记录被替换（ID 变化）")
	}
	// 工作台：终态 hazard 不计入待巡检；1、3 月两条计入。
	if due, _ := h.tasks.CountDue(date(2026, 3, 10)); due != 2 {
		t.Fatalf("[工作台计数] 待巡检应为 2（终态不计），实际 %d", due)
	}
}

// 场景 6：中途缺口续跑 —— 已成功期次保留，重试只补缺失期，不重复、不改写。
func TestBackfill_ResumeGapAfterPartialFailure(t *testing.T) {
	h := newIntegrationHarness(t)
	f := h.createFacility("续跑电梯")
	plan := model.InspectionPlan{FacilityID: f.ID, Name: "月度", Cycle: constants.CycleMonthly,
		StartDate: date(2026, 1, 1), Active: true}
	if e := h.db.Create(&plan).Error; e != nil {
		t.Fatalf("[任务生成] 预置计划失败: %v", e)
	}
	now := date(2026, 4, 15)
	put := func(period string, due time.Time, status string) {
		tk := model.InspectionTask{FacilityID: f.ID, PlanID: &plan.ID, Kind: constants.TaskKindRoutine,
			Cycle: constants.CycleMonthly, PeriodValue: period, DueDate: due, Status: status}
		if e := h.db.Create(&tk).Error; e != nil {
			t.Fatalf("[任务生成] 预置已成功期次 %s 失败: %v", period, e)
		}
	}
	// 模拟上次中途失败：1 月已完成、3/4 月已生成，唯独 2 月缺失。
	put("2026-01", date(2026, 1, 1), constants.TaskStatusDone)
	put("2026-03", date(2026, 3, 1), constants.TaskStatusPending)
	put("2026-04", date(2026, 4, 1), constants.TaskStatusPending)

	n, e := h.planSvc.GenerateDue(now)
	if e != nil {
		t.Fatalf("[期次枚举] 缺口续跑报错: %v", e)
	}
	assertPlanCreatedCount(t, "缺口续跑只补 2 月", n, 1)

	got := routinePeriods(h, f.ID)
	if len(got) != 4 {
		t.Fatalf("[状态回读] 续跑后应恰为 4 期，实际 %d", len(got))
	}
	feb, ok := got["2026-02"]
	if !ok {
		t.Fatalf("[状态回读] 缺失的 2026-02 未被补齐，实际 %v", periodKeysOf(got))
	}
	if !feb.DueDate.Equal(date(2026, 2, 1)) || feb.Status != constants.TaskStatusPending {
		t.Fatalf("[状态回读] 补齐的 2 月任务 due=%v status=%s 不符合预期", feb.DueDate, feb.Status)
	}
	if got["2026-01"].Status != constants.TaskStatusDone {
		t.Fatalf("[状态回读] 已完成的 1 月期次被改写")
	}
	// 待巡检：2/3/4 月=3，已完成的 1 月不计；不重复累计。
	if due, _ := h.tasks.CountDue(now); due != 3 {
		t.Fatalf("[工作台计数] 续跑后待巡检应为 3，实际 %d", due)
	}
	// 再续跑：0 新增。
	if n2, _ := h.planSvc.GenerateDue(now); n2 != 0 {
		t.Fatalf("[任务生成] 二次续跑不应新增，实际 %d", n2)
	}
}

// 场景 7：并发重跑 —— 多个 goroutine 同时补齐同一批缺失期次，最终每期恰有一条，
// 总新增数恰等于缺失期次数（唯一约束兜底，不产生重复任务/重复计数）。
func TestBackfill_ConcurrentRerunNoDuplicates(t *testing.T) {
	const rounds = 5
	for round := 0; round < rounds; round++ {
		h := newIntegrationHarness(t)
		f := h.createFacility(fmt.Sprintf("并发补齐设施%d", round))
		if _, e := h.planSvc.Create(f.ID, "月度巡检", constants.CycleMonthly, date(2026, 1, 10)); e != nil {
			t.Fatalf("[日期解析] 第%d轮建计划失败: %v", round+1, e)
		}
		now := date(2026, 4, 20)
		const workers = 6

		start := make(chan struct{})
		var wg sync.WaitGroup
		counts := make([]int, workers)
		errs := make([]error, workers)
		run := func(i int) {
			defer wg.Done()
			<-start // 真实同时放行，不串行
			counts[i], errs[i] = h.planSvc.GenerateDue(now)
		}
		wg.Add(workers)
		for i := 0; i < workers; i++ {
			go run(i)
		}
		close(start)
		wg.Wait()

		for i, e := range errs {
			if e != nil {
				t.Fatalf("[任务生成] 第%d轮并发 worker[%d] 报错（真实竞争未被唯一约束正确处理）: %v",
					round+1, i, e)
			}
		}
		total := 0
		for _, c := range counts {
			total += c
		}
		// 缺失期次为 1/2/3/4 月共 4 个；无论多少 worker 并发，合计只能新增 4 条。
		if total != 4 {
			t.Fatalf("[任务生成] 第%d轮并发新增合计=%d，期望恰为 4（无重复）", round+1, total)
		}
		got := routinePeriods(h, f.ID)
		if len(got) != 4 {
			t.Fatalf("[状态回读] 第%d轮最终任务数=%d，期望 4（%v）", round+1, len(got), periodKeysOf(got))
		}
		for _, key := range []string{"2026-01", "2026-02", "2026-03", "2026-04"} {
			if _, ok := got[key]; !ok {
				t.Fatalf("[状态回读] 第%d轮缺少期次 %s（%v）", round+1, key, periodKeysOf(got))
			}
		}
		if due, _ := h.tasks.CountDue(now); due != 4 {
			t.Fatalf("[工作台计数] 第%d轮待巡检应为 4，实际 %d（不得重复累计）", round+1, due)
		}
	}
}

func periodKeysOf(m map[string]model.InspectionTask) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
