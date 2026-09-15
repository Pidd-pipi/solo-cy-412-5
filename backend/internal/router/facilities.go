package router

import (
	"github.com/gin-gonic/gin"
	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/handler"
	"github.com/smartestate/smartestate/internal/middleware"
)

func RegisterFacilities(g *gin.RouterGroup, sv Services, h *handler.Handler) {
	x := handler.NewFacilityHandler(sv.Facilities, h)
	perm := middleware.RequirePermission(sv.Permissions, constants.PermissionInspectionManage)
	g.GET("/facilities", perm, x.List)
	g.GET("/facilities/:id", perm, x.Detail)
	g.POST("/facilities", perm, middleware.OperationLog(sv.Logs, "facility.create"), x.Create)
}
