import {request} from '../utils/request';
import type {InspectionResult, InspectionTask, InspectionTaskKind, InspectionTaskStatus} from '../types';

export interface TaskQuery {
  status?: InspectionTaskStatus | '';
  kind?: InspectionTaskKind | '';
  facility_id?: number;
  open?: boolean
}

export const listInspectionTasks = (q: TaskQuery = {}) => {
  const parts: string[] = [];
  if (q.status) parts.push(`status=${q.status}`);
  if (q.kind) parts.push(`kind=${q.kind}`);
  if (q.facility_id) parts.push(`facility_id=${q.facility_id}`);
  if (q.open) parts.push('open=true');
  const qs = parts.length ? `?${parts.join('&')}` : '';
  return request<InspectionTask[]>(`/inspection-tasks${qs}`);
};

export const claimInspectionTask = (id: number) =>
  request<InspectionTask>(`/inspection-tasks/${id}/claim`, {method: 'PATCH'});

export const submitRoutine = (id: number, result: 'normal' | 'hazard', finding: string) =>
  request<InspectionTask>(`/inspection-tasks/${id}/routine`, {
    method: 'POST', body: JSON.stringify({result, finding})
  });

export const submitRecheck = (id: number, result: 'pass' | 'fail', finding: string) =>
  request<InspectionTask>(`/inspection-tasks/${id}/recheck`, {
    method: 'POST', body: JSON.stringify({result, finding})
  });

export type {InspectionResult};
