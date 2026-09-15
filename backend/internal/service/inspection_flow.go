package service

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/model"
	"github.com/smartestate/smartestate/internal/repository"
	"gorm.io/gorm"
)

// disposition 汇总巡检停用处置流程所需仓储，所有跨表写操作都在单个数据库事务内完成，
// 保证“停用设施 + 生成维修单 + 推进任务状态”原子且互斥。
type disposition struct {
	db      *gorm.DB
	fac     *repository.FacilityRepository
	tasks   *repository.InspectionTaskRepository
	repairs *repository.RepairRepository
	logger  *slog.Logger
}

func newDisposition(db *gorm.DB, f *repository.FacilityRepository, t *repository.InspectionTaskRepository, r *repository.RepairRepository, l *slog.Logger) disposition {
	return disposition{db: db, fac: f, tasks: t, repairs: r, logger: l}
}

// disableAndCreateRepair 设施立即停用，并只生成一张关联维修工单。
// repair.source_task_id 的唯一索引确保一张巡检任务最多生成一张工单。
func (d disposition) disableAndCreateRepair(tx *gorm.DB, t model.InspectionTask, finding string, inspectorID uint) (uint, error) {
	fac, e := d.fac.ByIDForUpdate(tx, t.FacilityID)
	if e != nil {
		return 0, fmt.Errorf("facility %d lock failed: %w", t.FacilityID, e)
	}
	if fac.Status != constants.FacilityStatusDisabled {
		if e = d.fac.UpdateStatusInTx(tx, fac.ID, constants.FacilityStatusDisabled); e != nil {
			return 0, fmt.Errorf("facility %d disable failed: %w", fac.ID, e)
		}
	}
	taskID := t.ID
	facID := t.FacilityID
	repair := model.Repair{
		UserID:       inspectorID,
		Title:        fmt.Sprintf("【巡检隐患】%s 需维修", fac.Name),
		Description:  fmt.Sprintf("巡检发现安全隐患：%s。设施已立即停用，请尽快维修。", finding),
		Type:         "公共设施",
		Status:       constants.RepairStatusPending,
		FacilityID:   &facID,
		SourceTaskID: &taskID,
	}
	if e = d.repairs.CreateInTx(tx, &repair); e != nil {
		if errors.Is(e, repository.ErrDuplicate) {
			// 已存在关联工单：回查其 ID，绝不重复生成。
			existing, qerr := d.tasks.ByID(t.ID)
			if qerr == nil && existing.HazardRepairID != nil {
				return *existing.HazardRepairID, nil
			}
		}
		return 0, fmt.Errorf("InspectionTask[id=%d] create repair failed: %w", t.ID, e)
	}
	return repair.ID, nil
}

// createRecheckTask 维修完成后安排复检。同一维修单只安排一次复检（source_repair_id 唯一索引兜底）。
func (d disposition) createRecheckTask(tx *gorm.DB, facilityID, repairID uint, sourceTaskID *uint) (model.InspectionTask, error) {
	exists, e := d.tasks.RecheckExistsTx(tx, repairID)
	if e != nil {
		return model.InspectionTask{}, fmt.Errorf("recheck dedupe repair=%d failed: %w", repairID, e)
	}
	if exists {
		return model.InspectionTask{}, nil
	}
	cycle := d.resolveCycle(tx, facilityID, sourceTaskID)
	now := time.Now()
	task := model.InspectionTask{
		FacilityID:     facilityID,
		Kind:           constants.TaskKindRecheck,
		Cycle:          cycle,
		PeriodValue:    recheckPeriod(repairID),
		DueDate:        now,
		Status:         constants.TaskStatusPending,
		SourceRepairID: &repairID,
	}
	if e = d.tasks.CreateInTx(tx, &task); e != nil {
		if errors.Is(e, repository.ErrDuplicate) {
			return model.InspectionTask{}, nil
		}
		return model.InspectionTask{}, fmt.Errorf("Repair[id=%d] create recheck failed: %w", repairID, e)
	}
	return task, nil
}

// followupRepair 复检未通过：设施保持停用，并续建一张维修工单进入下一轮维修-复检。
func (d disposition) followupRepair(tx *gorm.DB, t model.InspectionTask, finding string, inspectorID uint) error {
	fac, e := d.fac.ByIDForUpdate(tx, t.FacilityID)
	if e != nil {
		return fmt.Errorf("facility %d lock failed: %w", t.FacilityID, e)
	}
	if fac.Status != constants.FacilityStatusDisabled {
		if e = d.fac.UpdateStatusInTx(tx, fac.ID, constants.FacilityStatusDisabled); e != nil {
			return fmt.Errorf("facility %d keep-disabled failed: %w", fac.ID, e)
		}
	}
	taskID := t.ID
	facID := t.FacilityID
	repair := model.Repair{
		UserID:       inspectorID,
		Title:        fmt.Sprintf("【复检未过】%s 续修", fac.Name),
		Description:  fmt.Sprintf("复检未通过：%s。设施维持停用，请继续维修后再次申请复检。", finding),
		Type:         "公共设施",
		Status:       constants.RepairStatusPending,
		FacilityID:   &facID,
		SourceTaskID: &taskID,
	}
	if e = d.repairs.CreateInTx(tx, &repair); e != nil {
		return fmt.Errorf("InspectionTask[id=%d] create followup repair failed: %w", t.ID, e)
	}
	return nil
}

// restore 复检通过，设施恢复可用。
func (d disposition) restore(tx *gorm.DB, facilityID uint) error {
	if e := d.fac.UpdateStatusInTx(tx, facilityID, constants.FacilityStatusAvailable); e != nil {
		return fmt.Errorf("facility %d restore failed: %w", facilityID, e)
	}
	return nil
}

// resolveCycle 推导复检任务周期：优先沿用来源巡检任务的周期，否则取设施最近任务，最后回退 monthly。
func (d disposition) resolveCycle(tx *gorm.DB, facilityID uint, sourceTaskID *uint) string {
	if sourceTaskID != nil {
		if src, e := d.tasks.ByIDForUpdate(tx, *sourceTaskID); e == nil && constants.ValidCycles[src.Cycle] {
			return src.Cycle
		}
	}
	if latest, e := d.tasks.LatestByFacility(facilityID); e == nil && constants.ValidCycles[latest.Cycle] {
		return latest.Cycle
	}
	return constants.CycleMonthly
}
