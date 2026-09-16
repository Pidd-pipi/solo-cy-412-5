package service

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/model"
	"github.com/smartestate/smartestate/internal/repository"
	"gorm.io/gorm"
)

type InspectionTaskService struct {
	db     *gorm.DB
	repo   *repository.InspectionTaskRepository
	flow   disposition
	logger *slog.Logger
}

func NewInspectionTaskService(db *gorm.DB, r *repository.InspectionTaskRepository, f *repository.FacilityRepository, rp *repository.RepairRepository, l *slog.Logger) *InspectionTaskService {
	return &InspectionTaskService{db: db, repo: r, flow: newDisposition(db, f, r, rp, l), logger: l}
}

func (s *InspectionTaskService) List(f repository.TaskFilter) ([]model.InspectionTask, error) {
	return s.repo.List(f)
}

func (s *InspectionTaskService) ByID(id uint) (model.InspectionTask, error) {
	v, e := s.repo.ByID(id)
	if errors.Is(e, repository.ErrNotFound) {
		return v, ErrNotFound
	}
	return v, e
}

// Claim 接单：条件更新保证多人同时接单只有一人成功。
func (s *InspectionTaskService) Claim(id, userID uint, role string) (model.InspectionTask, error) {
	ok, e := s.repo.Claim(id, userID, constants.TaskStatusPending)
	if e != nil {
		return model.InspectionTask{}, fmt.Errorf("InspectionTask[id=%d] claim failed: %w", id, e)
	}
	if !ok {
		cur, _ := s.repo.ByID(id)
		if constants.TerminalTaskStatuses[cur.Status] {
			return cur, fmt.Errorf("%w: InspectionTask[id=%d] already finalized, current role=%s", ErrImmutable, id, role)
		}
		return cur, fmt.Errorf("%w: InspectionTask[id=%d] already claimed by another inspector, current role=%s", ErrAlreadyClaimed, id, role)
	}
	s.logger.Info("inspection task claimed", "task_id", id, "inspector_id", userID)
	return s.repo.ByID(id)
}

