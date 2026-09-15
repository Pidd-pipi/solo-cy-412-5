package service

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/model"
	"github.com/smartestate/smartestate/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type inspectionFixture struct {
	db      *gorm.DB
	facs    *repository.FacilityRepository
	plans   *repository.InspectionPlanRepository
	tasks   *repository.InspectionTaskRepository
	repairs *repository.RepairRepository
	users   *repository.UserRepository

	facSvc  *FacilityService
	planSvc *InspectionPlanService
	taskSvc *InspectionTaskService
	repSvc  *RepairService

	staffID uint
	otherID uint
}

func newInspectionFixture(t *testing.T) *inspectionFixture {
	t.Helper()
	// 单一连接：内存 SQLite 各连接不共享数据，且单连接写锁天然串行化。
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if e != nil {
		t.Fatalf("open db: %v", e)
	}
	if sqlDB, e := db.DB(); e == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if e = db.AutoMigrate(&model.User{}, &model.Facility{}, &model.InspectionPlan{},
		&model.InspectionTask{}, &model.Repair{}); e != nil {
		t.Fatalf("migrate: %v", e)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	users := repository.NewUserRepository(db)
	facs := repository.NewFacilityRepository(db)
	plans := repository.NewInspectionPlanRepository(db)
	tasks := repository.NewInspectionTaskRepository(db)
	repairs := repository.NewRepairRepository(db)

	staff := model.User{Phone: "100", Nickname: "巡检员", Role: constants.UserRoleStaff}
	other := model.User{Phone: "101", Nickname: "巡检员乙", Role: constants.UserRoleStaff}
	if e = db.Create(&staff).Error; e != nil {
		t.Fatalf("create staff: %v", e)
	}
	if e = db.Create(&other).Error; e != nil {
		t.Fatalf("create other: %v", e)
	}

	fx := &inspectionFixture{
		db: db, facs: facs, plans: plans, tasks: tasks, repairs: repairs, users: users,
		staffID: staff.ID, otherID: other.ID,
	}
	fx.facSvc = NewFacilityService(facs, tasks, repairs, logger)
	fx.planSvc = NewInspectionPlanService(plans, facs, tasks, logger)
	fx.taskSvc = NewInspectionTaskService(db, tasks, facs, repairs, logger)
	fx.repSvc = NewRepairService(db, repairs, users, tasks, facs, logger)
	return fx
}

func (fx *inspectionFixture) createFacility(t *testing.T, name string) model.Facility {
	t.Helper()
	f := model.Facility{Name: name, Category: "消防", Location: "测试位置", Status: constants.FacilityStatusAvailable}
	if e := fx.db.Create(&f).Error; e != nil {
		t.Fatalf("create facility: %v", e)
	}
	return f
}

func (fx *inspectionFixture) createPlanAndTask(t *testing.T, facilityID uint, cycle string) (model.InspectionPlan, model.InspectionTask) {
	t.Helper()
	plan := model.InspectionPlan{FacilityID: facilityID, Name: "计划", Cycle: cycle, StartDate: time.Now(), Active: true}
	if e := fx.db.Create(&plan).Error; e != nil {
		t.Fatalf("create plan: %v", e)
	}
	task := model.InspectionTask{FacilityID: facilityID, PlanID: &plan.ID, Kind: constants.TaskKindRoutine,
		Cycle: cycle, PeriodValue: periodKey(cycle, time.Now()), DueDate: time.Now(), Status: constants.TaskStatusPending}
	if e := fx.db.Create(&task).Error; e != nil {
		t.Fatalf("create task: %v", e)
	}
	return plan, task
}
