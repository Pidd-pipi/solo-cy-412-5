package service

import (
	"testing"

	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/model"
	"github.com/smartestate/smartestate/internal/repository"
)

// 真实文件库：服务跨 4 个月才重跑先补齐 4 期；随后模拟“上次中途失败”——
// 1 月已完成、2 月任务缺失，3/4 月已生成。续跑必须只补 2 月，已完成/已存在期次不改写、不重复计数。
func TestIntegrationBackfillResumesMissingPeriodOnRealDB(t *testing.T) {
	h := newIntegrationHarness(t)
	f := h.createFacility("跨期补齐电梯")
	plan, e := h.planSvc.Create(f.ID, "月度巡检", constants.CycleMonthly, date(2026, 1, 1))
	if e != nil {
		t.Fatalf("建计划: %v", e)
	}
	now := date(2026, 4, 15)

	n, e := h.planSvc.GenerateDue(now)
	if e != nil {
		t.Fatalf("维修完成口径首次补齐: %v", e)
	}
	if n != 4 {
		t.Fatalf("状态回读：跨 4 个月应补 4 期, 实际 %d", n)
	}

	// 模拟上一次中途失败后的现场：1 月任务已完成（终态），2 月任务缺失，3/4 月已存在。
	list, _ := h.tasks.List(repository.TaskFilter{FacilityID: f.ID, Kind: constants.TaskKindRoutine})
	byPeriod := map[string]model.InspectionTask{}
	for _, tk := range list {
		byPeriod[tk.PeriodValue] = tk
	}
	jan, ok1 := byPeriod["2026-01"]
	feb, ok2 := byPeriod["2026-02"]
	if !ok1 || !ok2 {
		t.Fatalf("状态回读：缺少 1/2 月任务 %v", byPeriod)
	}
	if e = h.db.Model(&model.InspectionTask{}).Where("id = ?", jan.ID).
		Update("status", constants.TaskStatusDone).Error; e != nil {
		t.Fatalf("工单关闭口径：置 1 月为已完成: %v", e)
	}
	if e = h.db.Delete(&model.InspectionTask{}, feb.ID).Error; e != nil {
		t.Fatalf("复检提交口径：删除 2 月任务模拟缺口: %v", e)
	}

	// 续跑：只能补缺失的 2 月这 1 期。
	n, e = h.planSvc.GenerateDue(now)
	if e != nil {
		t.Fatalf("维修完成口径续跑: %v", e)
	}
	if n != 1 {
		t.Fatalf("页面结果：续跑应只补 1 期(2月), 实际 %d", n)
	}

	after, _ := h.tasks.List(repository.TaskFilter{FacilityID: f.ID, Kind: constants.TaskKindRoutine})
	if len(after) != 4 {
		t.Fatalf("状态回读：续跑后应恰为 4 期, 实际 %d（不得重复）", len(after))
	}
	var febTask model.InspectionTask
	janKeptDone := false
	for _, tk := range after {
		switch tk.PeriodValue {
		case "2026-01":
			if tk.Status == constants.TaskStatusDone {
				janKeptDone = true
			}
		case "2026-02":
			febTask = tk
		}
	}
	if !janKeptDone {
		t.Fatalf("复检提交口径：已完成的 1 月期次被改写")
	}
	if febTask.ID == 0 || febTask.Status != constants.TaskStatusPending {
		t.Fatalf("状态回读：补齐的 2 月任务应为待巡检, got %+v", febTask)
	}

	// 待巡检数量实时统计：2/3/4 月待巡检=3，1 月已完成不计；不得因续跑重复累计。
	due, _ := h.taskSvc.DueCount()
	if due != 3 {
		t.Fatalf("工作台计数：待巡检应=3, 实际 %d", due)
	}

	// 再续跑：0 新增，幂等。
	if n2, _ := h.planSvc.GenerateDue(now); n2 != 0 {
		t.Fatalf("重复重跑不应新增, 实际 %d", n2)
	}
	_ = plan
}

// 真实文件库：开始日在未来时不补任何期；进入开始期后只补起始期。
func TestIntegrationBackfillFutureStart(t *testing.T) {
	h := newIntegrationHarness(t)
	f := h.createFacility("未来季度设施")
	if _, e := h.planSvc.Create(f.ID, "季度巡检", constants.CycleQuarterly, date(2026, 12, 1)); e != nil {
		t.Fatalf("建计划: %v", e)
	}
	if n, _ := h.planSvc.GenerateDue(date(2026, 9, 1)); n != 0 {
		t.Fatalf("状态回读：未来开始日不应补齐, 实际 %d", n)
	}
	if n, _ := h.planSvc.GenerateDue(date(2026, 12, 20)); n != 1 {
		t.Fatalf("维修完成口径：进入开始季度应只补 1 期, 实际 %d", n)
	}
}
