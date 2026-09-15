<template>
  <section>
    <header class="page-head">
      <div>
        <p class="eyebrow">WORKBENCH</p>
        <h2>物业工作台</h2>
        <p>掌握巡检停用处置、报修、收费和社区公告的最新动态。</p>
      </div>
      <el-button type="primary" @click="load">刷新数据</el-button>
    </header>

    <h3 class="section-title">巡检与停用处置</h3>
    <div class="stat-grid">
      <div class="stat-link" @click="go('/inspections')">
        <StatCard label="待巡检" :value="summary.pending_inspections" hint="到期常规巡检与维修复检" />
      </div>
      <div class="stat-link" @click="go('/facilities')">
        <StatCard label="停用设施" :value="summary.disabled_facilities" hint="待维修/复检恢复" />
      </div>
      <div class="stat-link" @click="go('/inspections')">
        <StatCard label="未闭环巡检工单" :value="summary.unclosed_facility_repairs" hint="隐患维修未完成闭环" />
      </div>
    </div>

    <h3 class="section-title">日常运营</h3>
    <div class="stat-grid">
      <StatCard label="待处理工单" :value="summary.pending_repairs" hint="需要物业跟进" />
      <StatCard label="本月已收费用" :value="`¥${Number(summary.monthly_paid||0).toFixed(2)}`" hint="支付宝沙箱模拟数据" />
      <StatCard label="最新公告" :value="summary.announcements?.length||0" hint="近期开启阅读" />
    </div>

    <section class="dashboard-block">
      <h3>最新公告</h3>
      <AnnouncementCard v-for="v in summary.announcements" :key="v.id" :announcement="v" @open="open" />
      <EmptyState v-if="!summary.announcements?.length" />
    </section>
  </section>
</template>
<script setup lang="ts">
import {reactive, onMounted} from 'vue';
import {useRouter} from 'vue-router';
import {request} from '../utils/request';
import type {Announcement} from '../types';
import StatCard from '../components/common/StatCard.vue';
import AnnouncementCard from '../components/common/AnnouncementCard.vue';
import EmptyState from '../components/common/EmptyState.vue';

const router = useRouter();
const summary = reactive<{
  pending_repairs: number; monthly_paid: number; announcements: Announcement[];
  pending_inspections: number; disabled_facilities: number; unclosed_facility_repairs: number;
}>({
  pending_repairs: 0, monthly_paid: 0, announcements: [],
  pending_inspections: 0, disabled_facilities: 0, unclosed_facility_repairs: 0
});
async function load() {
  Object.assign(summary, await request('/dashboard/summary'));
}
function go(path: string) {
  router.push(path);
}
function open(id: number) {
  location.hash = `#/announcements?open=${id}`;
}
onMounted(load);
</script>
<style scoped>
.section-title{margin:22px 0 14px;font-size:15px;color:#5b6980}
.section-title:first-of-type{margin-top:0}
.stat-link{cursor:pointer;border-radius:14px}
.stat-link:hover{transform:translateY(-2px);transition:transform .15s}
</style>
