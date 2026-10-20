<script setup lang="ts">
import type { SyncJob } from '../api'

defineProps<{ jobs: SyncJob[] }>()
const emit = defineEmits<{ ack: [id: number] }>()

const names: Record<string, string> = {
  steam: 'Steam',
  psn: 'PSN',
  xbox: 'Xbox',
  switch: 'Switch',
  covers: '封面',
}

function title(kind: string) {
  return names[kind] ?? kind
}

function width(job: SyncJob) {
  if (job.total <= 0) return '0%'
  return `${Math.min(100, Math.round((job.done / job.total) * 100))}%`
}
</script>

<template>
  <aside v-if="jobs.length" class="sync-dock" aria-live="polite">
    <article v-for="job in jobs" :key="job.id" class="sync-note">
      <header>
        <strong>{{ title(job.kind) }}</strong>
        <span v-if="job.status === 'running' && job.total > 0">{{ job.done }} / {{ job.total }}</span>
      </header>
      <p>{{ job.message }}</p>
      <div v-if="job.status === 'running'" class="meter" :class="{ pending: job.total <= 0 }">
        <span :style="job.total > 0 ? { width: width(job) } : undefined"></span>
      </div>
      <button v-else type="button" class="tool-btn" @click="emit('ack', job.id)">我知道了</button>
    </article>
  </aside>
</template>
