import {request} from '../utils/request';
import type {InspectionCycle, InspectionPlan} from '../types';

export interface InspectionPlanPayload {
  facility_id: number; name: string; cycle: InspectionCycle; start_date?: string
}

export const listInspectionPlans = (active?: boolean) =>
  request<InspectionPlan[]>(`/inspection-plans${typeof active === 'boolean' ? `?active=${active}` : ''}`);

export const createInspectionPlan = (data: InspectionPlanPayload) =>
  request<InspectionPlan>('/inspection-plans', {method: 'POST', body: JSON.stringify(data)});

// 到期重跑：幂等，重复调用不产生重复任务。
export const generateInspectionTasks = () =>
  request<{generated: number}>('/inspection-plans/generate', {method: 'POST', body: '{}'});
