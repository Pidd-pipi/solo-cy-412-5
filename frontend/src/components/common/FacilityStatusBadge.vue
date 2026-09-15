<template>
  <el-tag :type="kind" effect="dark">{{ facilityStatusText[status] }}</el-tag>
</template>
<script setup lang="ts">
import {computed} from 'vue';
import {facilityStatusKind, facilityStatusText} from '../../constants/inspection';
import type {FacilityStatus} from '../../types';
const props = defineProps<{status: FacilityStatus}>();
const kind = computed(() => facilityStatusKind[props.status]);
</script>
