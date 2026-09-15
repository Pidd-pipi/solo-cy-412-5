package service

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/model"
	"github.com/smartestate/smartestate/internal/repository"
)

type InspectionPlanService struct {
	planRepo *repository.InspectionPlanRepository
	facRepo  *repository.FacilityRepository
	taskRepo *repository.InspectionTaskRepository
	logger   *slog.Logger
}

func NewInspectionPlanService(p *repository.InspectionPlanRepository, f *repository.FacilityRepository, t *repository.InspectionTaskRepository, l *slog.Logger) *InspectionPlanService {
	return &InspectionPlanService{p, f, t, l}
}

// Create 建计划。同一设施同一周期唯一，重复创建返回 ErrConflict。
func (s *InspectionPlanService) Create(facilityID uint, name, cycle string, startDate time.Time) (model.InspectionPlan, error) {
	if !constants.ValidCycles[cycle] {
		return model.InspectionPlan{}, fmt.Errorf("%w: InspectionPlan facility=%d invalid cycle=%s", ErrInvalidState, facilityID, cycle)
	}
	if _, e := s.facRepo.ByID(facilityID); e != nil {
		if errors.Is(e, repository.ErrNotFound) {
			return model.InspectionPlan{}, ErrNotFound
		}
		return model.InspectionPlan{}, fmt.Errorf("InspectionPlan facility=%d load failed: %w", facilityID, e)
	}
	if _, e := s.planRepo.ByFacilityAndCycle(facilityID, cycle); e == nil {
		return model.InspectionPlan{}, fmt.Errorf("%w: InspectionPlan facility=%d cycle=%s already exists", ErrConflict, facilityID, cycle)
	} else if !errors.Is(e, repository.ErrNotFound) {
		return model.InspectionPlan{}, fmt.Errorf("InspectionPlan facility=%d cycle=%s check failed: %w", facilityID, cycle, e)
	}
	v := model.InspectionPlan{FacilityID: facilityID, Name: name, Cycle: cycle, StartDate: startDate, Active: true}
	if e := s.planRepo.Create(&v); e != nil {
		if errors.Is(e, repository.ErrDuplicate) {
			return model.InspectionPlan{}, fmt.Errorf("%w: InspectionPlan facility=%d cycle=%s duplicated: %w", ErrConflict, facilityID, cycle, e)
		}
		return model.InspectionPlan{}, fmt.Errorf("InspectionPlan facility=%d create failed: %w", facilityID, e)
	}
	return s.planRepo.ByID(v.ID)
}

func (s *InspectionPlanService) List(active *bool) ([]model.InspectionPlan, error) {
	return s.planRepo.List(active)
}

// GenerateDue 对所有启用计划重跑到期生成；可重复调用，已存在的期次任务不会重建。
// 返回本次新生成的任务数（计划重跑幂等时为 0）。
func (s *InspectionPlanService) GenerateDue(now time.Time) (int, error) {
	plans, e := s.planRepo.ListActiveDue()
	if e != nil {
		return 0, fmt.Errorf("InspectionPlan generate load failed: %w", e)
	}
	created := 0
	for _, p := range plans {
		period := periodKey(p.Cycle, now)
		exists, e := s.taskRepo.ExistsSlot(p.FacilityID, constants.TaskKindRoutine, p.Cycle, period)
		if e != nil {
			return created, fmt.Errorf("InspectionPlan[id=%d] dedupe failed: %w", p.ID, e)
		}
		if exists {
			continue
		}
		task := model.InspectionTask{
			FacilityID:  p.FacilityID,
			PlanID:      &p.ID,
			Kind:        constants.TaskKindRoutine,
			Cycle:       p.Cycle,
			PeriodValue: period,
			DueDate:     dueDate(p.Cycle, p.StartDate, now),
			Status:      constants.TaskStatusPending,
		}
		if e := s.taskRepo.Create(&task); e != nil {
			// 并发重跑时唯一索引兜底：视作已存在，不算失败、不重复计数。
			if errors.Is(e, repository.ErrDuplicate) {
				continue
			}
			return created, fmt.Errorf("InspectionPlan[id=%d] generate task failed: %w", p.ID, e)
		}
		created++
		s.logger.Info("inspection task generated", "plan_id", p.ID, "facility_id", p.FacilityID, "period", period)
	}
	return created, nil
}
