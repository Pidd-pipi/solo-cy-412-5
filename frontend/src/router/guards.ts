import type {Router} from 'vue-router';
import {authStore, restoreSession} from '../stores/authStore';
import {hasPermission} from '../hooks/usePermission';
import type {PermissionCode} from '../types/permission';

export function installGuards(router: Router) {
  router.beforeEach(async to => {
    if (to.path !== '/login' && !authStore.token) return '/login';
    if (to.path === '/login' && authStore.token) return '/dashboard';
    // 硬刷新后先恢复用户资料，再做基于角色的路由权限判断。
    if (authStore.token && !authStore.user) {
      try {
        await restoreSession();
      } catch {
        return '/login';
      }
    }
    // 工作台对所有登录角色开放（保持历史行为）；其余页面按 meta.permission 拦截。
    if (to.path === '/dashboard') return true;
    const perm = to.meta?.permission as PermissionCode | undefined;
    if (perm && !hasPermission(authStore.user?.role, perm)) return '/dashboard';
    return true;
  });
}
