export type UserRole='resident'|'staff'|'admin'; export type RepairStatus='pending'|'assigned'|'processing'|'done'|'closed'; export interface User{id:number;phone:string;nickname:string;avatar:string;role:UserRole;building:string;unit:string;room:string;created_at:string}; export interface Repair{id:number;user_id:number;title:string;description:string;type:string;images:string;status:RepairStatus;handler_id?:number;user?:User;handler?:User;rating:number;facility_id?:number;facility?:Facility;source_task_id?:number;created_at:string;updated_at:string}; export interface Payment{id:number;user_id:number;fee_type:string;amount:number;month:string;status:string;paid_at?:string;created_at:string}; export interface Announcement{id:number;title:string;content:string;category:string;publisher_id:number;publisher?:User;publish_at:string;top:boolean;read_count:number}; export interface ApiResponse<T>{code:number;message:string;data:T}

// ---- 公共设施巡检与停用处置 ----
export type FacilityStatus='available'|'disabled';
export type InspectionCycle='daily'|'weekly'|'monthly'|'quarterly'|'yearly';
export type InspectionTaskKind='routine'|'recheck';
export type InspectionTaskStatus='pending'|'claimed'|'done'|'hazard'|'recheck_failed'|'restored';
export type InspectionResult='normal'|'hazard'|'pass'|'fail';

export interface Facility{
  id:number;name:string;category:string;location:string;status:FacilityStatus;
  remark:string;created_at:string;updated_at:string
}
export interface InspectionPlan{
  id:number;facility_id:number;facility?:Facility;name:string;cycle:InspectionCycle;
  start_date:string;active:boolean;created_at:string;updated_at:string
}
export interface InspectionTask{
  id:number;facility_id:number;facility?:Facility;plan_id?:number;plan?:InspectionPlan;
  kind:InspectionTaskKind;cycle:InspectionCycle;period_value:string;due_date:string;
  status:InspectionTaskStatus;inspector_id?:number;inspector?:User;
  result:InspectionResult|'';finding:string;
  source_repair_id?:number;source_repair?:Repair;hazard_repair_id?:number;hazard_repair?:Repair;
  reviewed_at?:string;created_at:string;updated_at:string
}
export interface FacilityDetail extends Facility{
  tasks:InspectionTask[];repairs:Repair[]
}