// SubmitRoutine 提交常规巡检结果。normal=完成（设施保持可用）；hazard=停用设施并生成唯一维修单。
func (s *InspectionTaskService) SubmitRoutine(id, userID uint, result, finding string, role string) (model.InspectionTask, error) {
	finding = strings.TrimSpace(finding)
	if result != constants.ResultNormal && result != constants.ResultHazard {
		return model.InspectionTask{}, fmt.Errorf("%w: InspectionTask[id=%d] invalid routine result=%s", ErrInvalidState, id, result)
	}
	if result == constants.ResultHazard && finding == "" {
		return model.InspectionTask{}, fmt.Errorf("%w: InspectionTask[id=%d] hazard finding is required", ErrInvalidState, id)
	}
	e := s.db.Transaction(func(tx *gorm.DB) error {
		t, e := s.flow.tasks.ByIDForUpdate(tx, id)
		if e != nil {
			if errors.Is(e, repository.ErrNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("InspectionTask[id=%d] load failed: %w", id, e)
		}
		if constants.TerminalTaskStatuses[t.Status] {
			return fmt.Errorf("%w: InspectionTask[id=%d] result cannot be changed, current role=%s", ErrImmutable, id, role)
		}
		if t.Status != constants.TaskStatusPending && t.Status != constants.TaskStatusClaimed {
			return fmt.Errorf("%w: InspectionTask[id=%d] cannot submit from status=%s", ErrInvalidState, id, t.Status)
		}
		if e = s.ensureOwner(t, userID); e != nil {
			return e
		}
		now := time.Now()
		if result == constants.ResultNormal {
			ok, ae := s.flow.tasks.AdvanceInTx(tx, id,
				[]string{constants.TaskStatusPending, constants.TaskStatusClaimed},
				map[string]interface{}{
					"status": constants.TaskStatusDone, "result": constants.ResultNormal,
					"finding": finding, "inspector_id": userID, "reviewed_at": now,
				})
			if ae != nil {
				return fmt.Errorf("InspectionTask[id=%d] finish failed: %w", id, ae)
			}
			if !ok {
				return ErrConflict
			}
			return nil
		}
		repairID, ae := s.flow.disableAndCreateRepair(tx, t, finding, userID)
		if ae != nil {
			return ae
		}
		ok, ae := s.flow.tasks.AdvanceInTx(tx, id,
			[]string{constants.TaskStatusPending, constants.TaskStatusClaimed},
			map[string]interface{}{
				"status": constants.TaskStatusHazard, "result": constants.ResultHazard,
				"finding": finding, "inspector_id": userID, "reviewed_at": now,
				"hazard_repair_id": repairID,
			})
		if ae != nil {
			return fmt.Errorf("InspectionTask[id=%d] hazard failed: %w", id, ae)
		}
		if !ok {
			return ErrConflict
		}
		return nil
	})
	if e != nil {
		return model.InspectionTask{}, e
	}
	s.logger.Info("inspection routine submitted", "task_id", id, "result", result)
	return s.repo.ByID(id)
}

// SubmitRecheck 提交复检结果。pass=恢复设施可用；fail=保持停用并续建维修工单。
func (s *InspectionTaskService) SubmitRecheck(id, userID uint, result, finding string, role string) (model.InspectionTask, error) {
	finding = strings.TrimSpace(finding)
	if result != constants.ResultPass && result != constants.ResultFail {
		return model.InspectionTask{}, fmt.Errorf("%w: InspectionTask[id=%d] invalid recheck result=%s", ErrInvalidState, id, result)
	}
	if result == constants.ResultFail && finding == "" {
		return model.InspectionTask{}, fmt.Errorf("%w: InspectionTask[id=%d] fail finding is required", ErrInvalidState, id)
	}
	e := s.db.Transaction(func(tx *gorm.DB) error {
		t, e := s.flow.tasks.ByIDForUpdate(tx, id)
		if e != nil {
			if errors.Is(e, repository.ErrNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("InspectionTask[id=%d] load failed: %w", id, e)
		}
		if t.Kind != constants.TaskKindRecheck {
			return fmt.Errorf("%w: InspectionTask[id=%d] is not a recheck task", ErrInvalidState, id)
		}
		if constants.TerminalTaskStatuses[t.Status] {
			return fmt.Errorf("%w: InspectionTask[id=%d] recheck cannot be changed, current role=%s", ErrImmutable, id, role)
		}
		if t.Status != constants.TaskStatusPending && t.Status != constants.TaskStatusClaimed {
			return fmt.Errorf("%w: InspectionTask[id=%d] cannot recheck from status=%s", ErrInvalidState, id, t.Status)
		}
		if e = s.ensureOwner(t, userID); e != nil {
			return e
		}
		now := time.Now()
		if result == constants.ResultPass {
			// 锁住设施行，串行化并发复检：仅在所有关联隐患工单与待复检都闭环时才恢复可用。
			if e = s.flow.lockFacility(tx, t.FacilityID); e != nil {
				return e
			}
			restored, re := s.flow.restoreIfAllClosed(tx, t.FacilityID, id)
			if re != nil {
				return re
			}
			newStatus := constants.TaskStatusRecheckPassed
			if restored {
				newStatus = constants.TaskStatusRestored
			}
			ok, ae := s.flow.tasks.AdvanceInTx(tx, id,
				[]string{constants.TaskStatusPending, constants.TaskStatusClaimed},
				map[string]interface{}{
					"status": newStatus, "result": constants.ResultPass,
					"finding": finding, "inspector_id": userID, "reviewed_at": now,
				})
			if ae != nil {
				return fmt.Errorf("InspectionTask[id=%d] recheck-pass advance failed: %w", id, ae)
			}
			if !ok {
				return ErrConflict
			}
			return nil
		}
		if e = s.flow.followupRepair(tx, t, finding, userID); e != nil {
			return e
		}
		ok, ae := s.flow.tasks.AdvanceInTx(tx, id,
			[]string{constants.TaskStatusPending, constants.TaskStatusClaimed},
			map[string]interface{}{
				"status": constants.TaskStatusRecheckFailed, "result": constants.ResultFail,
				"finding": finding, "inspector_id": userID, "reviewed_at": now,
			})
		if ae != nil {
			return fmt.Errorf("InspectionTask[id=%d] recheck-fail advance failed: %w", id, ae)
		}
		if !ok {
			return ErrConflict
		}
		return nil
	})
	if e != nil {
		return model.InspectionTask{}, e
	}
	s.logger.Info("inspection recheck submitted", "task_id", id, "result", result)
	return s.repo.ByID(id)
}

func (s *InspectionTaskService) ensureOwner(t model.InspectionTask, userID uint) error {
	if t.Status == constants.TaskStatusClaimed && t.InspectorID != nil && *t.InspectorID != userID {
		return fmt.Errorf("%w: InspectionTask[id=%d] claimed by inspector=%d not %d", ErrForbidden, t.ID, *t.InspectorID, userID)
	}
	return nil
}

func (s *InspectionTaskService) DueCount() (int64, error) {
	return s.repo.CountDue(time.Now())
}
