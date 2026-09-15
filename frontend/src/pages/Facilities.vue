<template>
  <section>
    <header class="page-head">
      <div>
        <p class="eyebrow">FACILITY CENTER</p>
        <h2>公共设施与巡检处置</h2>
        <p>按设施与周期管理巡检计划，跟踪停用、维修与复检恢复进度。</p>
      </div>
      <div class="head-actions">
        <el-button @click="generate">到期生成任务</el-button>
        <el-button type="primary" @click="facilityDialog = true">新增设施</el-button>
      </div>
    </header>

    <div class="toolbar">
      <el-select v-model="status" placeholder="全部状态" clearable style="width:160px" @change="load">
        <el-option label="可用" value="available" />
        <el-option label="停用" value="disabled" />
      </el-select>
    </div>

    <div class="facility-list">
      <FacilityCard
        v-for="v in facilities" :key="v.id" :facility="v"
        :open-tasks="openCount[v.id] || 0" @open="openDetail" />
      <EmptyState v-if="!facilities.length" description="暂无设施" />
    </div>

    <!-- 新增设施 -->
    <el-dialog v-model="facilityDialog" title="新增公共设施" width="460px">
      <el-form label-width="72px">
        <el-form-item label="名称"><el-input v-model="facilityForm.name" placeholder="如 1号楼电梯" /></el-form-item>
        <el-form-item label="类别">
          <el-select v-model="facilityForm.category" placeholder="选择类别">
            <el-option v-for="c in categories" :key="c" :label="c" :value="c" />
          </el-select>
        </el-form-item>
        <el-form-item label="位置"><el-input v-model="facilityForm.location" placeholder="如 1号楼1单元" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="facilityForm.remark" type="textarea" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="facilityDialog = false">取消</el-button>
        <el-button type="primary" @click="submitFacility">保存</el-button>
      </template>
    </el-dialog>

    <!-- 设施详情与处置进度 -->
    <el-dialog v-model="detailVisible" :title="detail?.name" width="680px" top="6vh">
      <div v-if="detail" class="detail">
        <div class="detail-head">
          <FacilityStatusBadge :status="detail.status" />
          <span>{{ detail.category }} · {{ detail.location }}</span>
        </div>
        <p v-if="detail.remark" class="detail-remark">{{ detail.remark }}</p>

        <section class="detail-block">
          <div class="detail-block-head">
            <h4>巡检计划</h4>
            <el-button link type="primary" @click="planDialog = true">+ 新建计划</el-button>
          </div>
          <div v-for="p in facilityPlans" :key="p.id" class="plan-row">
            <span>{{ p.name }}</span>
            <el-tag size="small" effect="plain">{{ CYCLE_TEXT[p.cycle] }}</el-tag>
            <small>{{ new Date(p.start_date).toLocaleDateString() }} 起 · {{ p.active ? '启用中' : '已停用' }}</small>
          </div>
          <EmptyState v-if="!facilityPlans.length" description="尚无巡检计划" />
        </section>

        <section class="detail-block">
          <h4>巡检任务</h4>
          <div v-for="t in detail.tasks" :key="t.id" class="progress-row">
            <InspectionTaskStatusBadge :status="t.status" />
            <span class="grow">{{ TASK_KIND_TEXT[t.kind] }} · {{ t.period_value }}</span>
            <small>{{ t.inspector?.nickname || '未接单' }}</small>
          </div>
          <EmptyState v-if="!detail.tasks.length" description="尚无巡检任务" />
        </section>

        <section class="detail-block">
          <h4>关联维修工单（处置进度）</h4>
          <div v-for="r in detail.repairs" :key="r.id" class="progress-row">
            <RepairStatusBadge :status="r.status" />
            <span class="grow">#{{ r.id }} {{ r.title }}</span>
            <small>{{ r.handler?.nickname || '待分派' }}</small>
          </div>
          <EmptyState v-if="!detail.repairs.length" description="暂无关联维修工单" />
        </section>
      </div>
    </el-dialog>

    <!-- 新建巡检计划 -->
    <el-dialog v-model="planDialog" title="新建巡检计划" width="420px" append-to-body>
      <el-form label-width="72px">
        <el-form-item label="计划名称"><el-input v-model="planForm.name" placeholder="如 电梯月度巡检" /></el-form-item>
        <el-form-item label="周期">
          <el-select v-model="planForm.cycle" placeholder="选择周期">
            <el-option v-for="(t,k) in CYCLE_TEXT" :key="k" :label="t" :value="k" />
          </el-select>
        </el-form-item>
        <el-form-item label="开始日期"><el-date-picker v-model="planStart" type="date" value-format="YYYY-MM-DD" placeholder="默认今天" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="planDialog = false">取消</el-button>
        <el-button type="primary" @click="submitPlan">保存</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import {computed, ref, onMounted} from 'vue';
