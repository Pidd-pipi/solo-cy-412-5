import type {FacilityStatus, InspectionCycle, InspectionTaskKind, InspectionTaskStatus} from '../types';

export const FACILITY_STATUS: Record<Uppercase<FacilityStatus>, FacilityStatus> = {
  AVAILABLE: 'available', DISABLED: 'disabled'
};
export const facilityStatusText: Record<FacilityStatus, string> = {
  available: '可用', disabled: '停用'
};

export const CYCLE_TEXT: Record<InspectionCycle, string> = {
  daily: '每日', weekly: '每周', monthly: '每月', quarterly: '每季度', yearly: '每年'
};

export const TASK_KIND_TEXT: Record<InspectionTaskKind, string> = {
  routine: '常规巡检', recheck: '维修复检'
};

export const INSPECTION_TASK_STATUS: Record<Uppercase<InspectionTaskStatus>, InspectionTaskStatus> = {
  PENDING: 'pending', CLAIMED: 'claimed', DONE: 'done',
  HAZARD: 'hazard', RECHECK_FAILED: 'recheck_failed', RESTORED: 'restored'
};
export const inspectionTaskStatusText: Record<InspectionTaskStatus, string> = {
  pending: '待巡检',
  claimed: '已接单',
  done: '巡检正常',
  hazard: '发现隐患·停用',
  recheck_failed: '复检未过·停用',
  restored: '复检通过·恢复'
};
// 任务状态 -> Element Plus 标签颜色（与 RepairStatusBadge 风格一致）。
export const inspectionTaskStatusKind: Record<InspectionTaskStatus, 'warning'|'primary'|'success'|'danger'|'info'> = {
  pending: 'warning',
  claimed: 'primary',
  done: 'success',
  hazard: 'danger',
  recheck_failed: 'danger',
  restored: 'success'
};
export const facilityStatusKind: Record<FacilityStatus, 'success'|'danger'> = {
  available: 'success', disabled: 'danger'
};
