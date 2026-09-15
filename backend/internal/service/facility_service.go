package service

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/model"
	"github.com/smartestate/smartestate/internal/repository"
)

// FacilityDetail 设施详情：同步设施状态与巡检处置进度。
type FacilityDetail struct {
	model.Facility
	Tasks   []model.InspectionTask `json:"tasks"`
	Repairs []model.Repair         `json:"repairs"`
}

type FacilityService struct {
	facRepo    *repository.FacilityRepository
	taskRepo   *repository.InspectionTaskRepository
	repairRepo *repository.RepairRepository
	logger     *slog.Logger
}

func NewFacilityService(f *repository.FacilityRepository, t *repository.InspectionTaskRepository, rp *repository.RepairRepository, l *slog.Logger) *FacilityService {
	return &FacilityService{f, t, rp, l}
}

func (s *FacilityService) Create(name, category, location, remark string) (model.Facility, error) {
	v := model.Facility{Name: name, Category: category, Location: location, Remark: remark, Status: constants.FacilityStatusAvailable}
	if e := s.facRepo.Create(&v); e != nil {
		return v, fmt.Errorf("Facility[name=%s] create failed: %w", name, e)
	}
	return v, nil
}

func (s *FacilityService) List(status string) ([]model.Facility, error) {
	return s.facRepo.List(status)
}

func (s *FacilityService) Detail(id uint) (FacilityDetail, error) {
	f, e := s.facRepo.ByID(id)
	if e != nil {
		if errors.Is(e, repository.ErrNotFound) {
			return FacilityDetail{}, ErrNotFound
		}
		return FacilityDetail{}, fmt.Errorf("Facility[id=%d] detail failed: %w", id, e)
	}
	tasks, e := s.taskRepo.List(repository.TaskFilter{FacilityID: id})
	if e != nil {
		return FacilityDetail{}, fmt.Errorf("Facility[id=%d] detail tasks failed: %w", id, e)
	}
	repairs, e := s.repairRepo.ListByFacility(id)
	if e != nil {
		return FacilityDetail{}, fmt.Errorf("Facility[id=%d] detail repairs failed: %w", id, e)
	}
	return FacilityDetail{Facility: f, Tasks: tasks, Repairs: repairs}, nil
}

func (s *FacilityService) DisabledCount() (int64, error) {
	return s.facRepo.CountByStatus(constants.FacilityStatusDisabled)
}
