package service

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/model"
	"github.com/smartestate/smartestate/internal/repository"
	"gorm.io/gorm"
)

type RepairService struct {
	repo     *repository.RepairRepository
	users    *repository.UserRepository
	taskRepo *repository.InspectionTaskRepository
	flow     disposition
	db       *gorm.DB
	logger   *slog.Logger
}

func NewRepairService(db *gorm.DB, r *repository.RepairRepository, u *repository.UserRepository, t *repository.InspectionTaskRepository, f *repository.FacilityRepository, l *slog.Logger) *RepairService {
	return &RepairService{repo: r, users: u, taskRepo: t, flow: newDisposition(db, f, t, r, l), db: db, logger: l}
}

func (s *RepairService) Create(uid uint, title, desc, typ, images string) (model.Repair, error) {
	v := model.Repair{UserID: uid, Title: title, Description: desc, Type: typ, Images: images, Status: constants.RepairStatusPending}
	if e := s.repo.Create(&v); e != nil {
		return v, fmt.Errorf("Repair[user_id=%d] create failed: %w", uid, e)
	}
	return s.repo.ByID(v.ID)
}
func (s *RepairService) List(status string) ([]model.Repair, error) { return s.repo.List(status) }

func (s *RepairService) Assign(id, handlerID uint, role string) (model.Repair, error) {
	handler, e := s.users.ByID(handlerID)
	if e != nil {
		return model.Repair{}, fmt.Errorf("Repair[id=%d] assign failed: staff not found, current role=%s: %w", id, role, e)
	}
	if handler.Role != constants.UserRoleStaff && handler.Role != constants.UserRoleAdmin {
		return model.Repair{}, fmt.Errorf("Repair[id=%d] assign failed: handler %d is not staff/admin, current role=%s", id, handlerID, role)
	}
	v, e := s.repo.ByID(id)
	if e != nil {
		return v, e
	}
	v.HandlerID = &handlerID
	v.Handler = nil
	v.User = model.User{}
	v.Status = constants.RepairStatusAssigned
	if e = s.repo.Update(&v); e != nil {
		return v, fmt.Errorf("Repair[id=%d] assign failed: %w", id, e)
	}
	return s.repo.ByID(id)
}

func (s *RepairService) UpdateStatus(id uint, status string, rating int, role string) (model.Repair, error) {
	if !constants.ValidRepairStatuses[status] {
		return model.Repair{}, fmt.Errorf("Repair[id=%d] status failed: invalid status, current role=%s", id, role)
	}
	v, e := s.repo.ByID(id)
	if e != nil {
		return v, e
	}
	// 巡检关联工单“维修完成”：在同一事务内条件推进并幂等安排复检，
	// 保证重复提交不会重复生成复检任务，也不会相互覆盖状态。
	if v.FacilityID != nil && status == constants.RepairStatusDone {
		if e = s.completeFacilityRepair(&v, rating); e != nil {
			return model.Repair{}, e
		}
		return s.repo.ByID(id)
	}
	v.Status = status
	if rating > 0 {
		v.Rating = rating
	}
	if e = s.repo.Update(&v); e != nil {
		return v, fmt.Errorf("Repair[id=%d] status failed: %w", id, e)
	}
	return s.repo.ByID(id)
}

// completeFacilityRepair 条件地把关联工单置为 done 并安排且仅安排一次复检。
func (s *RepairService) completeFacilityRepair(v *model.Repair, rating int) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		cur, e := s.repo.ByIDForUpdate(tx, v.ID)
		if e != nil {
			if errors.Is(e, repository.ErrNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("Repair[id=%d] complete load failed: %w", v.ID, e)
		}
		// 已完成/已闭环：幂等返回，绝不重复安排复检。
		if cur.Status == constants.RepairStatusDone || cur.Status == constants.RepairStatusClosed {
			return nil
		}
		fields := map[string]interface{}{}
		if rating > 0 {
			fields["rating"] = rating
		}
		ok, e := s.repo.AdvanceStatusInTx(tx, v.ID,
			[]string{constants.RepairStatusPending, constants.RepairStatusAssigned, constants.RepairStatusProcessing},
			constants.RepairStatusDone, fields)
		if e != nil {
			return fmt.Errorf("Repair[id=%d] complete failed: %w", v.ID, e)
		}
		if !ok {
			return fmt.Errorf("%w: Repair[id=%d] already advanced concurrently", ErrConflict, v.ID)
		}
		if _, e = s.flow.createRecheckTask(tx, *cur.FacilityID, cur.ID, cur.SourceTaskID); e != nil {
			return e
		}
		s.logger.Info("facility repair completed, recheck scheduled", "repair_id", cur.ID, "facility_id", *cur.FacilityID)
		return nil
	})
}

func (s *RepairService) OpenCount() (int64, error) { return s.repo.CountOpen() }

// UnclosedFacilityCount 停用处置中尚未闭环的数量：未完成的关联维修单 + 待处理复检任务。
// 复检等待期设施仍停用，因此并入该项可让“未闭环工单”与“停用设施”计数保持同步。
func (s *RepairService) UnclosedFacilityCount() (int64, error) {
	openRepairs, e := s.repo.CountUnclosedFacility()
	if e != nil {
		return 0, e
	}
	openRechecks, e := s.taskRepo.CountOpenRecheck()
	if e != nil {
		return 0, e
	}
	return openRepairs + openRechecks, nil
}
