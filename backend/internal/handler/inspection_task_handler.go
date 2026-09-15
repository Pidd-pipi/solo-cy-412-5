package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/smartestate/smartestate/internal/dto"
	"github.com/smartestate/smartestate/internal/repository"
	"github.com/smartestate/smartestate/internal/service"
)

type InspectionTaskHandler struct {
	Handler
	svc *service.InspectionTaskService
}

func NewInspectionTaskHandler(s *service.InspectionTaskService, h *Handler) *InspectionTaskHandler {
	return &InspectionTaskHandler{Handler: *h, svc: s}
}

func (h *InspectionTaskHandler) List(c *gin.Context) {
	f := repository.TaskFilter{Status: c.Query("status"), Kind: c.Query("kind")}
	if id, e := strconv.Atoi(c.Query("facility_id")); e == nil {
		f.FacilityID = uint(id)
	}
	if v := c.Query("open"); v == "true" {
		// 待办：未进入终态
		f.StatusNotIn = []string{"done", "hazard", "recheck_failed", "restored"}
	}
	out, err := h.svc.List(f)
	if err != nil {
		ServiceError(c, err)
		return
	}
	OK(c, out)
}

func (h *InspectionTaskHandler) Claim(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	v, e := h.svc.Claim(uint(id), c.GetUint("userID"), c.GetString("role"))
	if e != nil {
		ServiceError(c, e)
		return
	}
	OK(c, v)
}

func (h *InspectionTaskHandler) SubmitRoutine(c *gin.Context) {
	var r dto.SubmitRoutineRequest
	if !Bind(c, &r, h.Validate) {
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	v, e := h.svc.SubmitRoutine(uint(id), c.GetUint("userID"), r.Result, r.Finding, c.GetString("role"))
	if e != nil {
		ServiceError(c, e)
		return
	}
	OK(c, v)
}

func (h *InspectionTaskHandler) SubmitRecheck(c *gin.Context) {
	var r dto.SubmitRecheckRequest
	if !Bind(c, &r, h.Validate) {
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	v, e := h.svc.SubmitRecheck(uint(id), c.GetUint("userID"), r.Result, r.Finding, c.GetString("role"))
	if e != nil {
		ServiceError(c, e)
		return
	}
	OK(c, v)
}
