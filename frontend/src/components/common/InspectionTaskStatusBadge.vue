<template>
  <el-tag :type="kind" effect="light">{{ inspectionTaskStatusText[status] }}</el-tag>
</template>
<script setup lang="ts">
import {computed} from 'vue';
import {inspectionTaskStatusKind, inspectionTaskStatusText} from '../../constants/inspection';
import type {InspectionTaskStatus} from '../../types';
const p = defineProps<{status: InspectionTaskStatus}>();
const kind = computed(() => inspectionTaskStatusKind[p.status]);
</script>
