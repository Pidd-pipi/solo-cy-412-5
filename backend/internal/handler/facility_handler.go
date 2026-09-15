package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/smartestate/smartestate/internal/dto"
	"github.com/smartestate/smartestate/internal/service"
)

type FacilityHandler struct {
	Handler
	svc *service.FacilityService
}

func NewFacilityHandler(s *service.FacilityService, h *Handler) *FacilityHandler {
	return &FacilityHandler{Handler: *h, svc: s}
}

func (h *FacilityHandler) List(c *gin.Context) {
	v, e := h.svc.List(c.Query("status"))
	if e != nil {
		ServiceError(c, e)
		return
	}
	OK(c, v)
}

func (h *FacilityHandler) Detail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	v, e := h.svc.Detail(uint(id))
	if e != nil {
		ServiceError(c, e)
		return
	}
	OK(c, v)
}

func (h *FacilityHandler) Create(c *gin.Context) {
	var r dto.CreateFacilityRequest
	if !Bind(c, &r, h.Validate) {
		return
	}
	v, e := h.svc.Create(r.Name, r.Category, r.Location, r.Remark)
	if e != nil {
		ServiceError(c, e)
		return
	}
	OK(c, v)
}
