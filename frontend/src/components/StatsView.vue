<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { Stats, Words } from '../types'
import { fmtDate, fmtDur } from '../format'
import Stars from './Stars.vue'

const props = defineProps<{ stats: Stats | null; year: number; loading: boolean; words: Words; showPlatinum: boolean }>()
const emit = defineEmits<{ year: [year: number] }>()

const years = computed(() => {
  const current = new Date().getFullYear()
  return [current, current - 1, current - 2]
})

const cloudInks = ['#7c3aed', '#16a34a', '#ca8a04', '#dc2626', '#5d8f72']
const leaf = ref('#6eae86')
let themeWatch: MutationObserver | null = null
function readLeaf() {
  leaf.value = getComputedStyle(document.documentElement).getPropertyValue('--leaf').trim() || '#6eae86'
}
onMounted(() => {
  readLeaf()
  themeWatch = new MutationObserver(readLeaf)
  themeWatch.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
})
onBeforeUnmount(() => themeWatch?.disconnect())
const accent = computed(() => (props.stats?.byPath.length === 1 ? props.stats.byPath[0].color : leaf.value))
const maxMonth = computed(() => Math.max(1, ...(props.stats?.byMonth.map((item) => item.count) ?? [1])))
const maxMedium = computed(() => Math.max(1, ...(props.stats?.byMedium.map((item) => item.count) ?? [1])))
const maxVia = computed(() => Math.max(1, ...(props.stats?.byVia.map((item) => item.count) ?? [1])))

const lead = computed(() => {
  const stats = props.stats
  if (!stats) return ''
  if (stats.done === 0) return `这一年还没有${props.words.doneWord}。`
  const head = `这一年，${props.words.doneWord} ${stats.done}，累计 ${fmtDur(stats.spendMinutes)}。`
  const diff = stats.done - stats.prevDone
  if (stats.prevDone === 0) return `${head}去年还没有。`
  if (diff === 0) return `${head}和去年一样多。`
  return `${head}比去年${diff > 0 ? '多' : '少'} ${Math.abs(diff)}。`
})

const keyword = computed(() => props.stats?.topTags[0] ?? null)

const keywordColor = computed(() => {
  const name = keyword.value?.name ?? ''
  if (!name) return accent.value
  if (props.stats?.byPath.length === 1) return accent.value
  const seed = [...name].reduce((sum, char) => sum + char.charCodeAt(0), 0)
  return cloudInks[seed % cloudInks.length]
})

const paceDays = computed(() => {
  const stats = props.stats
  if (!stats || stats.done < 2) return 0
  const leap = new Date(props.year, 1, 29).getMonth() === 1
  const yearDays = leap ? 366 : 365
  const now = new Date()
  const elapsed = props.year === now.getFullYear()
    ? Math.min(yearDays, Math.floor((now.getTime() - new Date(props.year, 0, 1).getTime()) / 86400000) + 1)
    : yearDays
  return Math.max(1, Math.round(elapsed / stats.done))
})

const spendDelta = computed(() => {
  const stats = props.stats
  if (!stats || stats.prevSpend <= 0) return ''
  const diff = stats.spendMinutes - stats.prevSpend
  if (diff === 0) return '时长和去年一样'
  const way = diff > 0 ? '多' : '少'
  return `时长比去年${way} ${fmtDur(Math.abs(diff))}`
})

const peakMonth = computed(() => {
  const months = props.stats?.byMonth ?? []
  const peak = months.reduce((best, item) => (item.count > best.count ? item : best), months[0])
  return peak && peak.count > 0 ? peak : null
})

const maxRank = computed(() => Math.max(1, ...(props.stats?.ranking.map((item) => item.minutes) ?? [1])))

const mixTitle = computed(() => (props.stats && props.stats.byPath.length > 1 ? '各条小径' : props.words.medium))

const mix = computed(() => {
  const stats = props.stats
  if (!stats) return []
  const source = stats.byPath.length > 1 ? stats.byPath : stats.byMedium
  const total = source.reduce((sum, item) => sum + item.count, 0)
  if (!total) return []
  const gap = source.length > 1 ? 1.1 : 0
  let cursor = 0
  const rows = source.map((item) => {
    const full = (item.count / total) * 100
    const pct = Math.max(0.4, full - gap)
    const row = {
      name: item.name,
      color: item.color || accent.value,
      count: item.count,
      share: Math.round(full),
      pct,
      start: cursor,
    }
    cursor += full
    return row
  })
  return rows
})

