package service

import (
	"sync"
	"testing"

	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/repository"
)

// 同一设施两条独立隐患链：一条复检通过不能提前恢复，必须等所有隐患工单闭环。
func TestMultipleHazardsRestoreOnlyAfterAllClosed(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "电梯多发隐患")

	_, ra := fx.createHazardChain(t, f.ID, "2026-08", "厅门异响")
	_, rb := fx.createHazardChain(t, f.ID, "2026-09", "钢丝绳磨损")

	if fac, _ := fx.facs.ByID(f.ID); fac.Status != constants.FacilityStatusDisabled {
		t.Fatalf("facility should be disabled")
	}
	if n, _ := fx.repairs.CountOpenByFacilityTx(fx.db, f.ID); n != 2 {
		t.Fatalf("want 2 open repairs, got %d", n)
	}

	// 完成 A 的维修 → 安排复检 A；此时 B 仍在维修。
	rca := fx.completeRepairAndGetRecheck(t, f.ID, ra.ID)
	if n, _ := fx.repairs.CountOpenByFacilityTx(fx.db, f.ID); n != 1 {
		t.Fatalf("want 1 open repair (B), got %d", n)
	}
	if n, _ := fx.tasks.CountOpenRecheckByFacilityTx(fx.db, f.ID, 0); n != 1 {
		t.Fatalf("want 1 pending recheck, got %d", n)
	}

	// 复检 A 通过，但 B 未闭环：只结束本复检，设施必须保持停用。
	pa, e := fx.taskSvc.SubmitRecheck(rca.ID, fx.staffID, constants.ResultPass, "A 已修复", constants.UserRoleStaff)
	if e != nil {
		t.Fatalf("recheck A pass: %v", e)
	}
	if pa.Status != constants.TaskStatusRecheckPassed {
		t.Fatalf("want recheck_passed while other chain open, got %s", pa.Status)
	}
	if fac, _ := fx.facs.ByID(f.ID); fac.Status != constants.FacilityStatusDisabled {
		t.Fatalf("facility must remain disabled while chain B open, got %s", fac.Status)
	}

	// 完成 B → 复检 B 通过，此时全部闭环才恢复。
	rcb := fx.completeRepairAndGetRecheck(t, f.ID, rb.ID)
	pb, e := fx.taskSvc.SubmitRecheck(rcb.ID, fx.staffID, constants.ResultPass, "B 已修复", constants.UserRoleStaff)
	if e != nil {
		t.Fatalf("recheck B pass: %v", e)
	}
	if pb.Status != constants.TaskStatusRestored {
		t.Fatalf("want restored on final closure, got %s", pb.Status)
	}
	if fac, _ := fx.facs.ByID(f.ID); fac.Status != constants.FacilityStatusAvailable {
		t.Fatalf("facility should be available after all chains closed, got %s", fac.Status)
	}
	if n, _ := fx.facSvc.DisabledCount(); n != 0 {
		t.Fatalf("want 0 disabled, got %d", n)
	}
	if n, _ := fx.repSvc.UnclosedFacilityCount(); n != 0 {
		t.Fatalf("want 0 unclosed, got %d", n)
	}

	// 两个复检任务均为终态，不可再改写。
	for _, id := range []uint{rca.ID, rcb.ID} {
		if _, e = fx.taskSvc.SubmitRecheck(id, fx.staffID, constants.ResultFail, "x", constants.UserRoleStaff); e == nil {
			t.Fatalf("finalized recheck %d must be immutable", id)
		}
	}
}

