<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import GameCard from './GameCard.vue'
import { ApiError, getPlaza, listPlaza, type PlazaPerson } from '../api'
import { fmtDur } from '../format'

const props = defineProps<{ tick: number }>()
const emit = defineEmits<{ error: [message: string] }>()

const people = ref<PlazaPerson[]>([])
const person = ref<PlazaPerson | null>(null)
const loading = ref(false)
const pinTop = ref(96)
let pinObs: ResizeObserver | null = null

onMounted(() => {
  const bar = document.querySelector('.sticky')
  if (!bar) return
  const apply = () => { pinTop.value = Math.ceil(bar.getBoundingClientRect().height) }
  apply()
  pinObs = new ResizeObserver(apply)
  pinObs.observe(bar)
})
onUnmounted(() => pinObs?.disconnect())

function mark(name: string) {
  return [...name][0] ?? '径'
}

function line(item: PlazaPerson) {
  const time = fmtDur(item.minutes) || '0 分钟'
  return `${item.total} 条游戏 · ${time}`
}

async function load() {
  loading.value = true
  person.value = null
  try {
    const res = await listPlaza()
    people.value = res.people ?? []
    emit('error', '')
  } catch (err) {
    people.value = []
    emit('error', err instanceof ApiError ? err.message : '广场加载失败')
  } finally {
    loading.value = false
  }
}

async function open(item: PlazaPerson) {
  loading.value = true
  try {
    person.value = await getPlaza(item.id)
    emit('error', '')
  } catch (err) {
    emit('error', err instanceof ApiError ? err.message : '这份记录没有公开')
  } finally {
    loading.value = false
  }
}

watch(() => props.tick, () => { void load() }, { immediate: true })
</script>

<template>
  <div v-if="loading && !people.length && !person" class="plaza-home">
    <div v-for="i in 2" :key="i" class="plaza-card skeleton-card"></div>
  </div>
  <div v-else-if="person" class="plaza-profile" :class="{ dim: loading }">
    <div class="plaza-pin" :style="{ top: pinTop + 'px' }">
      <button type="button" class="text-btn plaza-back" @click="person = null">返回广场</button>
      <div class="plaza-id">
        <span class="plaza-avatar">{{ mark(person.username) }}</span>
        <div>
          <strong>{{ person.username }}</strong>
          <em>{{ line(person) }}</em>
        </div>
      </div>
    </div>
    <div v-if="person.entries.length" class="grid size-s plaza-wall">
      <GameCard
        v-for="entry in person.entries"
        :key="entry.id"
        :entry="entry"
        orient="portrait"
        @open="() => {}"
      />
    </div>
    <p v-else class="empty">还没有游玩记录</p>
  </div>
  <div v-else class="plaza-home">
    <p v-if="!people.length" class="empty">还没有人公开记录</p>
    <article v-for="item in people" :key="item.id" class="plaza-card">
      <button type="button" class="plaza-id" @click="open(item)">
        <span class="plaza-avatar">{{ mark(item.username) }}</span>
        <span class="plaza-who">
          <strong>{{ item.username }}</strong>
          <em>{{ line(item) }}</em>
        </span>
        <span class="plaza-enter">主页</span>
      </button>
      <div v-if="item.entries.length" class="grid size-s plaza-strip">
        <GameCard
          v-for="entry in item.entries.slice(0, 4)"
          :key="entry.id"
          :entry="entry"
          orient="portrait"
          @open="open(item)"
        />
      </div>
      <p v-else class="hint plaza-empty">还没有游玩记录</p>
    </article>
  </div>
</template>
