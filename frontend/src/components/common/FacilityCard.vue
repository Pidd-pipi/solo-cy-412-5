<template>
  <article class="card facility-card" @click="$emit('open', facility.id)">
    <div>
      <h3>{{ facility.name }}</h3>
      <p>{{ facility.category }} · {{ facility.location }}</p>
      <small v-if="facility.remark">{{ facility.remark }}</small>
    </div>
    <div class="facility-card-side">
      <FacilityStatusBadge :status="facility.status" />
      <small v-if="openTasks > 0" class="facility-todo">{{ openTasks }} 项待巡检</small>
    </div>
  </article>
</template>
<script setup lang="ts">
import type {Facility} from '../../types';
import FacilityStatusBadge from './FacilityStatusBadge.vue';
withDefaults(defineProps<{facility: Facility; openTasks?: number}>(), {openTasks: 0});
defineEmits<{(e: 'open', id: number): void}>();
</script>
<style scoped>
.facility-card{cursor:pointer}
.facility-card-side{display:flex;flex-direction:column;align-items:flex-end;gap:8px;justify-content:center}
.facility-todo{color:#d97706}
</style>
