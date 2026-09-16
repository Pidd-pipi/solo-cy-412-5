package service

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/model"
	"github.com/smartestate/smartestate/internal/repository"
)

// 同一设施同一周期只能有一项计划：重复创建被拒，且数据库唯一索引兜底。
func TestInspectionPlanUniquePerFacilityCycle(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "电梯")
	if _, e := fx.planSvc.Create(f.ID, "月度巡检", constants.CycleMonthly, time.Now()); e != nil {
		t.Fatalf("first create: %v", e)
	}
	if _, e := fx.planSvc.Create(f.ID, "再次月度", constants.CycleMonthly, time.Now()); !errors.Is(e, ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", e)
	}
	if _, e := fx.planSvc.Create(f.ID, "周巡检", constants.CycleWeekly, time.Now()); e != nil {
		t.Fatalf("different cycle should be allowed: %v", e)
	}
}

// 计划重跑不能生成重复任务：连续重跑，每期仅一条。
func TestGenerateDueIdempotent(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "配电房")
	if _, e := fx.planSvc.Create(f.ID, "月度巡检", constants.CycleMonthly, time.Now()); e != nil {
		t.Fatalf("create plan: %v", e)
	}
	for _, run := range []int{1, 2, 3} {
		n, e := fx.planSvc.GenerateDue(time.Now())
		if e != nil {
			t.Fatalf("run %d: %v", run, e)
		}
		if run == 1 && n != 1 {
			t.Fatalf("first run should generate 1, got %d", n)
		}
		if run > 1 && n != 0 {
			t.Fatalf("rerun %d should generate 0, got %d", run, n)
		}
	}
	tasks, _ := fx.tasks.List(repository.TaskFilter{FacilityID: f.ID})
	if len(tasks) != 1 {
		t.Fatalf("want exactly 1 task, got %d", len(tasks))
	}
}

// 多人同时接单：只有一个人成功。
func TestClaimConcurrentOnlyOneWins(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "水泵房")
	_, task := fx.createPlanAndTask(t, f.ID, constants.CycleMonthly)

	const n = 8
	var wg sync.WaitGroup
	results := make([]bool, n)
	errs := make([]error, n)
	inspectors := []uint{fx.staffID, fx.otherID}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = fx.taskSvc.Claim(task.ID, inspectors[idx%2], constants.UserRoleStaff)
			results[idx] = errs[idx] == nil
		}(i)
	}
	wg.Wait()
	wins := 0
	for _, ok := range results {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("want exactly 1 successful claim, got %d (errs=%v)", wins, errs)
	}
	got, _ := fx.tasks.ByID(task.ID)
	if got.Status != constants.TaskStatusClaimed {
		t.Fatalf("want claimed, got %s", got.Status)
	}
}

// 重复提交隐患：只停用一次、只生成一张维修工单。
func TestHazardSubmitIdempotentSingleRepair(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "消防栓")
	_, task := fx.createPlanAndTask(t, f.ID, constants.CycleMonthly)

	first, e := fx.taskSvc.SubmitRoutine(task.ID, fx.staffID, constants.ResultHazard, "阀门锈蚀", constants.UserRoleStaff)
	if e != nil {
		t.Fatalf("first submit: %v", e)
	}
	if first.Status != constants.TaskStatusHazard || first.HazardRepairID == nil {
		t.Fatalf("hazard not recorded correctly: %+v", first)
	}
	// 终态再提交被拒，不能改写已完成记录。
	if _, e = fx.taskSvc.SubmitRoutine(task.ID, fx.staffID, constants.ResultNormal, "", constants.UserRoleStaff); !errors.Is(e, ErrImmutable) {
		t.Fatalf("want ErrImmutable on resubmit, got %v", e)
	}
	fac, _ := fx.facs.ByID(f.ID)
	if fac.Status != constants.FacilityStatusDisabled {
		t.Fatalf("facility should be disabled")
	}
	repairs, _ := fx.repairs.ListByFacility(f.ID)
	if len(repairs) != 1 {
		t.Fatalf("want exactly 1 repair, got %d", len(repairs))
	}
}

