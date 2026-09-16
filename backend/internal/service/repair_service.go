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
	// 巡检关联工单进入终态：done=安排复检；closed=直接闭环（不复检），二者都在事务内
	// 条件推进，并在“全部隐患链已闭环”时才恢复设施；重复提交幂等，不重复安排复检。
	if v.FacilityID != nil && (status == constants.RepairStatusDone || status == constants.RepairStatusClosed) {
		if e = s.finalizeFacilityRepair(&v, status, rating); e != nil {
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

// finalizeFacilityRepair 条件地把关联工单推进到终态（done/closed）。
// done：幂等安排一次复检；closed：直接闭环、不安排复检。两种方式在设施上同步加锁，
// 仅当该设施再无未闭环维修单与待处理复检时才恢复可用，保证“直接关闭后的最终闭环”也能恢复。
func (s *RepairService) finalizeFacilityRepair(v *model.Repair, target string, rating int) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		cur, e := s.repo.ByIDForUpdate(tx, v.ID)
		if e != nil {
			if errors.Is(e, repository.ErrNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("Repair[id=%d] finalize load failed: %w", v.ID, e)
		}
		// 已终态：幂等返回，绝不重复安排复检或重复恢复。
		if cur.Status == constants.RepairStatusDone || cur.Status == constants.RepairStatusClosed {
			return nil
		}
		facilityID := *cur.FacilityID
		// 与复检恢复互斥：锁住设施行，避免“安排复检”与“复检通过恢复”交叉。
		if e = s.flow.lockFacility(tx, facilityID); e != nil {
			return e
		}
		fields := map[string]interface{}{}
		if rating > 0 {
			fields["rating"] = rating
		}
		ok, e := s.repo.AdvanceStatusInTx(tx, v.ID,
			[]string{constants.RepairStatusPending, constants.RepairStatusAssigned, constants.RepairStatusProcessing},
			target, fields)
		if e != nil {
			return fmt.Errorf("Repair[id=%d] finalize to %s failed: %w", v.ID, target, e)
		}
		if !ok {
			return fmt.Errorf("%w: Repair[id=%d] already advanced concurrently", ErrConflict, v.ID)
		}
		if target == constants.RepairStatusDone {
			if _, e = s.flow.createRecheckTask(tx, facilityID, cur.ID, cur.SourceTaskID); e != nil {
				return e
			}
			s.logger.Info("facility repair completed, recheck scheduled", "repair_id", cur.ID, "facility_id", facilityID)
			return nil
		}
		// 直接关闭：无复检。若这是最后一条未闭环链，则恢复设施。
		restored, e := s.flow.restoreIfAllClosed(tx, facilityID, 0)
		if e != nil {
			return e
		}
		s.logger.Info("facility repair closed directly", "repair_id", cur.ID, "facility_id", facilityID, "restored", restored)
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
