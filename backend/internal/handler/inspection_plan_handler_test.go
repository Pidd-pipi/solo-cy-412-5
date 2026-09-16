package handler_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/smartestate/smartestate/internal/handler"
	"github.com/smartestate/smartestate/internal/model"
	"github.com/smartestate/smartestate/internal/repository"
	"github.com/smartestate/smartestate/internal/service"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 真实持久化的 handler 测试：文件型 SQLite + 多连接 + WAL，handler/service/repository 全真实链路。
// 仅省略鉴权中间件（日期解析在进入业务前发生），不使用任何模拟对象。
func newPlanEngine(t *testing.T) (*gin.Engine, *service.InspectionPlanService, *service.FacilityService, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := filepath.Join(t.TempDir(), "plans.db") + "?_busy_timeout=8000&_journal_mode=WAL&_txlock=immediate"
	db, e := gorm.Open(sqlite.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if e != nil {
		t.Fatalf("[日期解析] 打开真实文件库失败: %v", e)
	}
	if sqlDB, e := db.DB(); e == nil {
		sqlDB.SetMaxOpenConns(8)
		sqlDB.SetMaxIdleConns(8)
	}
	if e = db.AutoMigrate(
		&model.User{}, &model.Repair{}, &model.Facility{},
		&model.InspectionPlan{}, &model.InspectionTask{},
	); e != nil {
		t.Fatalf("[日期解析] 迁移失败: %v", e)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	facRepo := repository.NewFacilityRepository(db)
	planRepo := repository.NewInspectionPlanRepository(db)
	taskRepo := repository.NewInspectionTaskRepository(db)
	repairRepo := repository.NewRepairRepository(db)
	facSvc := service.NewFacilityService(facRepo, taskRepo, repairRepo, logger)
	planSvc := service.NewInspectionPlanService(planRepo, facRepo, taskRepo, logger)
	h := &handler.Handler{Validate: validator.New()}
	planHandler := handler.NewInspectionPlanHandler(planSvc, h)

	r := gin.New()
	g := r.Group("/api/v1")
	g.POST("/inspection-plans", planHandler.Create)
	return r, planSvc, facSvc, db
}

func postPlan(t *testing.T, r *gin.Engine, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inspection-plans", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatalf("[日期解析] 响应不是合法 JSON: %v body=%s", e, w.Body.String())
	}
	return w.Code, out
}

// 日期解析：非法日期字符串必须明确 400，不能静默按当天落库。
func TestPlanCreateDateParsing(t *testing.T) {
	r, _, facSvc, db := newPlanEngine(t)
	f, e := facSvc.Create("日期电梯", "电梯", "一号楼", "")
	if e != nil {
		t.Fatalf("[任务生成] 准备设施失败: %v", e)
	}
	validBody := func(start string) string {
		b, _ := json.Marshal(map[string]any{
			"facility_id": f.ID, "name": "月度巡检", "cycle": "monthly", "start_date": start,
		})
		return string(b)
	}

	for _, bad := range []string{"2026-13-40", "not-a-date", "2026/01/15", "2026-1-1", "31-12-2026"} {
		code, out := postPlan(t, r, validBody(bad))
		if code != http.StatusBadRequest {
			t.Fatalf("[日期解析] start_date=%q 应返回 400，实际 %d（%v）", bad, code, out)
		}
		var n int64
		if e = db.Model(&model.InspectionPlan{}).Count(&n).Error; e != nil {
			t.Fatalf("[状态回读] 统计计划失败: %v", e)
		}
		if n != 0 {
			t.Fatalf("[状态回读] 非法日期 %q 不应落库，实际计划数=%d", bad, n)
		}
	}

	// 合法日期：200，且回读的开始日期与入参一致。
	code, out := postPlan(t, r, validBody("2026-01-15"))
	if code != http.StatusOK {
		t.Fatalf("[日期解析] 合法开始日期应 200，实际 %d（%v）", code, out)
	}
	data, _ := out["data"].(map[string]any)
	if data == nil || data["start_date"] == nil {
		t.Fatalf("[状态回读] 合法计划返回缺少 start_date: %v", out)
	}
	var plan model.InspectionPlan
	if e = db.First(&plan).Error; e != nil {
		t.Fatalf("[状态回读] 合法计划未落库: %v", e)
	}
	if y, m, d := plan.StartDate.Date(); y != 2026 || m != 1 || d != 15 {
		t.Fatalf("[状态回读] 落库开始日期=%d-%d-%d，期望 2026-01-15", y, m, d)
	}

	// 缺省 start_date：200（默认今天），不报错。
	b, _ := json.Marshal(map[string]any{
		"facility_id": f.ID, "name": "周巡检", "cycle": "weekly",
	})
	if code, out = postPlan(t, r, string(b)); code != http.StatusOK {
		t.Fatalf("[日期解析] 缺省开始日期应 200（默认今天），实际 %d（%v）", code, out)
	}
}