// 正常巡检不会停用设施，也不生成工单。
func TestNormalSubmitKeepsAvailable(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "健身器材")
	_, task := fx.createPlanAndTask(t, f.ID, constants.CycleMonthly)
	got, e := fx.taskSvc.SubmitRoutine(task.ID, fx.staffID, constants.ResultNormal, "", constants.UserRoleStaff)
	if e != nil {
		t.Fatalf("submit: %v", e)
	}
	if got.Status != constants.TaskStatusDone {
		t.Fatalf("want done, got %s", got.Status)
	}
	fac, _ := fx.facs.ByID(f.ID)
	if fac.Status != constants.FacilityStatusAvailable {
		t.Fatalf("facility should stay available")
	}
	repairs, _ := fx.repairs.ListByFacility(f.ID)
	if len(repairs) != 0 {
		t.Fatalf("normal inspection should create no repair, got %d", len(repairs))
	}
}

// 隐患缺少隐患描述时拒绝。
func TestHazardRequiresFinding(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "应急照明")
	_, task := fx.createPlanAndTask(t, f.ID, constants.CycleMonthly)
	if _, e := fx.taskSvc.SubmitRoutine(task.ID, fx.staffID, constants.ResultHazard, "  ", constants.UserRoleStaff); !errors.Is(e, ErrInvalidState) {
		t.Fatalf("want ErrInvalidState, got %v", e)
	}
}

// 完整闭环：隐患停用→唯一工单→完成→复检→通过恢复。
func TestFullLifecycleHazardRecheckPassRestore(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "电梯")
	_, routine := fx.createPlanAndTask(t, f.ID, constants.CycleMonthly)

	haz, e := fx.taskSvc.SubmitRoutine(routine.ID, fx.staffID, constants.ResultHazard, "异响严重", constants.UserRoleStaff)
	if e != nil {
		t.Fatalf("hazard: %v", e)
	}
	repairID := *haz.HazardRepairID

	// 重复完成维修不重复安排复检。
	if _, e = fx.repSvc.UpdateStatus(repairID, constants.RepairStatusDone, 0, constants.UserRoleStaff); e != nil {
		t.Fatalf("complete repair: %v", e)
	}
	if _, e = fx.repSvc.UpdateStatus(repairID, constants.RepairStatusDone, 0, constants.UserRoleStaff); e != nil {
		t.Fatalf("idempotent complete: %v", e)
	}
	rechecks, _ := fx.tasks.List(repository.TaskFilter{FacilityID: f.ID, Kind: constants.TaskKindRecheck})
	if len(rechecks) != 1 {
		t.Fatalf("want exactly 1 recheck task, got %d", len(rechecks))
	}
	// 复检通过前设施仍停用。
	fac, _ := fx.facs.ByID(f.ID)
	if fac.Status != constants.FacilityStatusDisabled {
		t.Fatalf("facility should remain disabled before recheck pass")
	}

	rc := rechecks[0]
	if _, e = fx.taskSvc.Claim(rc.ID, fx.staffID, constants.UserRoleStaff); e != nil {
		t.Fatalf("claim recheck: %v", e)
	}
	restored, e := fx.taskSvc.SubmitRecheck(rc.ID, fx.staffID, constants.ResultPass, "已恢复正常", constants.UserRoleStaff)
	if e != nil {
		t.Fatalf("recheck pass: %v", e)
	}
	if restored.Status != constants.TaskStatusRestored {
		t.Fatalf("want restored, got %s", restored.Status)
	}
	fac, _ = fx.facs.ByID(f.ID)
	if fac.Status != constants.FacilityStatusAvailable {
		t.Fatalf("facility should be available after recheck pass")
	}
	// 复检记录终态不可改写。
	if _, e = fx.taskSvc.SubmitRecheck(rc.ID, fx.staffID, constants.ResultFail, "x", constants.UserRoleStaff); !errors.Is(e, ErrImmutable) {
		t.Fatalf("want ErrImmutable mutating finalized recheck, got %v", e)
	}
}

