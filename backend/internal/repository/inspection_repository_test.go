package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTaskTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if e != nil {
		t.Fatalf("open: %v", e)
	}
	if sqlDB, e := db.DB(); e == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if e = db.AutoMigrate(&model.User{}, &model.Facility{}, &model.InspectionPlan{},
		&model.InspectionTask{}, &model.Repair{}); e != nil {
		t.Fatalf("migrate: %v", e)
	}
	return db
}

// 同设施/周期/期次/种类只能有一条任务：第二条插入必须报 ErrDuplicate。
func TestTaskSlotUniqueConstraint(t *testing.T) {
	db := newTaskTestDB(t)
	fac := model.Facility{Name: "电梯", Status: constants.FacilityStatusAvailable}
	db.Create(&fac)
	repo := NewInspectionTaskRepository(db)

	task := func(kind, period string) model.InspectionTask {
		return model.InspectionTask{FacilityID: fac.ID, Kind: kind, Cycle: "monthly",
			PeriodValue: period, DueDate: time.Now(), Status: constants.TaskStatusPending}
	}
	mk1 := task("routine", "2026-09")
	if e := repo.Create(&mk1); e != nil {
		t.Fatalf("first create: %v", e)
	}
	mkDup := task("routine", "2026-09")
	if e := repo.Create(&mkDup); !errors.Is(e, ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", e)
	}
	// 常规与复检槽位互不冲突。
	mk2 := task("recheck", "recheck-1")
	if e := repo.Create(&mk2); e != nil {
		t.Fatalf("recheck slot should be distinct: %v", e)
	}
}

// 接单条件更新：第一次成功，第二次因状态已变而失败。
func TestClaimCAS(t *testing.T) {
	db := newTaskTestDB(t)
	fac := model.Facility{Name: "水泵", Status: constants.FacilityStatusAvailable}
	db.Create(&fac)
	repo := NewInspectionTaskRepository(db)
	task := model.InspectionTask{FacilityID: fac.ID, Kind: "routine", Cycle: "monthly",
		PeriodValue: "2026-09", DueDate: time.Now(), Status: constants.TaskStatusPending}
	db.Create(&task)

	ok, e := repo.Claim(task.ID, 7, constants.TaskStatusPending)
	if e != nil || !ok {
		t.Fatalf("first claim ok=%v err=%v", ok, e)
	}
	got, _ := repo.ByID(task.ID)
	if got.Status != constants.TaskStatusClaimed || got.InspectorID == nil || *got.InspectorID != 7 {
		t.Fatalf("claim not persisted: %+v", got)
	}
	ok, e = repo.Claim(task.ID, 8, constants.TaskStatusPending)
	if e != nil || ok {
		t.Fatalf("second claim must fail, ok=%v err=%v", ok, e)
	}
}

// 一张巡检任务只能关联一张维修工单（source_task_id 唯一）。
func TestRepairSourceTaskUnique(t *testing.T) {
	db := newTaskTestDB(t)
	u := model.User{Phone: "1", Nickname: "u", Role: "staff"}
	db.Create(&u)
	fac := model.Facility{Name: "消防栓", Status: constants.FacilityStatusDisabled}
	db.Create(&fac)
	tid := uint(1)
	repo := NewRepairRepository(db)
	mk := func() *model.Repair {
		fid := fac.ID
		return &model.Repair{UserID: u.ID, Title: "维修", Type: "公共设施",
			Status: constants.RepairStatusPending, FacilityID: &fid, SourceTaskID: &tid}
	}
	if e := repo.Create(mk()); e != nil {
		t.Fatalf("first repair: %v", e)
	}
	e := db.Transaction(func(tx *gorm.DB) error {
		return repo.CreateInTx(tx, mk())
	})
	if !errors.Is(e, ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got %v", e)
	}
}