const heatMonths = computed(() => {
  const cells = heatCells.value
  const weeks = cells.length / 7
  const labels = Array.from({ length: weeks }, () => '')
  let lastMonth = -1
  let lastLabeled = -4
  for (let week = 0; week < weeks; week++) {
    const dated = cells.slice(week * 7, week * 7 + 7).find((cell) => cell.date)
    if (!dated) continue
    const month = Number(dated.date.slice(5, 7))
    if (month !== lastMonth && week - lastLabeled >= 3) {
      labels[week] = `${month}月`
      lastLabeled = week
    }
    lastMonth = month
  }
  return labels
})

function shareOf(minutes: number) {
  const total = props.stats?.spendMinutes ?? 0
  if (!total || minutes <= 0) return ''
  return `占今年 ${Math.max(1, Math.round((minutes / total) * 100))}%`
}

const heatCells = computed(() => {
  const year = props.year
  const marks = new Map((props.stats?.days ?? []).map((item) => [item.date, item.count]))
  const start = new Date(year, 0, 1)
  const pad = (start.getDay() + 6) % 7
  const total = new Date(year, 1, 29).getMonth() === 1 ? 366 : 365
  const cells: { key: string; date: string; count: number }[] = []
  for (let i = 0; i < pad; i++) cells.push({ key: `pad-${i}`, date: '', count: 0 })
  for (let i = 0; i < total; i++) {
    const date = new Date(year, 0, 1 + i)
    const iso = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
    cells.push({ key: iso, date: iso, count: marks.get(iso) ?? 0 })
  }
  while (cells.length % 7) cells.push({ key: `tail-${cells.length}`, date: '', count: 0 })
  return cells
})

function monthHeight(count: number) {
  return `${Math.round((count / maxMonth.value) * 100)}%`
}
function barWidth(count: number, max: number) {
  return `${Math.round((count / max) * 100)}%`
}
const cloudEl = ref<HTMLElement | null>(null)
const cloudBox = ref({ w: 360, h: 196 })
let cloudObserver: ResizeObserver | null = null

watch(cloudEl, (el) => {
  cloudObserver?.disconnect()
  cloudObserver = null
  if (!el) return
  const measure = () => {
    cloudBox.value = { w: Math.max(1, el.clientWidth), h: Math.max(1, el.clientHeight) }
  }
  cloudObserver = new ResizeObserver(measure)
  cloudObserver.observe(el)
  measure()
})
onBeforeUnmount(() => cloudObserver?.disconnect())

const cloudWords = computed(() => {
  const tags = [...(props.stats?.topTags ?? [])].sort((a, b) => b.count - a.count || a.name.localeCompare(b.name, 'zh'))
  const max = Math.max(1, ...tags.map((item) => item.count))
  const boxW = cloudBox.value.w
  const boxH = cloudBox.value.h
  const rects: { x: number; y: number; w: number; h: number }[] = []
  const single = props.stats?.byPath.length === 1
  return tags.map((tag) => {
    const ratio = tag.count / max
    const size = 13 + Math.round(Math.sqrt(ratio) * 18)
    const seed = [...tag.name].reduce((sum, char) => sum + char.charCodeAt(0), 0)
    const rotate = ratio > 0.75 ? 0 : ((seed % 5) - 2) * 8
    const w = tag.name.length * size + 12
    const h = size + 10
    let angle = (seed % 12) * 0.35
    let radius = 0
    let x = Math.max(4, (boxW - w) / 2)
    let y = Math.max(4, (boxH - h) / 2)
    for (let step = 0; step < 520; step++) {
      const cx = boxW / 2 + Math.cos(angle) * radius
      const cy = boxH / 2 + Math.sin(angle) * radius * 0.72
      const nx = cx - w / 2
      const ny = cy - h / 2
      const inside = nx >= 4 && ny >= 4 && nx + w <= boxW - 4 && ny + h <= boxH - 4
      const hit = rects.some((rect) => nx < rect.x + rect.w && nx + w > rect.x && ny < rect.y + rect.h && ny + h > rect.y)
      if (inside && !hit) {
        x = nx
        y = ny
        break
      }
      angle += 0.45
      radius += 1.15
    }
    rects.push({ x, y, w, h })
    return {
      name: tag.name,
      count: tag.count,
      style: {
        left: `${x}px`,
        top: `${y}px`,
        fontSize: `${size}px`,
        fontWeight: 560 + Math.round(ratio * 140),
        color: single ? accent.value : cloudInks[seed % cloudInks.length],
        opacity: 0.72 + ratio * 0.28,
        transform: `rotate(${rotate}deg)`,
      },
    }
  })
})
function heatLevel(count: number) {
  if (count >= 3) return 3
  return count
}
</script>

