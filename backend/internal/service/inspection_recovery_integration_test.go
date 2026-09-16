package service

import (
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/model"
	"github.com/smartestate/smartestate/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 本文件是“设施恢复状态”的集成测试，刻意满足以下约束：
//   - 使用真实的文件型 SQLite（t.TempDir 下的 .db），不是 :memory: 替身；
//   - 多连接连接池（SetMaxOpenConns=8），不是单连接；
//   - 与生产 openDB 相同的 WAL + busy_timeout + txlock=immediate 参数，走真实事务与锁等待；
//   - 全部经过真实 repository/service 事务链路，不使用任何模拟对象；
//   - 并发场景用真实 goroutine + 同时放行（start gate），不强制串行。
//
// 每条失败信息都带有阶段前缀（维修完成 / 复检提交 / 工单关闭 / 状态回读 / 页面结果 / 工作台计数）。

type integrationHarness struct {
	t     *testing.T
	db    *gorm.DB
	dir   string
	facs  *repository.FacilityRepository
	tasks *repository.InspectionTaskRepository
	reps  *repository.RepairRepository

	facSvc  *FacilityService
	planSvc *InspectionPlanService
	taskSvc *InspectionTaskService
	repSvc  *RepairService

	staffID uint
}

func newIntegrationHarness(t *testing.T) *integrationHarness {
	t.Helper()
	dir := t.TempDir()
	// 与生产 openDB 的 SQLite 参数保持一致；文件型 + 多连接 + 忙等待，产生真实锁竞争。
	dsn := filepath.Join(dir, "inspection.db") + "?_busy_timeout=8000&_journal_mode=WAL&_txlock=immediate"
	db, e := gorm.Open(sqlite.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if e != nil {
		t.Fatalf("open real file db: %v", e)
	}
	sqlDB, e := db.DB()
	if e != nil {
		t.Fatalf("get sql db: %v", e)
	}
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(8)
	if e = db.AutoMigrate(
		&model.User{}, &model.Repair{}, &model.Payment{}, &model.Announcement{},
		&model.AnnouncementRead{}, &model.OperationLog{}, &model.Role{},
		&model.Permission{}, &model.RolePermission{},
		&model.Facility{}, &model.InspectionPlan{}, &model.InspectionTask{},
	); e != nil {
		t.Fatalf("migrate: %v", e)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	userRepo := repository.NewUserRepository(db)
	facRepo := repository.NewFacilityRepository(db)
	planRepo := repository.NewInspectionPlanRepository(db)
	taskRepo := repository.NewInspectionTaskRepository(db)
	repairRepo := repository.NewRepairRepository(db)

	staff := model.User{Phone: "200", Nickname: "巡检员", Role: constants.UserRoleStaff}
	if e = db.Create(&staff).Error; e != nil {
		t.Fatalf("create staff: %v", e)
	}

	return &integrationHarness{
		t: t, db: db, dir: dir, facs: facRepo, tasks: taskRepo, reps: repairRepo,
		facSvc:  NewFacilityService(facRepo, taskRepo, repairRepo, logger),
		planSvc: NewInspectionPlanService(planRepo, facRepo, taskRepo, logger),
		taskSvc: NewInspectionTaskService(db, taskRepo, facRepo, repairRepo, logger),
		repSvc:  NewRepairService(db, repairRepo, userRepo, taskRepo, facRepo, logger),
		staffID: staff.ID,
	}
}

func (h *integrationHarness) createFacility(name string) model.Facility {
	f := model.Facility{Name: name, Category: "电梯", Location: "一号楼", Status: constants.FacilityStatusAvailable}
	if e := h.db.Create(&f).Error; e != nil {
		h.t.Fatalf("建设施: %v", e)
	}
	return f
}

// hazardChain 走真实巡检事务：常规任务 -> 提交隐患 -> 停用 + 唯一维修单。
func (h *integrationHarness) hazardChain(phase string, fid uint, period, finding string) (model.InspectionTask, model.Repair) {
	task := model.InspectionTask{FacilityID: fid, Kind: constants.TaskKindRoutine,
		Cycle: constants.CycleMonthly, PeriodValue: period, DueDate: time.Now(), Status: constants.TaskStatusPending}
	if e := h.db.Create(&task).Error; e != nil {
		h.t.Fatalf("%s: 造常规任务: %v", phase, e)
	}
	res, e := h.taskSvc.SubmitRoutine(task.ID, h.staffID, constants.ResultHazard, finding, constants.UserRoleStaff)
	if e != nil {
		h.t.Fatalf("%s: 隐患提交: %v", phase, e)
	}
	if res.HazardRepairID == nil {
		h.t.Fatalf("%s: 隐患提交未生成关联维修单", phase)
	}
	rep, e := h.reps.ByID(*res.HazardRepairID)
	if e != nil {
		h.t.Fatalf("%s: 回读维修单: %v", phase, e)
	}
	return res, rep
}

// completeRepair 走真实“维修完成”事务，并回读该单新安排的待处理复检。
func (h *integrationHarness) completeRepair(phase string, rid uint) model.InspectionTask {
	if _, e := h.repSvc.UpdateStatus(rid, constants.RepairStatusDone, 0, constants.UserRoleStaff); e != nil {
		h.t.Fatalf("%s: 维修完成: %v", phase, e)
	}
	list, e := h.tasks.List(repository.TaskFilter{Kind: constants.TaskKindRecheck, Status: constants.TaskStatusPending})
	if e != nil {
		h.t.Fatalf("%s: 回读待复检: %v", phase, e)
	}
	for _, rc := range list {
		if rc.SourceRepairID != nil && *rc.SourceRepairID == rid {
			return rc
		}
	}
	h.t.Fatalf("%s: 维修完成后未找到来源单 %d 的待复检", phase, rid)
	return model.InspectionTask{}
}

// closeRepair 走真实“直接关闭”事务（不安排复检）。
func (h *integrationHarness) closeRepair(phase string, rid uint) {
	if _, e := h.repSvc.UpdateStatus(rid, constants.RepairStatusClosed, 0, constants.UserRoleStaff); e != nil {
		h.t.Fatalf("%s: 工单关闭: %v", phase, e)
	}
}

func (h *integrationHarness) openRepairs(phase string, fid uint) int64 {
	n, e := h.reps.CountOpenByFacilityTx(h.db, fid)
	if e != nil {
		h.t.Fatalf("%s: 统计开放工单: %v", phase, e)
	}
	return n
}
func (h *integrationHarness) openRechecks(phase string, fid uint) int64 {
	n, e := h.tasks.CountOpenRecheckByFacilityTx(h.db, fid, 0)
	if e != nil {
		h.t.Fatalf("%s: 统计待复检: %v", phase, e)
	}
	return n
}

// assertFacility 状态回读 + 页面结果（FacilityService.Detail 即设施详情页数据源）。
func (h *integrationHarness) assertFacility(phase string, fid uint, wantStatus string, wantOpenRepairs, wantOpenRechecks int64) {
	h.t.Helper()
	got, e := h.facs.ByID(fid)
	if e != nil {
		h.t.Fatalf("%s: 状态回读失败: %v", phase, e)
	}
	if got.Status != wantStatus {
		h.t.Fatalf("%s: 设施状态=%s, 期望 %s（仍有开放工单=%d 待复检=%d）",
			phase, got.Status, wantStatus, h.openRepairs(phase, fid), h.openRechecks(phase, fid))
	}
	if n := h.openRepairs(phase, fid); n != wantOpenRepairs {
		h.t.Fatalf("%s: 开放工单数=%d, 期望 %d", phase, n, wantOpenRepairs)
	}
	if n := h.openRechecks(phase, fid); n != wantOpenRechecks {
		h.t.Fatalf("%s: 待复检数=%d, 期望 %d", phase, n, wantOpenRechecks)
	}
	// 页面结果：详情页必须与库内状态/进度一致。
	page, e := h.facSvc.Detail(fid)
	if e != nil {
		h.t.Fatalf("%s: 页面结果读取失败: %v", phase, e)
	}
	if page.Status != wantStatus {
		h.t.Fatalf("%s: 页面设施状态=%s, 期望 %s", phase, page.Status, wantStatus)
	}
	var pageOpen int64
	for _, r := range page.Repairs {
		if r.Status != constants.RepairStatusDone && r.Status != constants.RepairStatusClosed {
			pageOpen++
		}
	}
	if pageOpen != wantOpenRepairs {
		h.t.Fatalf("%s: 页面开放工单数=%d, 期望 %d", phase, pageOpen, wantOpenRepairs)
	}
}

func (h *integrationHarness) assertDashboard(phase string, wantDisabled, wantUnclosed int64) {
	h.t.Helper()
	disabled, e := h.facSvc.DisabledCount()
	if e != nil {
		h.t.Fatalf("%s: 工作台停用计数: %v", phase, e)
	}
	unclosed, e := h.repSvc.UnclosedFacilityCount()
	if e != nil {
		h.t.Fatalf("%s: 工作台未闭环计数: %v", phase, e)
	}
	if disabled != wantDisabled || unclosed != wantUnclosed {
		h.t.Fatalf("%s: 工作台计数 停用设施=%d(期望%d) 未闭环=%d(期望%d)",
			phase, disabled, wantDisabled, unclosed, wantUnclosed)
	}
}

// 场景 1：两张以上隐患工单时，单条复检通过只结束该复检，设施保持停用。
func TestIntegrationSingleRecheckPassStaysDisabledWithMultipleOpen(t *testing.T) {
	h := newIntegrationHarness(t)
	f := h.createFacility("多发隐患电梯")
	_, ra := h.hazardChain("隐患A", f.ID, "2026-07", "厅门异响")
	_, rb := h.hazardChain("隐患B", f.ID, "2026-08", "钢丝绳磨损")
	_, rc := h.hazardChain("隐患C", f.ID, "2026-09", "按钮失灵")
	h.assertFacility("隐患产生后", f.ID, constants.FacilityStatusDisabled, 3, 0)

	rca := h.completeRepair("维修完成A", ra.ID)
	h.assertFacility("维修完成A（B/C 未闭环，复检A待处理）", f.ID, constants.FacilityStatusDisabled, 2, 1)

	res, e := h.taskSvc.SubmitRecheck(rca.ID, h.staffID, constants.ResultPass, "A已修复", constants.UserRoleStaff)
	if e != nil {
		t.Fatalf("复检提交A: %v", e)
	}
	if res.Status != constants.TaskStatusRecheckPassed {
		t.Fatalf("复检提交A: 任务状态=%s, 期望 recheck_passed（另有隐患未闭环）", res.Status)
	}
	// 关键：B、C 两张工单仍开放，设施必须停用。
	h.assertFacility("复检提交A通过（B/C仍开放）", f.ID, constants.FacilityStatusDisabled, 2, 0)
	h.assertDashboard("复检提交A通过后工作台", 1, 2)
	_ = rb
	_ = rc
}

// 场景 2：一张工单走复检，另一张直接关闭；最后直接关闭触发最终闭环并恢复。
func TestIntegrationDirectCloseDrivesFinalClosure(t *testing.T) {
	h := newIntegrationHarness(t)
	f := h.createFacility("关闭闭环电梯")
	_, ra := h.hazardChain("隐患A", f.ID, "2026-08", "故障A")
	_, rb := h.hazardChain("隐患B", f.ID, "2026-09", "故障B")

	rca := h.completeRepair("维修完成A", ra.ID)
	// B 仍开放时，A 复检通过但不恢复。
	if res, e := h.taskSvc.SubmitRecheck(rca.ID, h.staffID, constants.ResultPass, "ok", constants.UserRoleStaff); e != nil {
		t.Fatalf("复检提交A: %v", e)
	} else if res.Status != constants.TaskStatusRecheckPassed {
		t.Fatalf("复检提交A: 期望 recheck_passed, got %s", res.Status)
	}
	h.assertFacility("复检A通过（B仍开放）", f.ID, constants.FacilityStatusDisabled, 1, 0)

	// 直接关闭 B：无复检，但它是最后一条开放链，应在同一事务内恢复设施。
	h.closeRepair("工单关闭B", rb.ID)
	h.assertFacility("工单关闭B后最终闭环", f.ID, constants.FacilityStatusAvailable, 0, 0)
	h.assertDashboard("关闭后工作台", 0, 0)

	// 单链隐患工单被直接关闭：应立即恢复，且不产生复检。
	g := h.createFacility("直接关闭门禁")
	_, rd := h.hazardChain("隐患D", g.ID, "2026-09", "读卡器坏")
	h.closeRepair("工单关闭D", rd.ID)
	h.assertFacility("工单关闭D后立即恢复", g.ID, constants.FacilityStatusAvailable, 0, 0)
	if n := h.openRechecks("关闭D后核对", g.ID); n != 0 {
		t.Fatalf("工单关闭D后不应产生复检，实际 %d", n)
	}

	// 重复关闭幂等：不改变状态、不报错。
	h.closeRepair("重复关闭B", rb.ID)
	h.assertFacility("重复关闭B", f.ID, constants.FacilityStatusAvailable, 0, 0)
}

// 场景 3：复检未通过续修；续修单未闭环前，其他复检通过也不能恢复。
func TestIntegrationRecheckFailFollowupKeepsDisabledUntilClosed(t *testing.T) {
	h := newIntegrationHarness(t)
	f := h.createFacility("续修水泵房")
	_, ra := h.hazardChain("隐患A", f.ID, "2026-08", "压力异常")
	_, rb := h.hazardChain("隐患B", f.ID, "2026-09", "密封渗漏")

	rca := h.completeRepair("维修完成A", ra.ID)
	fail, e := h.taskSvc.SubmitRecheck(rca.ID, h.staffID, constants.ResultFail, "仍有异常", constants.UserRoleStaff)
	if e != nil {
		t.Fatalf("复检提交A(未过): %v", e)
	}
	if fail.Status != constants.TaskStatusRecheckFailed {
		t.Fatalf("复检提交A(未过): 期望 recheck_failed, got %s", fail.Status)
	}
	// 续修单已生成：开放工单为 B + A的续修单 = 2。
	h.assertFacility("复检A未过续修", f.ID, constants.FacilityStatusDisabled, 2, 0)

	// B 完成并复检通过，但 A 的续修单仍开放 -> 不恢复。
	rcb := h.completeRepair("维修完成B", rb.ID)
	passB, e := h.taskSvc.SubmitRecheck(rcb.ID, h.staffID, constants.ResultPass, "B已修复", constants.UserRoleStaff)
	if e != nil {
		t.Fatalf("复检提交B: %v", e)
	}
	if passB.Status != constants.TaskStatusRecheckPassed {
		t.Fatalf("复检提交B: 期望 recheck_passed（A续修中）, got %s", passB.Status)
	}
	h.assertFacility("复检B通过（A续修单开放）", f.ID, constants.FacilityStatusDisabled, 1, 0)

	// 找到 A 的续修单（非原始 A 工单），完成它并复检通过 -> 此时全部闭环恢复。
	repairs, e := h.reps.ListByFacility(f.ID)
	if e != nil {
		t.Fatalf("回读续修单: %v", e)
	}
	var followupID uint
	for _, r := range repairs {
		if r.ID != ra.ID && r.ID != rb.ID && r.SourceTaskID != nil {
			followupID = r.ID
		}
	}
	if followupID == 0 {
		t.Fatalf("复检未过后未找到续修工单")
	}
	rc2 := h.completeRepair("续修完成", followupID)
	pass2, e := h.taskSvc.SubmitRecheck(rc2.ID, h.staffID, constants.ResultPass, "复检通过", constants.UserRoleStaff)
	if e != nil {
		t.Fatalf("复检提交(续修): %v", e)
	}
	if pass2.Status != constants.TaskStatusRestored {
		t.Fatalf("最终复检: 期望 restored, got %s", pass2.Status)
	}
	h.assertFacility("续修复检通过最终恢复", f.ID, constants.FacilityStatusAvailable, 0, 0)
	h.assertDashboard("最终工作台", 0, 0)
}

// 场景 4：前一条复检通过恢复后，又出现新隐患：设施应再次停用，直到新链闭环。
func TestIntegrationNewHazardAfterPriorRestore(t *testing.T) {
	h := newIntegrationHarness(t)
	f := h.createFacility("二次隐患电梯")
	_, ra := h.hazardChain("隐患A", f.ID, "2026-08", "首次故障")
	rca := h.completeRepair("维修完成A", ra.ID)
	passA, e := h.taskSvc.SubmitRecheck(rca.ID, h.staffID, constants.ResultPass, "ok", constants.UserRoleStaff)
	if e != nil {
		t.Fatalf("复检提交A: %v", e)
	}
	if passA.Status != constants.TaskStatusRestored {
		t.Fatalf("首次复检: 期望 restored, got %s", passA.Status)
	}
	h.assertFacility("首次恢复", f.ID, constants.FacilityStatusAvailable, 0, 0)

	// 新隐患出现（下一期次）：必须再次停用并新建唯一工单。
	_, rb := h.hazardChain("新隐患B", f.ID, "2026-09", "新故障")
	h.assertFacility("新隐患出现后", f.ID, constants.FacilityStatusDisabled, 1, 0)
	h.assertDashboard("新隐患后工作台", 1, 1)

	// 新链完成并复检通过 -> 恢复；旧任务保持终态不被改写。
	rcb := h.completeRepair("维修完成B", rb.ID)
	passB, e := h.taskSvc.SubmitRecheck(rcb.ID, h.staffID, constants.ResultPass, "ok", constants.UserRoleStaff)
	if e != nil {
		t.Fatalf("复检提交B: %v", e)
	}
	if passB.Status != constants.TaskStatusRestored {
		t.Fatalf("二次复检: 期望 restored, got %s", passB.Status)
	}
	h.assertFacility("二次恢复", f.ID, constants.FacilityStatusAvailable, 0, 0)
}

// 场景 5：两个复检“同时”通过——真实 goroutine 并发、同时放行，只能有一个承担恢复。
// 对多个设施重复并发，确保不是偶发；全程多连接、真实事务锁等待。
func TestIntegrationConcurrentRechecksOnlyOneRestores(t *testing.T) {
	const rounds = 5
	h := newIntegrationHarness(t)
	for round := 0; round < rounds; round++ {
		f := h.createFacility("并发复检设施" + string(rune('A'+round)))
		_, ra := h.hazardChain("并发隐患A", f.ID, "2026-08", "A")
		_, rb := h.hazardChain("并发隐患B", f.ID, "2026-09", "B")
		rca := h.completeRepair("并发维修完成A", ra.ID)
		rcb := h.completeRepair("并发维修完成B", rb.ID)
		if n := h.openRechecks("并发前核对", f.ID); n != 2 {
			t.Fatalf("第%d轮: 期望 2 条待复检, got %d", round+1, n)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		results := make([]error, 2)
		statuses := make([]string, 2)
		submit := func(id uint, idx int) {
			defer wg.Done()
			<-start // 同时放行，避免人为串行
			res, e := h.taskSvc.SubmitRecheck(id, h.staffID, constants.ResultPass, "ok", constants.UserRoleStaff)
			results[idx] = e
			if e == nil {
				statuses[idx] = res.Status
			}
		}
		wg.Add(2)
		go submit(rca.ID, 0)
		go submit(rcb.ID, 1)
		close(start)
		wg.Wait()

		for i, e := range results {
			if e != nil {
				t.Fatalf("第%d轮: 并发复检提交[%d]失败（真实并发冲突未被正确处理）: %v", round+1, i, e)
			}
		}
		restored, passed := 0, 0
		for _, s := range statuses {
			switch s {
			case constants.TaskStatusRestored:
				restored++
			case constants.TaskStatusRecheckPassed:
				passed++
			default:
				t.Fatalf("第%d轮: 并发复检出现非法终态 %q", round+1, s)
			}
		}
		if restored != 1 || passed != 1 {
			t.Fatalf("第%d轮: 并发复检后应恰有 1 restored、1 recheck_passed，got restored=%d passed=%d",
				round+1, restored, passed)
		}
		h.assertFacility("并发复检后状态回读", f.ID, constants.FacilityStatusAvailable, 0, 0)
	}
	h.assertDashboard("全部并发闭环后工作台", 0, 0)
}
