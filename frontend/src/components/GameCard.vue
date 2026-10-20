<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Entry } from '../types'
import { statusLabel, wordsOf } from '../types'
import { fmtDate } from '../format'
import CoverArt from './CoverArt.vue'
import Stars from './Stars.vue'

const props = defineProps<{ entry: Entry; orient: 'portrait' | 'landscape' }>()
defineEmits<{ open: [] }>()

const broken = ref(false)
const shownCover = computed(() =>
  props.orient === 'landscape' && props.entry.coverLandscapeUrl
    ? props.entry.coverLandscapeUrl
    : props.entry.coverUrl,
)
watch(shownCover, () => { broken.value = false })

const words = computed(() => wordsOf(props.entry.path))

function brief(mins: number) {
  const h = Math.floor(mins / 60)
  const m = mins % 60
  if (h <= 0) return `${m} 分`
  if (m === 0) return `${h} 小时`
  return `${h} 小时 ${m} 分`
}

function shortLabel(label: string) {
  return label.replace(/(时长|用时)$/, '')
}

const eyebrow = computed(() => {
  const bits: string[] = []
  if (props.entry.medium) bits.push(props.entry.medium.name)
  if (props.entry.finishDate) bits.push(fmtDate(props.entry.finishDate))
  else bits.push(statusLabel(props.entry.status, words.value))
  return bits.join(' · ')
})

const completionBadge = computed(() =>
  props.entry.badges.find((badge) => badge === '全收集' || badge === '全成就') ?? '',
)
const honor = computed(() => {
  if (props.entry.badges.includes('白金')) return 'platinum'
  if (!completionBadge.value) return ''
  if ((props.entry.medium?.name ?? '').toLowerCase() === 'xbox') return 'xbox'
  return 'completion'
})
const progress = computed(() => {
  const total = props.entry.progressTotal
  const earned = props.entry.progressEarned
  if (!total || total <= 0 || earned == null) return null
  const pct = Math.max(0, Math.min(100, Math.round((earned * 100) / total)))
  return { earned, total, pct }
})
const marks = computed(() =>
  props.entry.badges.filter((badge) => badge !== '白金' && badge !== '全收集' && badge !== '全成就'),
)
</script>

<template>
  <div class="card-slot" :class="[entry.path.slug, honor]" @click="$emit('open')">
    <div v-if="honor === 'platinum'" class="medal platinum" aria-label="白金">
      <img src="/platinum.png" alt="" />
    </div>
    <div v-else-if="honor" class="medal" :class="honor" :aria-label="completionBadge">
      <img :src="honor === 'xbox' ? '/xbox.png' : '/steam.png'" alt="" />
    </div>
    <article class="card" :class="[entry.path.slug, honor]">
    <div class="card-cover">
      <img
        v-if="shownCover && !broken"
        :src="shownCover"
        :alt="entry.title"
        @error="broken = true"
      />
      <CoverArt v-else :cover="entry.cover" />
    </div>
    <div class="card-veil"></div>
    <div class="card-copy">
      <div class="card-title">
        <h3>{{ entry.title }}</h3>
        <Stars v-if="entry.rating != null" :value="entry.rating" />
      </div>
      <p v-if="eyebrow" class="card-line">{{ eyebrow }}</p>
      <p v-if="entry.spendMinutes || entry.finishMinutes != null" class="card-times">
        <span v-if="entry.finishMinutes != null">{{ shortLabel(words.finishTime) }} {{ brief(entry.finishMinutes) }}</span>
        <span v-if="entry.spendMinutes">累计 {{ brief(entry.spendMinutes) }}</span>
      </p>
      <p v-if="progress" class="card-progress" :aria-label="`完成度 ${progress.pct}%`">
        <span class="meter"><span :style="{ width: progress.pct + '%' }"></span></span>
        <span>{{ progress.pct }}% · {{ progress.earned }}/{{ progress.total }}</span>
      </p>
      <p v-if="marks.length" class="card-marks">{{ marks.join(' · ') }}</p>
    </div>
    </article>
  </div>
</template>