<template>
  <section class="stats" :style="stats?.byPath.length === 1 ? { '--heat': accent } : undefined">
    <div class="stats-toolbar">
      <div v-if="stats" class="report-hero">
        <div v-if="keyword" class="keyword">
          <span>年度关键词</span>
          <strong :style="{ color: keywordColor }">{{ keyword.name }}</strong>
          <em>出现 {{ keyword.count }} 次</em>
        </div>
        <div class="hero-copy">
          <p class="lead">{{ lead }}</p>
          <div v-if="stats.done" class="deltas">
            <span v-if="paceDays">大约每 {{ paceDays }} 天{{ words.doneWord }}一次</span>
            <span v-if="spendDelta">{{ spendDelta }}</span>
            <span v-if="peakMonth">{{ peakMonth.month }} 月最密，{{ peakMonth.count }} 次</span>
          </div>
        </div>
      </div>
      <h2 v-else>{{ year }} 年</h2>
      <select :value="year" @change="emit('year', Number(($event.target as HTMLSelectElement).value))">
        <option v-for="item in years" :key="item" :value="item">{{ item }}</option>
      </select>
    </div>
    <p v-if="loading && !stats" class="hint">正在统计…</p>
    <template v-else-if="stats">
      <div class="metrics metrics-main">
        <article><span>本年{{ words.doneWord }}</span><strong>{{ stats.done }}</strong></article>
        <article><span>{{ words.spend }}</span><strong>{{ fmtDur(stats.spendMinutes) }}</strong></article>
        <article><span>抵达日</span><strong>{{ stats.activeDays }}</strong></article>
        <article><span>平均评分</span><strong>{{ stats.avgRating ?? '—' }}</strong></article>
      </div>
      <div class="metrics metrics-quiet">
        <article><span>平均每次</span><strong>{{ stats.done ? fmtDur(stats.avgSpend) : '—' }}</strong></article>
        <article v-if="showPlatinum"><span>白金</span><strong>{{ stats.platinum }}</strong></article>
        <article v-else><span>留痕</span><strong>{{ stats.marked }}</strong></article>
        <article><span>{{ words.doingTitle }}</span><strong>{{ stats.doing }}</strong></article>
        <article><span>{{ words.want }}</span><strong>{{ stats.want }}</strong></article>
      </div>

      <div v-if="stats.longest || stats.favorite" class="spotlights">
        <article v-if="stats.longest" class="spot">
          <div class="spot-cover" :style="{ background: stats.longest.color }">
            <img v-if="stats.longest.coverUrl" :src="stats.longest.coverUrl" alt="" />
          </div>
          <div>
            <span>这一年最久的</span>
            <strong>{{ stats.longest.title }}</strong>
            <p>{{ [stats.longest.path, stats.longest.medium, fmtDur(stats.longest.minutes), shareOf(stats.longest.minutes)].filter(Boolean).join(' · ') }}</p>
          </div>
        </article>
        <article v-if="stats.favorite" class="spot">
          <div class="spot-cover" :style="{ background: stats.favorite.color }">
            <img v-if="stats.favorite.coverUrl" :src="stats.favorite.coverUrl" alt="" />
          </div>
          <div>
            <span>评分最高的</span>
            <strong>{{ stats.favorite.title }}</strong>
            <p>
              {{ [stats.favorite.path, stats.favorite.medium].filter(Boolean).join(' · ') }}
              <Stars :value="stats.favorite.rating ?? 0" />
            </p>
          </div>
        </article>
      </div>

      <article class="panel heat-panel">
        <div class="panel-head">
          <h3>抵达日历 <em>{{ stats.activeDays }} 天</em></h3>
          <div class="heat-legend" aria-hidden="true">
            <span>少</span>
            <i data-n="1"></i><i data-n="2"></i><i data-n="3"></i>
            <span>多</span>
          </div>
        </div>
        <div class="heat-wrap">
          <div class="heat-wd" aria-hidden="true"><span>一</span><span>三</span><span>五</span></div>
          <div class="heat-board">
            <div class="heat-months" aria-hidden="true">
              <span v-for="(label, index) in heatMonths" :key="index">{{ label }}</span>
            </div>
            <div class="heat">
              <span
                v-for="cell in heatCells"
                :key="cell.key"
                :data-n="cell.date ? heatLevel(cell.count) : ''"
                :title="cell.date ? `${fmtDate(cell.date)}${cell.count ? ' · ' + cell.count : ''}` : undefined"
              ></span>
            </div>
          </div>
        </div>
      </article>

      <div class="stats-grid">
        <article class="panel">
          <h3>月度{{ words.doneWord }}</h3>
          <p v-if="peakMonth" class="panel-note">{{ peakMonth.month }} 月 {{ words.doneWord }} {{ peakMonth.count }} 次，{{ fmtDur(peakMonth.minutes) || '未记时长' }}</p>
          <div class="bars">
            <div v-for="item in stats.byMonth" :key="item.month" class="bar-col" :class="{ peak: peakMonth && item.month === peakMonth.month }" :title="item.count ? `${item.count} · ${fmtDur(item.minutes)}` : ''">
              <span class="bar-n">{{ item.count || '' }}</span>
              <div class="track"><div class="bar" :style="{ height: monthHeight(item.count) }"></div></div>
              <span class="bar-l">{{ item.month }}月</span>
            </div>
          </div>
        </article>
        <article class="panel">
          <template v-if="mix.length">
            <h3>{{ mixTitle }}</h3>
            <div class="mix">
              <svg class="donut" viewBox="0 0 36 36" aria-hidden="true">
                <circle cx="18" cy="18" r="15.915" fill="none" stroke="var(--track)" stroke-width="3.4" />
                <circle
                  v-for="item in mix"
                  :key="item.name"
                  cx="18"
                  cy="18"
                  r="15.915"
                  fill="none"
                  :stroke="item.color"
                  stroke-width="3.4"
                  :stroke-dasharray="`${item.pct} ${100 - item.pct}`"
                  :stroke-dashoffset="25 - item.start"
                />
                <text x="18" y="17.2" text-anchor="middle">{{ mix[0].share }}%</text>
                <text x="18" y="21.4" text-anchor="middle">{{ mix[0].name }}</text>
              </svg>
              <ul>
                <li v-for="item in mix" :key="item.name">
                  <i :style="{ background: item.color }"></i>
                  <span>{{ item.name }}</span>
                  <b>{{ item.share }}%</b>
                </li>
              </ul>
            </div>
          </template>
          <template v-if="stats.byPath.length > 1">
            <h3 :class="{ subhead: mix.length }">{{ words.medium }}</h3>
            <div v-if="!stats.byMedium.length" class="hint">这一年还没有{{ words.doneWord }}记录</div>
            <div v-for="item in stats.byMedium" :key="item.name" class="plat-row">
              <span>{{ item.name }}</span>
              <div class="plat-track"><div class="plat-bar" :style="{ width: barWidth(item.count, maxMedium), background: item.color }"></div></div>
              <b>{{ item.count }}</b>
            </div>
          </template>
          <p v-else-if="!mix.length" class="hint">这一年还没有{{ words.doneWord }}记录</p>
          <template v-if="stats.byVia.length">
            <h3 class="subhead">{{ words.via }}</h3>
            <div v-for="item in stats.byVia" :key="item.name" class="plat-row">
              <span>{{ item.name }}</span>
              <div class="plat-track"><div class="plat-bar via" :style="{ width: barWidth(item.count, maxVia) }"></div></div>
              <b>{{ item.count }}</b>
            </div>
          </template>
        </article>
        <article v-if="stats.ranking.length" class="panel">
          <h3>时长排行</h3>
          <ol class="rank">
            <li v-for="(item, index) in stats.ranking" :key="item.title + index">
              <i class="rank-fill" :style="{ width: barWidth(item.minutes, maxRank) }"></i>
              <em>{{ index + 1 }}</em>
              <div class="rank-cover" :style="{ background: item.color }">
                <img v-if="item.coverUrl" :src="item.coverUrl" alt="" />
              </div>
              <div>
                <strong>{{ item.title }}</strong>
                <p>{{ item.medium }}</p>
              </div>
              <b>{{ fmtDur(item.minutes) }}</b>
            </li>
          </ol>
        </article>
        <article class="panel">
          <h3>口味</h3>
          <div v-if="!stats.topTags.length" class="hint">还没有标签</div>
          <div v-else ref="cloudEl" class="cloud">
            <span v-for="tag in cloudWords" :key="tag.name" :title="`${tag.name} ${tag.count}`" :style="tag.style">{{ tag.name }}</span>
          </div>
          <template v-if="stats.byBadge.length">
            <h3 class="subhead">印记</h3>
            <div class="tag-row">
              <span v-for="badge in stats.byBadge" :key="badge.name" class="pill tag">{{ badge.name }} {{ badge.count }}</span>
            </div>
          </template>
        </article>
        <article class="panel">
          <h3>最近{{ words.doneWord }}</h3>
          <ul class="recent">
            <li v-for="item in stats.recent" :key="item.title + item.finishDate + item.path">
              <div>
                <strong>{{ item.title }}</strong>
                <p>{{ item.path }} · {{ item.medium }} · {{ fmtDate(item.finishDate) }}</p>
              </div>
              <Stars v-if="item.rating != null" :value="item.rating" />
            </li>
          </ul>
        </article>
      </div>
    </template>
  </section>
</template>