// 复检未通过续修的链路也算未闭环：另一链复检通过仍不得恢复。
func TestRecheckFailFollowupKeepsFacilityDisabled(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "水泵房")
	_, ra := fx.createHazardChain(t, f.ID, "2026-08", "压力异常")
	_, rb := fx.createHazardChain(t, f.ID, "2026-09", "密封渗漏")

	rca := fx.completeRepairAndGetRecheck(t, f.ID, ra.ID)
	// A 复检未通过 → 续建维修单（设施仍停用）。
	if _, e := fx.taskSvc.SubmitRecheck(rca.ID, fx.staffID, constants.ResultFail, "仍有异常", constants.UserRoleStaff); e != nil {
		t.Fatalf("recheck A fail: %v", e)
	}
	// B 维修完成、复检通过，但 A 续修单仍未闭环 → 不得恢复。
	rcb := fx.completeRepairAndGetRecheck(t, f.ID, rb.ID)
	pb, e := fx.taskSvc.SubmitRecheck(rcb.ID, fx.staffID, constants.ResultPass, "B 已修复", constants.UserRoleStaff)
	if e != nil {
		t.Fatalf("recheck B pass: %v", e)
	}
	if pb.Status != constants.TaskStatusRecheckPassed {
		t.Fatalf("want recheck_passed, got %s", pb.Status)
	}
	if fac, _ := fx.facs.ByID(f.ID); fac.Status != constants.FacilityStatusDisabled {
		t.Fatalf("facility must stay disabled due to A follow-up, got %s", fac.Status)
	}
}

// 并发提交两个复检：最终设施可用，且恰有一个任务承担“恢复”（restored），另一个为 recheck_passed。
func TestConcurrentRecheckOnlyLastClosureRestores(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "配电房")
	_, ra := fx.createHazardChain(t, f.ID, "2026-08", "端子松动")
	_, rb := fx.createHazardChain(t, f.ID, "2026-09", "绝缘老化")
	rca := fx.completeRepairAndGetRecheck(t, f.ID, ra.ID)
	rcb := fx.completeRepairAndGetRecheck(t, f.ID, rb.ID)

	if n, _ := fx.tasks.CountOpenRecheckByFacilityTx(fx.db, f.ID, 0); n != 2 {
		t.Fatalf("want 2 pending rechecks, got %d", n)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	pass := func(id uint, idx int) {
		defer wg.Done()
		_, errs[idx] = fx.taskSvc.SubmitRecheck(id, fx.staffID, constants.ResultPass, "ok", constants.UserRoleStaff)
	}
	wg.Add(2)
	go pass(rca.ID, 0)
	go pass(rcb.ID, 1)
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			t.Fatalf("concurrent recheck %d error: %v", i, e)
		}
	}

	final, _ := fx.tasks.List(repository.TaskFilter{FacilityID: f.ID, Kind: constants.TaskKindRecheck})
	restored, passed := 0, 0
	for _, rc := range final {
		switch rc.Status {
		case constants.TaskStatusRestored:
			restored++
		case constants.TaskStatusRecheckPassed:
			passed++
		default:
			t.Fatalf("unexpected terminal recheck status %s", rc.Status)
		}
	}
	if restored != 1 || passed != 1 {
		t.Fatalf("want exactly 1 restored and 1 recheck_passed, got restored=%d passed=%d", restored, passed)
	}
	if fac, _ := fx.facs.ByID(f.ID); fac.Status != constants.FacilityStatusAvailable {
		t.Fatalf("facility must be available once both chains close, got %s", fac.Status)
	}
}

// 单链路复检通过仍直接恢复（保持原有行为）。
func TestSingleChainRecheckPassRestores(t *testing.T) {
	fx := newInspectionFixture(t)
	f := fx.createFacility(t, "门禁")
	_, r := fx.createHazardChain(t, f.ID, "2026-09", "读卡器故障")
	rc := fx.completeRepairAndGetRecheck(t, f.ID, r.ID)
	p, e := fx.taskSvc.SubmitRecheck(rc.ID, fx.staffID, constants.ResultPass, "ok", constants.UserRoleStaff)
	if e != nil {
		t.Fatalf("recheck pass: %v", e)
	}
	if p.Status != constants.TaskStatusRestored {
		t.Fatalf("single chain should restore directly, got %s", p.Status)
	}
	if fac, _ := fx.facs.ByID(f.ID); fac.Status != constants.FacilityStatusAvailable {
		t.Fatalf("facility should be available")
	}
}
