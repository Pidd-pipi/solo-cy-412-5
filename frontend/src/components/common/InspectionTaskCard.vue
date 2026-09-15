<template>
  <article class="card inspection-card">
    <div class="inspection-main">
      <div class="inspection-title">
        <h3>{{ task.facility?.name || `设施 #${task.facility_id}` }}</h3>
        <el-tag size="small" effect="plain">{{ TASK_KIND_TEXT[task.kind] }}</el-tag>
        <InspectionTaskStatusBadge :status="task.status" />
      </div>
      <p>{{ CYCLE_TEXT[task.cycle] }} · 期次 {{ task.period_value }} · 到期 {{ fmt(task.due_date) }}</p>
      <p v-if="task.finding" class="inspection-finding">
        {{ task.kind === 'recheck' ? '复检结论' : '巡检记录' }}：{{ task.finding }}
      </p>
      <small v-if="task.hazard_repair">关联维修单 #{{ task.hazard_repair.id }}（{{ repairText(task.hazard_repair.status) }}）</small>
      <small v-else-if="task.source_repair">来源维修单 #{{ task.source_repair.id }}（{{ repairText(task.source_repair.status) }}）</small>
    </div>
    <div v-if="canAct" class="inspection-actions">
      <el-button v-if="task.status==='pending'" type="primary" @click="$emit('claim', task.id)">接单</el-button>
      <template v-if="task.kind==='routine' && actionable">
        <el-button type="success" @click="$emit('normal', task.id)">巡检正常</el-button>
        <el-button type="danger" @click="$emit('hazard', task.id)">发现隐患</el-button>
      </template>
      <template v-if="task.kind==='recheck' && actionable">
        <el-button type="success" @click="$emit('pass', task.id)">复检通过</el-button>
        <el-button type="danger" @click="$emit('fail', task.id)">复检未过</el-button>
      </template>
    </div>
  </article>
</template>
<script setup lang="ts">
import {computed} from 'vue';
import type {InspectionTask} from '../../types';
import {CYCLE_TEXT, TASK_KIND_TEXT} from '../../constants/inspection';
import {repairStatusText} from '../../constants/repair';
import InspectionTaskStatusBadge from './InspectionTaskStatusBadge.vue';

const props = defineProps<{task: InspectionTask}>();
defineEmits<{
  (e: 'claim', id: number): void
  (e: 'normal', id: number): void
  (e: 'hazard', id: number): void
  (e: 'pass', id: number): void
  (e: 'fail', id: number): void
}>();

const actionable = computed(() => props.task.status === 'pending' || props.task.status === 'claimed');
const canAct = computed(() =>
  props.task.status === 'pending' || props.task.status === 'claimed'
);
const fmt = (v: string) => new Date(v).toLocaleDateString();
const repairText = (s: string) => repairStatusText[s as keyof typeof repairStatusText] || s;
</script>
<style scoped>
.inspection-main{flex:1}
.inspection-title{display:flex;align-items:center;gap:10px}
.inspection-title h3{margin:0}
.inspection-finding{color:#b91c1c}
.inspection-actions{display:flex;flex-direction:column;gap:8px;justify-content:center;min-width:120px}
</style>