// 复检未通过：不能恢复，设施保持停用并续建维修工单；修好后再复检通过才恢复。
func TestRecheckFailKeepsDisabledThenRecover(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "电梯2")
	_, routine := fx.createPlanAndTask(t, f.ID, constants.CycleMonthly)
	haz, _ := fx.taskSvc.SubmitRoutine(routine.ID, fx.staffID, constants.ResultHazard, "钢丝绳磨损", constants.UserRoleStaff)

	_, _ = fx.repSvc.UpdateStatus(*haz.HazardRepairID, constants.RepairStatusDone, 0, constants.UserRoleStaff)
	firstRechecks, _ := fx.tasks.List(repository.TaskFilter{FacilityID: f.ID, Kind: constants.TaskKindRecheck})
	if len(firstRechecks) != 1 {
		t.Fatalf("want 1 recheck, got %d", len(firstRechecks))
	}
	rc1 := firstRechecks[0]
	res, e := fx.taskSvc.SubmitRecheck(rc1.ID, fx.staffID, constants.ResultFail, "仍有异响", constants.UserRoleStaff)
	if e != nil {
		t.Fatalf("recheck fail: %v", e)
	}
	if res.Status != constants.TaskStatusRecheckFailed {
		t.Fatalf("want recheck_failed, got %s", res.Status)
	}
	fac, _ := fx.facs.ByID(f.ID)
	if fac.Status != constants.FacilityStatusDisabled {
		t.Fatalf("facility must remain disabled on failed recheck")
	}
	// 续建了一张新维修工单（隐患单 + 续修单 = 2）。
	repairs, _ := fx.repairs.ListByFacility(f.ID)
	if len(repairs) != 2 {
		t.Fatalf("want 2 repairs after follow-up, got %d", len(repairs))
	}

	// 完成续修单 → 安排第二次复检。
	followup := repairs[0] // 最新在前
	if _, e = fx.repSvc.UpdateStatus(followup.ID, constants.RepairStatusDone, 0, constants.UserRoleStaff); e != nil {
		t.Fatalf("complete followup repair: %v", e)
	}
	allRechecks, _ := fx.tasks.List(repository.TaskFilter{FacilityID: f.ID, Kind: constants.TaskKindRecheck})
	if len(allRechecks) != 2 {
		t.Fatalf("want 2 recheck tasks, got %d", len(allRechecks))
	}
	var rc2 model.InspectionTask
	for _, rc := range allRechecks {
		if rc.Status == constants.TaskStatusPending {
			rc2 = rc
		}
	}
	if rc2.ID == 0 {
		t.Fatalf("second pending recheck not found")
	}
	if _, e = fx.taskSvc.SubmitRecheck(rc2.ID, fx.staffID, constants.ResultPass, "复检通过", constants.UserRoleStaff); e != nil {
		t.Fatalf("second recheck pass: %v", e)
	}
	fac, _ = fx.facs.ByID(f.ID)
	if fac.Status != constants.FacilityStatusAvailable {
		t.Fatalf("facility should recover after the passing recheck")
	}
}

