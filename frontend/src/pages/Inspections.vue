<template>
  <section>
    <header class="page-head">
      <div>
        <p class="eyebrow">INSPECTION WORKBENCH</p>
        <h2>巡检执行与停用处置</h2>
        <p>接单后提交巡检结论；发现隐患设施立即停用并生成唯一维修工单，复检通过才恢复可用。</p>
      </div>
      <el-button type="primary" @click="load">刷新</el-button>
    </header>

    <div class="toolbar inspection-filters">
      <el-radio-group v-model="scope" @change="load">
        <el-radio-button label="open">待处理</el-radio-button>
        <el-radio-button label="all">全部</el-radio-button>
      </el-radio-group>
      <el-select v-model="kind" placeholder="全部类型" clearable style="width:140px" @change="load">
        <el-option label="常规巡检" value="routine" />
        <el-option label="维修复检" value="recheck" />
      </el-select>
      <el-select v-model="taskStatus" placeholder="全部状态" clearable style="width:150px" @change="load">
        <el-option v-for="(t,k) in inspectionTaskStatusText" :key="k" :label="t" :value="k" />
      </el-select>
    </div>

    <div class="inspection-list">
      <InspectionTaskCard
        v-for="t in tasks" :key="t.id" :task="t"
        @claim="onClaim" @normal="onNormal" @hazard="openFinding('hazard',$event)"
        @pass="onPass" @fail="openFinding('fail',$event)" />
      <EmptyState v-if="!tasks.length" description="暂无巡检任务" />
    </div>

    <section v-if="openFacilityRepairs.length" class="repair-block">
      <h3>待完成的巡检维修单</h3>
      <p class="block-hint">标记完成后将自动安排一次复检（重复提交不会重复安排）。</p>
      <div v-for="r in openFacilityRepairs" :key="r.id" class="card repair-line">
        <div>
          <h3>#{{ r.id }} {{ r.title }}</h3>
          <small>{{ r.facility?.name }} · {{ repairStatusText[r.status] }}</small>
        </div>
        <el-button type="success" @click="completeRepair(r.id)">维修完成·安排复检</el-button>
      </div>
    </section>

    <!-- 隐患/复检未过说明 -->
    <el-dialog v-model="findingVisible" :title="findingKind==='hazard' ? '报告安全隐患（设施将停用）' : '复检未通过（设施维持停用）'" width="460px">
      <el-input v-model="finding" type="textarea" :rows="4"
        :placeholder="findingKind==='hazard' ? '请描述发现的安全隐患，将自动停用设施并生成维修工单' : '请说明仍未达标的问题，将续建维修工单'" />
      <template #footer>
        <el-button @click="findingVisible = false">取消</el-button>
        <el-button :type="findingKind==='hazard' ? 'danger' : 'warning'" @click="submitFinding">确认提交</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import {computed, ref, onMounted} from 'vue';
import {ElMessage, ElMessageBox} from 'element-plus';
import {
  listInspectionTasks, claimInspectionTask, submitRoutine, submitRecheck
} from '../api/inspectionTask';
import {listRepairs, updateRepairStatus} from '../api/repair';
import type {InspectionTask, InspectionTaskKind, InspectionTaskStatus, Repair} from '../types';
import {inspectionTaskStatusText} from '../constants/inspection';
import {repairStatusText} from '../constants/repair';
import InspectionTaskCard from '../components/common/InspectionTaskCard.vue';
import EmptyState from '../components/common/EmptyState.vue';

const tasks = ref<InspectionTask[]>([]);
const repairs = ref<Repair[]>([]);
const scope = ref<'open' | 'all'>('open');
const kind = ref<InspectionTaskKind | ''>('');
const taskStatus = ref<InspectionTaskStatus | ''>('');

const openFacilityRepairs = computed(() =>
  repairs.value.filter(r => r.facility_id && !['done', 'closed'].includes(r.status))
);

async function load() {
  tasks.value = await listInspectionTasks({
    open: scope.value === 'open',
    kind: kind.value || undefined,
    status: taskStatus.value || undefined
  });
  repairs.value = await listRepairs();
}

async function onClaim(id: number) {
  try {
    await claimInspectionTask(id);
    ElMessage.success('接单成功');
    await load();
  } catch (e) {
    ElMessage.error((e as Error).message);
  }
}

async function onNormal(id: number) {
  try {
    await ElMessageBox.confirm('确认该设施巡检正常？', '提交巡检', {type: 'success'});
  } catch {
    return;
  }
  try {
    await submitRoutine(id, 'normal', '');
    ElMessage.success('巡检已提交，设施保持可用');
    await load();
  } catch (e) {
    ElMessage.error((e as Error).message);
  }
}

async function onPass(id: number) {
  try {
    await submitRecheck(id, 'pass', '');
    ElMessage.success('复检通过，设施已恢复可用');
    await load();
  } catch (e) {
    ElMessage.error((e as Error).message);
  }
}

const findingVisible = ref(false);
const finding = ref('');
const findingKind = ref<'hazard' | 'fail'>('hazard');
const findingTaskId = ref(0);
function openFinding(kind: 'hazard' | 'fail', id: number) {
  findingKind.value = kind;
  findingTaskId.value = id;
  finding.value = '';
  findingVisible.value = true;
}
async function submitFinding() {
  if (!finding.value.trim()) {
    ElMessage.error('请填写问题说明');
    return;
  }
  try {
    if (findingKind.value === 'hazard') {
      await submitRoutine(findingTaskId.value, 'hazard', finding.value.trim());
      ElMessage.success('已停用设施并生成维修工单');
    } else {
      await submitRecheck(findingTaskId.value, 'fail', finding.value.trim());
      ElMessage.success('已记录复检未通过，设施维持停用并续建维修单');
    }
    findingVisible.value = false;
    await load();
  } catch (e) {
    ElMessage.error((e as Error).message);
  }
}

async function completeRepair(id: number) {
  try {
    await updateRepairStatus(id, 'done');
    ElMessage.success('维修已完成，已安排复检');
    await load();
  } catch (e) {
    ElMessage.error((e as Error).message);
  }
}

onMounted(load);
</script>
<style scoped>
.inspection-filters{display:flex;gap:12px;align-items:center}
.inspection-list{max-width:920px}
.repair-block{margin-top:30px;max-width:920px}
.block-hint{color:#8b97aa;font-size:13px;margin:0 0 12px}
.repair-line{align-items:center}
</style>
