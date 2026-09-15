import {request} from '../utils/request';
import type {Facility, FacilityDetail, FacilityStatus} from '../types';

export interface FacilityPayload {
  name: string; category: string; location: string; remark?: string
}

export const listFacilities = (status?: FacilityStatus | '') =>
  request<Facility[]>(`/facilities${status ? `?status=${status}` : ''}`);

export const detailFacility = (id: number) =>
  request<FacilityDetail>(`/facilities/${id}`);

export const createFacility = (data: FacilityPayload) =>
  request<Facility>('/facilities', {method: 'POST', body: JSON.stringify(data)});
