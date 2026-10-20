<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import FilterBar from './components/FilterBar.vue'
import GameCard from './components/GameCard.vue'
import GameDialog from './components/GameDialog.vue'
import SyncDialog from './components/SyncDialog.vue'
import SyncDock from './components/SyncDock.vue'
import StatsView from './components/StatsView.vue'
import AuthGate from './components/AuthGate.vue'
import ThemeSwitch from './components/ThemeSwitch.vue'
import PlazaView from './components/PlazaView.vue'
import { ApiError, ackJob, getMe, getMeta, getStats, listEntries, listJobs, logoutAccount, setGardenPublic, type SyncJob } from './api'
import { readTheme, type ThemeId } from './theme'
import { emptyMeta, preset, wordsOf, type Entry, type Filters, type Medium, type Meta, type Stats, type Tag, type Via, type ViewId } from './types'

const view = ref<ViewId>('year')
const filters = ref<Filters>(preset('year'))
const entries = ref<Entry[]>([])
const meta = ref<Meta>(emptyMeta())
const stats = ref<Stats | null>(null)
const statsYear = ref(new Date().getFullYear())
const loading = ref(false)
const statsLoading = ref(false)
const error = ref('')
const orient = ref<'portrait' | 'landscape'>(localStorage.getItem('forking-paths-orient') === 'landscape' ? 'landscape' : 'portrait')
const savedSize = localStorage.getItem('forking-paths-card-size')
const cardSize = ref<'s' | 'm' | 'l'>(savedSize === 's' || savedSize === 'l' ? savedSize : 'm')
const theme = ref<ThemeId>(readTheme())
const username = ref('')
const gardenPublic = ref(false)
const plazaTick = ref(0)
const booting = ref(true)
const dialogOpen = ref(false)
const syncOpen = ref(false)
const jobs = ref<SyncJob[]>([])
let jobPoll = 0
const editing = ref<Entry | null>(null)
const collapsed = reactive<Record<string, boolean>>({})

let timer = 0
let ctrl: AbortController | null = null
let suppressFilterWatch = false

const currentPath = computed(() => meta.value.paths.find((item) => item.id === filters.value.pathId) ?? null)
const words = computed(() => wordsOf(currentPath.value))
const showPlatinum = computed(() => currentPath.value?.slug === 'games')
const runningKinds = computed(() => jobs.value.filter((job) => job.status === 'running').map((job) => job.kind))

const views = computed(() => [
  { id: 'plaza' as ViewId, name: '广场' },
  { id: 'all' as ViewId, name: '总览' },
  { id: 'month' as ViewId, name: `本月${words.value.doneWord}` },
  { id: 'year' as ViewId, name: `本年${words.value.doneWord}` },
  { id: 'playing' as ViewId, name: words.value.doingTitle },
  { id: 'stats' as ViewId, name: '统计' },
])

const groups = computed(() => {
  if (view.value === 'playing') return [{ key: words.value.doing, entries: entries.value }]
  const undone = words.value.undone
  const map = new Map<string, Entry[]>()
  for (const entry of entries.value) {
    const key = entry.finishDate ? entry.finishDate.slice(0, 4) : undone
    const list = map.get(key) ?? []
    list.push(entry)
    map.set(key, list)
  }
  return [...map.keys()]
    .sort((a, b) => {
      if (a === undone) return 1
      if (b === undone) return -1
      return b.localeCompare(a)
    })
    .map((key) => ({ key, entries: map.get(key) ?? [] }))
})

function setPath(pathId: number) {
  suppressFilterWatch = true
  filters.value = { ...filters.value, pathId, mediumIds: [] }
  suppressFilterWatch = false
  if (view.value === 'stats') {
    loadStats()
    return
  }
  window.clearTimeout(timer)
  entries.value = []
  fetchEntries()
}

function armJobPoll() {
  window.clearInterval(jobPoll)
  jobPoll = 0
  if (jobs.value.some((job) => job.status === 'running')) {
    jobPoll = window.setInterval(() => { void pullJobs() }, 2000)
  }
}