// 工作台计数：待巡检、停用设施、未闭环关联工单均为实时 COUNT，重跑不重复累计。
func TestDashboardCountsAreConsistent(t *testing.T) {
	fx := newInspectionFixture(t)
	f1 := fx.createFacility(t, "设施甲")
	f2 := fx.createFacility(t, "设施乙")
	fx.createPlanAndTask(t, f1.ID, constants.CycleMonthly)
	_, t2 := fx.createPlanAndTask(t, f2.ID, constants.CycleMonthly)

	due, _ := fx.taskSvc.DueCount()
	if due != 2 {
		t.Fatalf("want 2 due, got %d", due)
	}
	// f2 发现隐患停用并产生一张未闭环工单。
	haz, _ := fx.taskSvc.SubmitRoutine(t2.ID, fx.staffID, constants.ResultHazard, "隐患", constants.UserRoleStaff)
	_ = haz
	due, _ = fx.taskSvc.DueCount()
	if due != 1 { // t1 仍待巡检；hazard 为终态不计入
		t.Fatalf("want 1 due after hazard, got %d", due)
	}
	disabled, _ := fx.facSvc.DisabledCount()
	if disabled != 1 {
		t.Fatalf("want 1 disabled, got %d", disabled)
	}
	unclosed, _ := fx.repSvc.UnclosedFacilityCount()
	if unclosed != 1 {
		t.Fatalf("want 1 unclosed facility repair, got %d", unclosed)
	}
	// 完成维修 → 生成复检（待处理）。维修已 done 但复检未做，设施仍停用，
	// 因此“未闭环”按 1 张待复检计（与停用设施保持同步）。
	repairs, _ := fx.repairs.ListByFacility(f2.ID)
	_, _ = fx.repSvc.UpdateStatus(repairs[0].ID, constants.RepairStatusDone, 0, constants.UserRoleStaff)
	due, _ = fx.taskSvc.DueCount()
	if due != 2 { // t1 + 新复检
		t.Fatalf("want 2 due (routine + recheck), got %d", due)
	}
	unclosed, _ = fx.repSvc.UnclosedFacilityCount()
	if unclosed != 1 {
		t.Fatalf("want 1 unclosed (pending recheck), got %d", unclosed)
	}
	// 复检通过且为唯一隐患链 → 恢复，未闭环归零。
	rechecks, _ := fx.tasks.List(repository.TaskFilter{FacilityID: f2.ID, Kind: constants.TaskKindRecheck})
	if _, e := fx.taskSvc.SubmitRecheck(rechecks[0].ID, fx.staffID, constants.ResultPass, "ok", constants.UserRoleStaff); e != nil {
		t.Fatalf("recheck pass: %v", e)
	}
	unclosed, _ = fx.repSvc.UnclosedFacilityCount()
	if unclosed != 0 {
		t.Fatalf("want 0 unclosed after recheck pass, got %d", unclosed)
	}
	disabled, _ = fx.facSvc.DisabledCount()
	if disabled != 0 {
		t.Fatalf("want 0 disabled after full closure, got %d", disabled)
	}
	// 重跑计划生成不影响计数。
	if _, e := fx.planSvc.GenerateDue(time.Now()); e != nil {
		t.Fatalf("rerun: %v", e)
	}
}

// 设施详情聚合状态、任务与处置工单。
func TestFacilityDetailAggregates(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "监控室")
	_, task := fx.createPlanAndTask(t, f.ID, constants.CycleMonthly)
	_, _ = fx.taskSvc.SubmitRoutine(task.ID, fx.staffID, constants.ResultHazard, "线路老化", constants.UserRoleStaff)

	d, e := fx.facSvc.Detail(f.ID)
	if e != nil {
		t.Fatalf("detail: %v", e)
	}
	if d.Status != constants.FacilityStatusDisabled {
		t.Fatalf("detail status wrong: %s", d.Status)
	}
	if len(d.Tasks) != 1 || len(d.Repairs) != 1 {
		t.Fatalf("want 1 task and 1 repair, got tasks=%d repairs=%d", len(d.Tasks), len(d.Repairs))
	}
	if _, e = fx.facSvc.Detail(999999); !errors.Is(e, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", e)
	}
}

// 非接单巡检人不能提交已被他人接单的任务。
func TestClaimedTaskOwnedByClaimant(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "门禁")
	_, task := fx.createPlanAndTask(t, f.ID, constants.CycleWeekly)
	if _, e := fx.taskSvc.Claim(task.ID, fx.staffID, constants.UserRoleStaff); e != nil {
		t.Fatalf("claim: %v", e)
	}
	if _, e := fx.taskSvc.SubmitRoutine(task.ID, fx.otherID, constants.ResultNormal, "", constants.UserRoleStaff); !errors.Is(e, ErrForbidden) {
		t.Fatalf("want ErrForbidden for non-owner, got %v", e)
	}
}
