package main

import (
	"fmt"
	"github.com/smartestate/smartestate/internal/config"
	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/model"
	"github.com/smartestate/smartestate/internal/repository"
	"github.com/smartestate/smartestate/internal/router"
	"github.com/smartestate/smartestate/internal/service"
	"github.com/smartestate/smartestate/internal/util"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"log"
	"strings"
	"time"
)

func main() {
	cfg := config.Load()
	db, err := openDB(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err = db.AutoMigrate(&model.User{}, &model.Repair{}, &model.Payment{}, &model.Announcement{}, &model.AnnouncementRead{}, &model.OperationLog{}, &model.Role{}, &model.Permission{}, &model.RolePermission{}, &model.Facility{}, &model.InspectionPlan{}, &model.InspectionTask{}); err != nil {
		log.Fatal(err)
	}
	if err = seed(db); err != nil {
		log.Fatal(err)
	}
	logger := util.NewLogger()
	ur := repository.NewUserRepository(db)
	rr := repository.NewRepairRepository(db)
	pr := repository.NewPaymentRepository(db)
	ar := repository.NewAnnouncementRepository(db)
	lr := repository.NewOperationLogRepository(db)
	fr := repository.NewFacilityRepository(db)
	ir := repository.NewInspectionPlanRepository(db)
	tr := repository.NewInspectionTaskRepository(db)
	sv := router.Services{
		Users:           service.NewUserService(ur, logger),
		Repairs:         service.NewRepairService(db, rr, ur, tr, fr, logger),
		Payments:        service.NewPaymentService(pr, logger),
		Announcements:   service.NewAnnouncementService(ar, logger),
		Facilities:      service.NewFacilityService(fr, tr, rr, logger),
		InspectionPlans: service.NewInspectionPlanService(ir, fr, tr, logger),
		InspectionTasks: service.NewInspectionTaskService(db, tr, fr, rr, logger),
		Permissions:     service.NewPermissionService(),
		Logs:            service.NewOperationLogService(lr, logger),
	}
	// 启动时幂等重跑一次到期任务生成；可安全重复执行，不会产生重复任务。
	if _, e := sv.InspectionPlans.GenerateDue(time.Now()); e != nil {
		log.Printf("generate due inspection tasks failed: %v", e)
	}
	log.Printf("SmartEstate server listening on :%s", cfg.Port)
	if err = router.New(cfg, sv, logger).Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
func openDB(c config.Config) (*gorm.DB, error) {
	gormCfg := &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true}
	if c.DBDriver == "mysql" {
		return gorm.Open(mysql.Open(c.DSN), gormCfg)
	}
	// SQLite：启用 WAL 与忙等待，使并发写事务在本地开发下也会阻塞重试而非立即报
	// “database is locked”；生产使用 MySQL 时由行锁串行化。
	dsn := c.DSN
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	if !strings.Contains(dsn, "_busy_timeout") {
		dsn += sep + "_busy_timeout=5000"
		sep = "&"
	}
	if !strings.Contains(dsn, "_journal") {
		dsn += sep + "_journal_mode=WAL"
	}
	if !strings.Contains(dsn, "_txlock") {
		dsn += sep + "_txlock=immediate"
	}
	db, e := gorm.Open(sqlite.Open(dsn), gormCfg)
	if e != nil {
		return nil, e
	}
	if sqlDB, e := db.DB(); e == nil {
		sqlDB.SetMaxOpenConns(10)
	}
	return db, nil
}
func seed(db *gorm.DB) error {
	var n int64
	if e := db.Model(&model.User{}).Count(&n).Error; e != nil {
		return e
	}
	if n > 0 {
		return nil
	}
	hash, e := util.HashPassword("password123")
	if e != nil {
		return e
	}
	users := []model.User{{Phone: "13800000001", PasswordHash: hash, Nickname: "张业主", Role: constants.UserRoleResident, Building: "1栋", Unit: "2单元", Room: "802"}, {Phone: "13800000002", PasswordHash: hash, Nickname: "王管家", Role: constants.UserRoleStaff}, {Phone: "13800000003", PasswordHash: hash, Nickname: "系统管理员", Role: constants.UserRoleAdmin}}
	if e = db.Create(&users).Error; e != nil {
		return e
	}
	if e = db.Create(&model.Repair{UserID: users[0].ID, Title: "客厅灯具闪烁", Description: "晚间开灯时出现闪烁，请安排师傅检查。", Type: "水电", Status: constants.RepairStatusPending}).Error; e != nil {
		return e
	}
	if e = db.Create(&model.Payment{UserID: users[0].ID, FeeType: "物业费", Amount: 268.50, Month: "2026-08", Status: "unpaid"}).Error; e != nil {
		return e
	}
	if e = db.Create(&model.Announcement{Title: "夏季消防安全提醒", Content: "请勿在楼道堆放杂物，保持消防通道畅通。", Category: "紧急", PublisherID: users[1].ID, PublishAt: time.Now(), Top: true}).Error; e != nil {
		return e
	}
	if e = seedInspections(db, users); e != nil {
		return e
	}
	for _, p := range []model.Permission{{Code: "repair:manage", Name: "报修管理"}, {Code: "payment:manage", Name: "收费管理"}, {Code: "announcement:publish", Name: "公告发布"}, {Code: "log:read", Name: "日志查看"}, {Code: "inspection:manage", Name: "设施巡检管理"}} {
		if e = db.Create(&p).Error; e != nil {
			return e
		}
	}
	fmt.Print("")
	return nil
}

// seedInspections 写入巡检模块演示数据：可用设施（待巡检任务）与停用设施（隐患工单处置中）。
func seedInspections(db *gorm.DB, users []model.User) error {
	first := model.Facility{Name: "1号楼电梯", Category: "电梯", Location: "1号楼1单元", Status: constants.FacilityStatusAvailable}
	second := model.Facility{Name: "地下车库消防泵房", Category: "消防", Location: "地下一层B区", Status: constants.FacilityStatusDisabled, Remark: "巡检发现压力表异常，已停用待修"}
	if e := db.Create(&first).Error; e != nil {
		return e
	}
	if e := db.Create(&second).Error; e != nil {
		return e
	}
	now := time.Now()
	monthKey := now.Format("2006-01")
	plans := []model.InspectionPlan{
		{FacilityID: first.ID, Name: "电梯月度巡检", Cycle: constants.CycleMonthly, StartDate: now, Active: true},
		{FacilityID: second.ID, Name: "消防泵房月度巡检", Cycle: constants.CycleMonthly, StartDate: now, Active: true},
	}
	if e := db.Create(&plans).Error; e != nil {
		return e
	}
	routine := model.InspectionTask{FacilityID: first.ID, PlanID: &plans[0].ID, Kind: constants.TaskKindRoutine, Cycle: constants.CycleMonthly, PeriodValue: monthKey, DueDate: now, Status: constants.TaskStatusPending}
	if e := db.Create(&routine).Error; e != nil {
		return e
	}
	fid := second.ID
	repair := model.Repair{UserID: users[1].ID, Title: "消防泵房压力表异常维修", Description: "巡检发现泵房压力表读数异常，存在安全隐患，需立即检修。", Type: "公共设施", Status: constants.RepairStatusProcessing, HandlerID: &users[1].ID, FacilityID: &fid}
	if e := db.Create(&repair).Error; e != nil {
		return e
	}
	rid := repair.ID
	hazard := model.InspectionTask{FacilityID: second.ID, PlanID: &plans[1].ID, Kind: constants.TaskKindRoutine, Cycle: constants.CycleMonthly, PeriodValue: monthKey, DueDate: now, Status: constants.TaskStatusHazard, Result: constants.ResultHazard, Finding: "压力表读数低于安全阈值", InspectorID: &users[1].ID, ReviewedAt: &now, HazardRepairID: &rid}
	if e := db.Create(&hazard).Error; e != nil {
		return e
	}
	tid := hazard.ID
	return db.Model(&model.Repair{}).Where("id = ?", rid).Update("source_task_id", tid).Error
}
