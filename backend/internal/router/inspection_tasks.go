package router

import (
	"github.com/gin-gonic/gin"
	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/handler"
	"github.com/smartestate/smartestate/internal/middleware"
)

func RegisterInspectionTasks(g *gin.RouterGroup, sv Services, h *handler.Handler) {
	x := handler.NewInspectionTaskHandler(sv.InspectionTasks, h)
	perm := middleware.RequirePermission(sv.Permissions, constants.PermissionInspectionManage)
	g.GET("/inspection-tasks", perm, x.List)
	// 接单：多人同时接单仅一人成功（条件更新）。
	g.PATCH("/inspection-tasks/:id/claim", perm, middleware.OperationLog(sv.Logs, "inspection.task.claim"), x.Claim)
	// 常规巡检提交：normal 完成；hazard 停用设施且只生成一张维修工单。
	g.POST("/inspection-tasks/:id/routine", perm, middleware.OperationLog(sv.Logs, "inspection.task.routine"), x.SubmitRoutine)
	// 复检提交：pass 恢复可用；fail 保持停用并续建维修工单。
	g.POST("/inspection-tasks/:id/recheck", perm, middleware.OperationLog(sv.Logs, "inspection.task.recheck"), x.SubmitRecheck)
}
