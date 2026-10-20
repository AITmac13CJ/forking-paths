<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { Entry, EntryInput, Medium, Meta, Tag, Via } from '../types'
import { wordsOf } from '../types'
import { palettes, motifNames } from '../covers'
import { createEntry, createMedium, createTag, createVia, deleteEntry, updateEntry } from '../api'
import CoverArt from './CoverArt.vue'
import CoverUpload from './CoverUpload.vue'
import Stars from './Stars.vue'

const props = defineProps<{ open: boolean; entry: Entry | null; meta: Meta; defaultPathId: number }>()
const emit = defineEmits<{
  close: []
  saved: []
  tag: [tag: Tag]
  medium: [medium: Medium]
  via: [via: Via]
}>()

const draft = ref<EntryInput>(blank())
const spendH = ref('')
const spendM = ref('')
const finishH = ref('')
const finishM = ref('')
const saving = ref(false)
const formError = ref('')
const tagName = ref('')
const mediumName = ref('')
const viaName = ref('')
const broken = ref(false)

const path = computed(() => props.meta.paths.find((item) => item.id === draft.value.pathId) ?? null)
const words = computed(() => wordsOf(path.value))
const media = computed(() => props.meta.media.filter((item) => item.pathId === draft.value.pathId))
const vias = computed(() => props.meta.vias.filter((item) => item.pathId === draft.value.pathId))
const statuses = computed(() => [
  { id: 'done', label: words.value.done },
  { id: 'doing', label: words.value.doing },
  { id: 'want', label: words.value.want },
  { id: 'hold', label: words.value.hold },
])
const badgeOptions = computed(() => {
  const set = new Set(path.value?.badges ?? [])
  for (const badge of draft.value.badges) set.add(badge)
  return [...set]
})

