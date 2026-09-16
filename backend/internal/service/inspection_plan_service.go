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
	if startDate.IsZero() {
		return model.InspectionPlan{}, fmt.Errorf("%w: InspectionPlan facility=%d invalid start_date", ErrInvalidState, facilityID)
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

// GenerateDue 对所有启用计划重跑到期生成，可重复调用。
//
// 补齐语义：服务跨多个周期才重跑时，按计划开始日补齐“起始期次 → 当前期次”的全部到期任务，
// 每个期次只生成一条；期次任务已存在（含已完成/已停用等终态）时跳过，绝不改写既有结果。
// 逐期次独立写入（非单个大事务），因此中途失败时已成功的期次保留，重试只继续缺失期次；
// 唯一索引 + ExistsSlot 双重判重使重试不会重复累计待巡检数量。
// 返回本次新生成的任务数。
func (s *InspectionPlanService) GenerateDue(now time.Time) (int, error) {
	plans, e := s.planRepo.ListActiveDue()
	if e != nil {
		return 0, fmt.Errorf("InspectionPlan generate load failed: %w", e)
	}
	created := 0
	var firstErr error
	for _, p := range plans {
		periods, e := enumerateDuePeriods(p.Cycle, p.StartDate, now)
		if e != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("InspectionPlan[id=%d] enumerate periods failed: %w", p.ID, e)
			}
			continue
		}
		for _, period := range periods {
			exists, e := s.taskRepo.ExistsSlot(p.FacilityID, constants.TaskKindRoutine, p.Cycle, period.Key)
			if e != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("InspectionPlan[id=%d] dedupe period=%s failed: %w", p.ID, period.Key, e)
				}
				continue
			}
			if exists {
				continue // 已完成或已存在的期次不改写
			}
			task := model.InspectionTask{
				FacilityID:  p.FacilityID,
				PlanID:      &p.ID,
				Kind:        constants.TaskKindRoutine,
				Cycle:       p.Cycle,
				PeriodValue: period.Key,
				DueDate:     period.Due,
				Status:      constants.TaskStatusPending,
			}
			if e := s.taskRepo.Create(&task); e != nil {
				// 并发重跑时唯一索引兜底：视作已存在，不算失败、不重复计数。
				if errors.Is(e, repository.ErrDuplicate) {
					continue
				}
				if firstErr == nil {
					firstErr = fmt.Errorf("InspectionPlan[id=%d] generate period=%s failed: %w", p.ID, period.Key, e)
				}
				continue
			}
			created++
			s.logger.Info("inspection task generated", "plan_id", p.ID, "facility_id", p.FacilityID, "period", period.Key)
		}
	}
	return created, firstErr
}
