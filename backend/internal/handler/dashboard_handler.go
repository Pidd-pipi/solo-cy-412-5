package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/smartestate/smartestate/internal/service"
)

type DashboardHandler struct {
	repairs  *service.RepairService
	payments *service.PaymentService
	anns     *service.AnnouncementService
	facs     *service.FacilityService
	tasks    *service.InspectionTaskService
}

func NewDashboardHandler(r *service.RepairService, p *service.PaymentService, a *service.AnnouncementService, f *service.FacilityService, t *service.InspectionTaskService) *DashboardHandler {
	return &DashboardHandler{r, p, a, f, t}
}
func (h *DashboardHandler) Summary(c *gin.Context) {
	open, _ := h.repairs.OpenCount()
	amount, _ := h.payments.MonthlyPaid()
	anns, _ := h.anns.List()
	if len(anns) > 3 {
		anns = anns[:3]
	}
	due, _ := h.tasks.DueCount()
	disabled, _ := h.facs.DisabledCount()
	unclosed, _ := h.repairs.UnclosedFacilityCount()
	OK(c, gin.H{
		"pending_repairs":           open,
		"monthly_paid":              amount,
		"announcements":             anns,
		"pending_inspections":       due,
		"disabled_facilities":       disabled,
		"unclosed_facility_repairs": unclosed,
	})
}
