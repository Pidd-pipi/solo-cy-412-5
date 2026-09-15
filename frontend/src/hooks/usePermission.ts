import {computed} from 'vue';
import {authStore} from '../stores/authStore';
import type {PermissionCode} from '../types/permission';

export const rolePermissionMap: Record<string, PermissionCode[]> = {
  resident: [],
  staff: ['repair:manage', 'payment:manage', 'announcement:publish', 'inspection:manage'],
  admin: ['repair:manage', 'payment:manage', 'announcement:publish', 'log:read', 'inspection:manage']
};

export function hasPermission(role: string | undefined, code: PermissionCode): boolean {
  return rolePermissionMap[role || 'resident']?.includes(code) || false;
}

export function usePermission(code: PermissionCode) {
  return computed(() => hasPermission(authStore.user?.role, code));
}