function blank(): EntryInput {
  const today = new Date()
  const iso = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`
  const pathId = props.defaultPathId || props.meta.paths[0]?.id || 0
  const mediumId = props.meta.media.find((item) => item.pathId === pathId)?.id ?? 0
  return {
    title: '',
    pathId,
    mediumId,
    viaId: null,
    tagIds: [],
    releaseDate: '',
    status: 'done',
    spendMinutes: 0,
    finishMinutes: null,
    finishDate: iso,
    rating: 4,
    badges: [],
    progressEarned: null,
    progressTotal: null,
    cover: { ...(palettes[0] as EntryInput['cover']) },
    coverUrl: '',
    coverLandscapeUrl: '',
    notes: '',
  }
}

function split(mins: number | null): [string, string] {
  if (mins == null) return ['', '']
  return [String(Math.floor(mins / 60)), String(mins % 60)]
}

function toInput(entry: Entry): EntryInput {
  return {
    title: entry.title,
    pathId: entry.path.id,
    mediumId: entry.medium?.id ?? 0,
    viaId: entry.via?.id ?? null,
    tagIds: entry.tags.map((tag) => tag.id),
    releaseDate: entry.releaseDate,
    status: entry.status,
    spendMinutes: entry.spendMinutes,
    finishMinutes: entry.finishMinutes,
    finishDate: entry.finishDate,
    rating: entry.rating,
    badges: [...entry.badges],
    progressEarned: entry.progressEarned,
    progressTotal: entry.progressTotal,
    cover: { ...entry.cover },
    coverUrl: entry.coverUrl,
    coverLandscapeUrl: entry.coverLandscapeUrl,
    notes: entry.notes,
  }
}

watch(() => props.open, (isOpen) => {
  document.body.style.overflow = isOpen ? 'hidden' : ''
  if (!isOpen) return
  formError.value = ''
  tagName.value = ''
  mediumName.value = ''
  viaName.value = ''
  broken.value = false
  if (props.entry) {
    draft.value = toInput(props.entry)
    ;[spendH.value, spendM.value] = split(props.entry.spendMinutes)
    ;[finishH.value, finishM.value] = split(props.entry.finishMinutes)
  } else {
    draft.value = blank()
    spendH.value = ''
    spendM.value = ''
    finishH.value = ''
    finishM.value = ''
  }
})

watch(() => draft.value.pathId, (id, prev) => {
  if (!prev || id === prev) return
  const nextMedia = props.meta.media.filter((item) => item.pathId === id)
  if (!nextMedia.some((item) => item.id === draft.value.mediumId)) {
    draft.value.mediumId = nextMedia[0]?.id ?? 0
  }
  const nextVias = props.meta.vias.filter((item) => item.pathId === id)
  if (draft.value.viaId && !nextVias.some((item) => item.id === draft.value.viaId)) {
    draft.value.viaId = null
  }
  const allowed = new Set(props.meta.paths.find((item) => item.id === id)?.badges ?? [])
  draft.value.badges = draft.value.badges.filter((badge) => allowed.has(badge))
})

function toMinutes(h: string | number, m: string | number): number | null {
  const hours = String(h ?? '').trim()
  const minutes = String(m ?? '').trim()
  if (hours === '' && minutes === '') return null
  const hh = Number(hours || 0)
  const mm = Number(minutes || 0)
  if (!Number.isFinite(hh) || !Number.isFinite(mm) || hh < 0 || mm < 0) {
    throw new Error('时长需要是非负数字')
  }
  return Math.floor(hh) * 60 + Math.floor(mm)
}

function onViaPick(event: Event) {
  const value = (event.target as HTMLSelectElement).value
  draft.value.viaId = value ? Number(value) : null
}

function onRate(star: number) {
  if (draft.value.rating === star) draft.value.rating = star - 0.5
  else if (draft.value.rating === star - 0.5) draft.value.rating = null
  else draft.value.rating = star
}

function toggleTag(id: number) {
  draft.value.tagIds = draft.value.tagIds.includes(id)
    ? draft.value.tagIds.filter((item) => item !== id)
    : [...draft.value.tagIds, id]
}

function toggleBadge(badge: string) {
  draft.value.badges = draft.value.badges.includes(badge)
    ? draft.value.badges.filter((item) => item !== badge)
    : [...draft.value.badges, badge]
}

async function addTag() {
  const name = tagName.value.trim()
  if (!name) return
  try {
    const tag = await createTag(name)
    emit('tag', tag)
    if (!draft.value.tagIds.includes(tag.id)) draft.value.tagIds = [...draft.value.tagIds, tag.id]
    tagName.value = ''
  } catch (error) {
    formError.value = error instanceof Error ? error.message : '添加标签失败'
  }
}

async function addMedium() {
  const name = mediumName.value.trim()
  if (!name || !draft.value.pathId) return
  try {
    const medium = await createMedium(draft.value.pathId, name)
    emit('medium', medium)
    draft.value.mediumId = medium.id
    mediumName.value = ''
  } catch (error) {
    formError.value = error instanceof Error ? error.message : '添加载体失败'
  }
}

async function addVia() {
  const name = viaName.value.trim()
  if (!name || !draft.value.pathId) return
  try {
    const via = await createVia(draft.value.pathId, name)
    emit('via', via)
    draft.value.viaId = via.id
    viaName.value = ''
  } catch (error) {
    formError.value = error instanceof Error ? error.message : '添加经由失败'
  }
}

async function save() {
  formError.value = ''
  if (!draft.value.title.trim()) {
    formError.value = '请填写名称'
    return
  }
  if (!draft.value.pathId) {
    formError.value = '请选择一条小径'
    return
  }
  if (!draft.value.mediumId) {
    formError.value = '请选择载体'
    return
  }
  let spend = 0
  let finish: number | null = null
  try {
    spend = toMinutes(spendH.value, spendM.value) ?? 0
    finish = toMinutes(finishH.value, finishM.value)
  } catch (error) {
    formError.value = error instanceof Error ? error.message : '时长不正确'
    return
  }
  const payload: EntryInput = {
    ...draft.value,
    title: draft.value.title.trim(),
    spendMinutes: spend,
    finishMinutes: finish,
    tagIds: [...draft.value.tagIds],
    badges: [...draft.value.badges],
    cover: { ...draft.value.cover },
  }
  saving.value = true
  try {
    if (props.entry) await updateEntry(props.entry.id, payload)
    else await createEntry(payload)
    emit('saved')
  } catch (error) {
    formError.value = error instanceof Error ? error.message : '保存失败'
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (!props.entry) return
  if (!confirm(`删除「${props.entry.title}」？`)) return
  saving.value = true
  try {
    await deleteEntry(props.entry.id)
    emit('saved')
  } catch (error) {
    formError.value = error instanceof Error ? error.message : '删除失败'
  } finally {
    saving.value = false
  }
}

function onKey(event: KeyboardEvent) {
  if (event.key === 'Escape' && props.open) emit('close')
}

onMounted(() => document.addEventListener('keydown', onKey))
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKey)
  document.body.style.overflow = ''
})
</script>

<template>
  <div v-if="open" class="overlay" @click.self="emit('close')">
    <form class="dialog" @submit.prevent="save">
      <header class="dialog-head">
        <h2>{{ entry ? '编辑记录' : '新的记录' }}</h2>
        <button type="button" class="icon-x" @click="emit('close')">×</button>
      </header>

      <div class="form-grid">
        <div class="span-2 cover-edit">
          <div class="cover-preview">
            <img v-if="draft.coverUrl && !broken" :src="draft.coverUrl" alt="" @error="broken = true" />
            <CoverArt v-else :cover="draft.cover" />
          </div>
          <div>
            <p class="hint">封面配色</p>
            <div class="swatches">
              <button
                v-for="(palette, index) in palettes"
                :key="index"
                type="button"
                class="swatch"
                :class="{ on: draft.cover.a === palette.a && draft.cover.b === palette.b }"
                :style="{ background: `linear-gradient(135deg, ${palette.a}, ${palette.b})` }"
                @click="draft.cover = { ...draft.cover, a: palette.a, b: palette.b }"
              />
            </div>
            <p class="hint">图形</p>
            <div class="choice-row">
              <button
                v-for="(label, motif) in motifNames"
                :key="motif"
                type="button"
                class="mini"
                :class="{ on: draft.cover.motif === motif }"
                @click="draft.cover = { ...draft.cover, motif }"
              >{{ label }}</button>
            </div>
            <CoverUpload v-model="draft.coverUrl" label="竖版封面（可选）" @update:model-value="broken = false" />
            <CoverUpload v-model="draft.coverLandscapeUrl" label="横版封面（可选）" />
          </div>
        </div>

        <label class="field">
          小径
          <select v-model.number="draft.pathId">
            <option v-for="item in meta.paths" :key="item.id" :value="item.id">{{ item.name }}</option>
          </select>
        </label>
        <label class="field">
          {{ words.name }}
          <input v-model="draft.title" maxlength="80" required />
        </label>
        <label class="field">
          {{ words.medium }}
          <select v-model.number="draft.mediumId">
            <option v-for="item in media" :key="item.id" :value="item.id">{{ item.name }}</option>
          </select>
          <span class="inline-add">
            <input v-model="mediumName" placeholder="新的载体" maxlength="24" @keydown.enter.prevent="addMedium" />
            <button type="button" class="text-btn" @click="addMedium">添加</button>
          </span>
        </label>
        <label class="field">
          {{ words.release }}
          <input v-model="draft.releaseDate" type="date" />
        </label>

        <div class="field span-2">
          <span>标签</span>
          <div class="choice-row">
            <button
              v-for="tag in meta.tags"
              :key="tag.id"
              type="button"
              class="mini"
              :class="{ on: draft.tagIds.includes(tag.id) }"
              @click="toggleTag(tag.id)"
            >{{ tag.name }}</button>
          </div>
          <div class="inline-add">
            <input v-model="tagName" placeholder="新标签" maxlength="16" @keydown.enter.prevent="addTag" />
            <button type="button" class="text-btn" @click="addTag">添加</button>
          </div>
        </div>

        <label class="field">
          {{ words.spend }}
          <span class="time-inputs">
            <input v-model="spendH" type="number" min="0" placeholder="时" />
            <em>小时</em>
            <input v-model="spendM" type="number" min="0" placeholder="分" />
            <em>分钟</em>
          </span>
        </label>
        <div class="field">
          <span>{{ words.status }}</span>
          <div class="choice-row">
            <button
              v-for="status in statuses"
              :key="status.id"
              type="button"
              class="mini"
              :class="{ on: draft.status === status.id }"
              @click="draft.status = status.id"
            >{{ status.label }}</button>
          </div>
        </div>

        <div class="field span-2">
          <span>标记</span>
          <div class="choice-row">
            <button
              v-for="badge in badgeOptions"
              :key="badge"
              type="button"
              class="mini"
              :class="{ on: draft.badges.includes(badge) }"
              @click="toggleBadge(badge)"
            >{{ badge }}</button>
          </div>
        </div>

        <label class="field">
          {{ words.finishTime }}
          <span class="time-inputs">
            <input v-model="finishH" type="number" min="0" placeholder="时" />
            <em>小时</em>
            <input v-model="finishM" type="number" min="0" placeholder="分" />
            <em>分钟</em>
          </span>
        </label>
        <label class="field">
          {{ words.finishDate }}
          <input v-model="draft.finishDate" type="date" />
        </label>

        <label class="field">
          {{ words.via }}
          <select :value="draft.viaId ?? ''" @change="onViaPick">
            <option value="">未选择</option>
            <option v-for="item in vias" :key="item.id" :value="item.id">{{ item.name }}</option>
          </select>
          <span class="inline-add">
            <input v-model="viaName" placeholder="新的经由" maxlength="24" @keydown.enter.prevent="addVia" />
            <button type="button" class="text-btn" @click="addVia">添加</button>
          </span>
        </label>
        <div class="field">
          <span>评分</span>
          <Stars :value="draft.rating" interactive @rate="onRate" />
          <p class="hint">再点同一颗星改为半星，点第三次清除。</p>
        </div>

        <label class="field span-2">
          笔记
          <textarea v-model="draft.notes" maxlength="4000" placeholder="走过之后，还想记住的"></textarea>
        </label>
      </div>

      <footer class="dialog-foot">
        <button v-if="entry" type="button" class="danger" :disabled="saving" @click="remove">删除</button>
        <span class="spacer"></span>
        <p v-if="formError" class="form-error">{{ formError }}</p>
        <button type="button" class="text-btn" @click="emit('close')">取消</button>
        <button type="submit" class="new-btn" :disabled="saving">{{ saving ? '保存中' : '保存' }}</button>
      </footer>
    </form>
  </div>
</template>