import {ElMessage} from 'element-plus';
import {listFacilities, detailFacility, createFacility, type FacilityPayload} from '../api/facility';
import {listInspectionPlans, createInspectionPlan, generateInspectionTasks} from '../api/inspectionPlan';
import {listInspectionTasks} from '../api/inspectionTask';
import type {Facility, FacilityDetail, FacilityStatus, InspectionCycle, InspectionPlan, InspectionTask} from '../types';
import {CYCLE_TEXT, TASK_KIND_TEXT} from '../constants/inspection';
import FacilityCard from '../components/common/FacilityCard.vue';
import FacilityStatusBadge from '../components/common/FacilityStatusBadge.vue';
import InspectionTaskStatusBadge from '../components/common/InspectionTaskStatusBadge.vue';
import RepairStatusBadge from '../components/common/RepairStatusBadge.vue';
import EmptyState from '../components/common/EmptyState.vue';

const categories = ['电梯', '消防', '配电', '给排水', '健身器材', '监控', '门禁', '其他'];

const facilities = ref<Facility[]>([]);
const status = ref<FacilityStatus | ''>('');
const openTasks = ref<InspectionTask[]>([]);
const openCount = computed(() => {
  const m: Record<number, number> = {};
  openTasks.value.forEach(t => { m[t.facility_id] = (m[t.facility_id] || 0) + 1; });
  return m;
});

const facilityDialog = ref(false);
const facilityForm = ref<FacilityPayload>({name: '', category: '电梯', location: '', remark: ''});

const detailVisible = ref(false);
const detail = ref<FacilityDetail | null>(null);
const plans = ref<InspectionPlan[]>([]);
const facilityPlans = computed(() => plans.value.filter(p => p.facility_id === detail.value?.id));

const planDialog = ref(false);
const planForm = ref<{name: string; cycle: InspectionCycle}>({name: '', cycle: 'monthly'});
const planStart = ref('');

async function load() {
  facilities.value = await listFacilities(status.value);
  openTasks.value = await listInspectionTasks({open: true});
  plans.value = await listInspectionPlans();
}

async function openDetail(id: number) {
  detail.value = await detailFacility(id);
  detailVisible.value = true;
}

async function submitFacility() {
  try {
    if (!facilityForm.value.name || !facilityForm.value.location) {
      ElMessage.error('名称与位置必填');
      return;
    }
    await createFacility(facilityForm.value);
    ElMessage.success('设施已创建');
    facilityDialog.value = false;
    facilityForm.value = {name: '', category: '电梯', location: '', remark: ''};
    await load();
  } catch (e) {
    ElMessage.error((e as Error).message);
  }
}

async function submitPlan() {
  if (!detail.value) return;
  try {
    await createInspectionPlan({
      facility_id: detail.value.id,
      name: planForm.value.name || `${detail.value.name}巡检`,
      cycle: planForm.value.cycle,
      start_date: planStart.value || undefined
    });
    ElMessage.success('巡检计划已创建');
    planDialog.value = false;
    planForm.value = {name: '', cycle: 'monthly'};
    planStart.value = '';
    await load();
    await openDetail(detail.value.id);
  } catch (e) {
    ElMessage.error((e as Error).message);
  }
}

async function generate() {
  try {
    const r = await generateInspectionTasks();
    ElMessage.success(r.generated > 0 ? `已生成 ${r.generated} 项到期任务` : '没有新的到期任务（重跑未重复生成）');
    await load();
  } catch (e) {
    ElMessage.error((e as Error).message);
  }
}

onMounted(load);
</script>
<style scoped>
.head-actions{display:flex;gap:10px}
.facility-list{max-width:860px}
.detail-head{display:flex;align-items:center;gap:12px;margin-bottom:6px}
.detail-remark{color:#65728a}
.detail-block{margin-top:18px;border-top:1px dashed #e2e8f2;padding-top:12px}
.detail-block-head{display:flex;justify-content:space-between;align-items:center}
.detail-block h4{margin:0 0 10px}
.plan-row,.progress-row{display:flex;align-items:center;gap:10px;padding:7px 0}
.plan-row small,.progress-row small{color:#8b97aa}
.grow{flex:1}
</style>
