package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/dto"
	"github.com/smartestate/smartestate/internal/service"
)

type InspectionPlanHandler struct {
	Handler
	svc *service.InspectionPlanService
}

func NewInspectionPlanHandler(s *service.InspectionPlanService, h *Handler) *InspectionPlanHandler {
	return &InspectionPlanHandler{Handler: *h, svc: s}
}

func (h *InspectionPlanHandler) List(c *gin.Context) {
	var active *bool
	if a := c.Query("active"); a == "true" {
		t := true
		active = &t
	} else if a == "false" {
		f := false
		active = &f
	}
	v, e := h.svc.List(active)
	if e != nil {
		ServiceError(c, e)
		return
	}
	OK(c, v)
}

func (h *InspectionPlanHandler) Create(c *gin.Context) {
	var r dto.CreateInspectionPlanRequest
	if !Bind(c, &r, h.Validate) {
		return
	}
	// 缺省开始日为今天；一旦显式提供则必须是合法 YYYY-MM-DD，非法值不得按当天静默处理。
	start := time.Now()
	if r.StartDate != "" {
		parsed, e := time.ParseInLocation("2006-01-02", r.StartDate, time.Local)
		if e != nil {
			Fail(c, 400, constants.CodeBadRequest, "计划开始日期不合法，格式应为 YYYY-MM-DD")
			return
		}
		start = parsed
	}
	v, e := h.svc.Create(r.FacilityID, r.Name, r.Cycle, start)
	if e != nil {
		ServiceError(c, e)
		return
	}
	OK(c, v)
}

// Generate 手动重跑到期生成；重复调用幂等，不产生重复任务。
func (h *InspectionPlanHandler) Generate(c *gin.Context) {
	n, e := h.svc.GenerateDue(time.Now())
	if e != nil {
		ServiceError(c, e)
		return
	}
	OK(c, gin.H{"generated": n})
}