async function pullJobs() {
  const prev = jobs.value
  try {
    const next = await listJobs()
    const finished = next.some((job) => job.status !== 'running' && prev.some((item) => item.id === job.id && item.status === 'running'))
    jobs.value = next
    armJobPoll()
    if (finished) {
      if (view.value === 'stats') await loadStats()
      else await fetchEntries()
    }
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      username.value = ''
      jobs.value = []
      armJobPoll()
    }
  }
}

async function dismissJob(id: number) {
  jobs.value = jobs.value.filter((job) => job.id !== id)
  try {
    await ackJob(id)
  } catch {
    await pullJobs()
  }
}

function setView(id: ViewId) {
  view.value = id
  if (id === 'plaza') {
    plazaTick.value += 1
    return
  }
  if (id === 'stats') {
    loadStats()
    return
  }
  suppressFilterWatch = true
  filters.value = preset(id, filters.value.pathId)
  suppressFilterWatch = false
  window.clearTimeout(timer)
  entries.value = []
  fetchEntries()
}

function schedule() {
  window.clearTimeout(timer)
  timer = window.setTimeout(fetchEntries, 180)
}

async function fetchEntries() {
  ctrl?.abort()
  const mine = new AbortController()
  ctrl = mine
  loading.value = true
  try {
    const res = await listEntries(filters.value, mine.signal)
    entries.value = res.entries ?? []
    error.value = ''
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') return
    entries.value = []
    if (err instanceof ApiError && err.status === 401) {
      username.value = ''
      return
    }
    error.value = err instanceof Error && err.message !== 'Failed to fetch'
      ? err.message
      : '无法连接后端。请先启动数据库和 Go 服务。'
  } finally {
    if (ctrl === mine) loading.value = false
  }
}

async function loadStats() {
  statsLoading.value = true
  try {
    stats.value = await getStats(statsYear.value, filters.value.pathId)
    error.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : '统计加载失败'
  } finally {
    statsLoading.value = false
  }
}

watch(filters, () => {
  if (suppressFilterWatch || view.value === 'stats') return
  schedule()
}, { deep: true, flush: 'sync' })

watch(statsYear, () => {
  if (view.value === 'stats') loadStats()
})

watch(orient, (value) => {
  localStorage.setItem('forking-paths-orient', value)
})

watch(cardSize, (value) => {
  localStorage.setItem('forking-paths-card-size', value)
})

watch(theme, (value) => {
  localStorage.setItem('forking-paths-theme', value)
  document.documentElement.dataset.theme = value
})

async function togglePublic() {
  const next = !gardenPublic.value
  gardenPublic.value = next
  try {
    const res = await setGardenPublic(next)
    gardenPublic.value = res.public
    if (view.value === 'plaza') plazaTick.value += 1
  } catch (err) {
    gardenPublic.value = !next
    error.value = err instanceof Error ? err.message : '没有改成'
  }
}

function openCreate() {
  editing.value = null
  dialogOpen.value = true
}

function openEdit(entry: Entry) {
  editing.value = entry
  dialogOpen.value = true
}

async function onSaved() {
  dialogOpen.value = false
  await fetchEntries()
  if (view.value === 'stats') await loadStats()
}

function onTag(tag: Tag) {
  if (!meta.value.tags.some((item) => item.id === tag.id)) meta.value.tags.push(tag)
}

function onMedium(medium: Medium) {
  if (!meta.value.media.some((item) => item.id === medium.id)) meta.value.media.push(medium)
}

function onVia(via: Via) {
  if (!meta.value.vias.some((item) => item.id === via.id)) meta.value.vias.push(via)
}

async function enterGarden(name: string) {
  username.value = name
  error.value = ''
  try {
    const me = await getMe()
    gardenPublic.value = Boolean(me.public)
  } catch {
    gardenPublic.value = false
  }
  try {
    meta.value = await getMeta()
  } catch {
    error.value = '无法连接后端。请先启动数据库和 Go 服务。'
  }
  await fetchEntries()
  await pullJobs()
}

