<template>
  <router-view v-if="$route.path==='/login'"/>
  <div v-else class="shell">
    <aside>
      <div class="brand">
        <span>SE</span>
        <div><b>SmartEstate</b><small>智慧社区物业</small></div>
      </div>
      <nav>
        <RouterLink to="/dashboard">工作台</RouterLink>
        <RouterLink v-if="canRepair" to="/repairs">报修管理</RouterLink>
        <RouterLink v-if="canInspection" to="/facilities">设施台账</RouterLink>
        <RouterLink v-if="canInspection" to="/inspections">巡检处置</RouterLink>
        <RouterLink to="/payments">费用缴纳</RouterLink>
        <RouterLink v-if="canAnnounce" to="/announcements">社区公告</RouterLink>
        <RouterLink to="/profile">个人中心</RouterLink>
      </nav>
      <div class="account">
        <el-avatar>{{authStore.user?.nickname?.slice(0,1)}}</el-avatar>
        <div>
          <b>{{authStore.user?.nickname}}</b>
          <small>{{roleText[authStore.user?.role||'resident']}}</small>
        </div>
        <el-button link @click="logout">退出</el-button>
      </div>
    </aside>
    <main class="content"><router-view/></main>
  </div>
</template>
<script setup lang="ts">
import {onMounted} from 'vue';
import {useRouter} from 'vue-router';
import {authStore, signOut, restoreSession} from './stores/authStore';
import {roleText} from './utils/roleText';
import {usePermission} from './hooks/usePermission';

const router = useRouter();
const canRepair = usePermission('repair:manage');
const canInspection = usePermission('inspection:manage');
const canAnnounce = usePermission('announcement:publish');
onMounted(restoreSession);
function logout() {
  signOut();
  router.push('/login');
}
</script>
