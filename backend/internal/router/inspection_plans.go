package router

import (
	"github.com/gin-gonic/gin"
	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/handler"
	"github.com/smartestate/smartestate/internal/middleware"
)

func RegisterInspectionPlans(g *gin.RouterGroup, sv Services, h *handler.Handler) {
	x := handler.NewInspectionPlanHandler(sv.InspectionPlans, h)
	perm := middleware.RequirePermission(sv.Permissions, constants.PermissionInspectionManage)
	g.GET("/inspection-plans", perm, x.List)
	g.POST("/inspection-plans", perm, middleware.OperationLog(sv.Logs, "inspection.plan.create"), x.Create)
	// 到期重跑：幂等，重复调用不产生重复任务。
	g.POST("/inspection-plans/generate", perm, middleware.OperationLog(sv.Logs, "inspection.plan.generate"), x.Generate)
}