async function leaveGarden() {
  try {
    await logoutAccount()
  } catch {
    /* 会话清掉即可 */
  }
  username.value = ''
  gardenPublic.value = false
  entries.value = []
  stats.value = null
  jobs.value = []
  armJobPoll()
}

onMounted(async () => {
  try {
    const me = await getMe()
    await enterGarden(me.username)
  } catch (err) {
    if (!(err instanceof ApiError) || err.status !== 401) {
      error.value = '无法连接后端。请先启动数据库和 Go 服务。'
    }
  } finally {
    booting.value = false
  }
})

onUnmounted(() => window.clearInterval(jobPoll))
</script>

<template>
  <div v-if="booting" class="page"></div>
  <div v-else-if="!username" class="auth-wrap">
    <ThemeSwitch v-model="theme" class="auth-theme" />
    <AuthGate @authed="enterGarden" />
    <p v-if="error" class="banner auth-banner">{{ error }}</p>
  </div>
  <div v-else class="page">
    <div class="sticky">
      <header class="top">
        <div class="brand">
          <span class="logo">径</span>
          <div>
            <strong>小径分岔的花园</strong>
            <em>The Garden of Forking Paths</em>
          </div>
        </div>
        <nav v-if="view !== 'plaza'" class="paths">
          <button type="button" class="path-btn" :class="{ active: filters.pathId === 0 }" @click="setPath(0)">全部</button>
          <button
            v-for="path in meta.paths"
            :key="path.id"
            type="button"
            class="path-btn"
            :class="{ active: filters.pathId === path.id }"
            @click="setPath(path.id)"
          >{{ path.name }}</button>
        </nav>
        <div class="top-actions">
          <ThemeSwitch v-model="theme" />
          <input v-if="view !== 'stats' && view !== 'plaza'" v-model="filters.q" class="search" placeholder="搜索" />
          <span class="who">{{ username }}</span>
          <button type="button" class="tool-btn" :class="{ on: gardenPublic }" :aria-pressed="gardenPublic" @click="togglePublic">
            {{ gardenPublic ? '已公开' : '公开记录' }}
          </button>
          <button type="button" class="tool-btn" @click="leaveGarden">退出</button>
          <button type="button" class="tool-btn" @click="syncOpen = true">同步</button>
          <button type="button" class="new-btn" @click="openCreate">New <span class="caret">▾</span></button>
        </div>
      </header>
      <nav class="views">
        <button
          v-for="item in views"
          :key="item.id"
          type="button"
          class="view-btn"
          :class="{ active: view === item.id }"
          @click="setView(item.id)"
        >
          <svg v-if="item.id === 'plaza'" viewBox="0 0 16 16" class="ico"><circle cx="5" cy="5.5" r="2" fill="none" stroke="currentColor" stroke-width="1.3" /><circle cx="11" cy="5.5" r="2" fill="none" stroke="currentColor" stroke-width="1.3" /><path d="M1.8 13.2c.4-2 1.8-3 3.2-3s2.8 1 3.2 3M7.8 13.2c.4-2 1.8-3 3.2-3s2.8 1 3.2 3" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" /></svg>
          <svg v-else-if="item.id === 'all'" viewBox="0 0 16 16" class="ico"><rect x="2" y="2" width="5" height="5" rx="1" fill="currentColor" /><rect x="9" y="2" width="5" height="5" rx="1" fill="currentColor" /><rect x="2" y="9" width="5" height="5" rx="1" fill="currentColor" /><rect x="9" y="9" width="5" height="5" rx="1" fill="currentColor" /></svg>
          <svg v-else-if="item.id === 'month' || item.id === 'year'" viewBox="0 0 16 16" class="ico"><rect x="2" y="3" width="12" height="11" rx="1.5" fill="none" stroke="currentColor" stroke-width="1.3" /><path d="M2 6.5h12M5 2v3M11 2v3" stroke="currentColor" stroke-width="1.3" /></svg>
          <svg v-else-if="item.id === 'playing'" viewBox="0 0 16 16" class="ico"><circle cx="8" cy="8" r="5.2" fill="none" stroke="currentColor" stroke-width="1.3" /><path d="M8 5.2V8l2 1.4" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" /></svg>
          <svg v-else viewBox="0 0 16 16" class="ico"><path d="M3 12V8M6.5 12V5M10 12V7M13.5 12V3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" /></svg>
          {{ item.name }}
        </button>
        <div v-if="view !== 'stats'" class="view-tools">
          <div class="orient" role="group" aria-label="卡片大小">
            <button type="button" :class="{ active: cardSize === 's' }" aria-label="小卡片" @click="cardSize = 's'">小</button>
            <button type="button" :class="{ active: cardSize === 'm' }" aria-label="中卡片" @click="cardSize = 'm'">中</button>
            <button type="button" :class="{ active: cardSize === 'l' }" aria-label="大卡片" @click="cardSize = 'l'">大</button>
          </div>
          <div class="orient">
            <button type="button" :class="{ active: orient === 'portrait' }" aria-label="竖版" @click="orient = 'portrait'">
              <svg viewBox="0 0 16 16" class="ico"><rect x="4.5" y="2" width="7" height="12" rx="1.4" fill="none" stroke="currentColor" stroke-width="1.4" /></svg>
              竖版
            </button>
            <button type="button" :class="{ active: orient === 'landscape' }" aria-label="横版" @click="orient = 'landscape'">
              <svg viewBox="0 0 16 16" class="ico"><rect x="2" y="4.5" width="12" height="7" rx="1.4" fill="none" stroke="currentColor" stroke-width="1.4" /></svg>
              横版
            </button>
          </div>
        </div>
      </nav>
      <FilterBar v-if="view !== 'stats' && view !== 'plaza'" :filters="filters" :meta="meta" :words="words" @update="filters = $event" />
    </div>

    <p v-if="error" class="banner">{{ error }}</p>

    <PlazaView
      v-if="view === 'plaza'"
      :tick="plazaTick"
      @error="error = $event"
    />

    <StatsView
      v-else-if="view === 'stats'"
      :stats="stats"
      :year="statsYear"
      :loading="statsLoading"
      :words="words"
      :show-platinum="showPlatinum"
      @year="statsYear = $event"
    />

    <template v-else>
      <div v-if="loading && !entries.length" class="grid" :class="[orient, `size-${cardSize}`]">
        <div v-for="i in 8" :key="i" class="skeleton"></div>
      </div>
      <div v-else-if="!entries.length && !error" class="empty">
        <p>这条小径上还没有记录</p>
        <button type="button" class="text-btn" @click="setView(view)">恢复这个视图的默认筛选</button>
      </div>
      <div v-else :class="{ dim: loading && entries.length }">
        <section v-for="group in groups" :key="group.key">
          <button type="button" class="group-head" @click="collapsed[group.key] = !collapsed[group.key]">
            <svg viewBox="0 0 16 16" class="chev" :class="{ folded: collapsed[group.key] }"><path d="M4 6.5 8 10.5 12 6.5" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" /></svg>
            {{ group.key }}
            <span class="count">{{ group.entries.length }}</span>
          </button>
          <div v-show="!collapsed[group.key]" class="grid" :class="[orient, `size-${cardSize}`]">
            <GameCard
              v-for="entry in group.entries"
              :key="entry.id"
              :entry="entry"
              :orient="orient"
              @open="openEdit(entry)"
            />
          </div>
        </section>
      </div>
    </template>

    <SyncDock :jobs="jobs" @ack="dismissJob" />
    <SyncDialog :open="syncOpen" :running="runningKinds" @close="syncOpen = false" @started="pullJobs" />
    <GameDialog
      :open="dialogOpen"
      :entry="editing"
      :meta="meta"
      :default-path-id="filters.pathId"
      @close="dialogOpen = false"
      @saved="onSaved"
      @tag="onTag"
      @medium="onMedium"
      @via="onVia"
    />
  </div>
</template>
