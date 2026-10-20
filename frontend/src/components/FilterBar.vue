<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { Filters, Meta, SortField, Words } from '../types'
import { statusLabel } from '../types'

const props = defineProps<{ filters: Filters; meta: Meta; words: Words }>()
const emit = defineEmits<{ update: [filters: Filters] }>()

const open = ref('')
const root = ref<HTMLElement | null>(null)

const sortFields = computed<{ id: SortField; label: string }[]>(() => [
  { id: 'finish_date', label: props.words.finishDate },
  { id: 'release_date', label: props.words.release },
  { id: 'rating', label: '评分' },
  { id: 'spend_minutes', label: props.words.spend },
  { id: 'finish_minutes', label: props.words.finishTime },
  { id: 'title', label: props.words.name },
])

const statuses = computed(() => [
  { id: 'done', label: props.words.done },
  { id: 'doing', label: props.words.doing },
  { id: 'want', label: props.words.want },
  { id: 'hold', label: props.words.hold },
])
const mediumChoices = computed(() => {
  const list = props.filters.pathId
    ? props.meta.media.filter((item) => item.pathId === props.filters.pathId)
    : props.meta.media
  return list
})
const mediumLabel = computed(() =>
  props.filters.mediumIds
    .map((id) => props.meta.media.find((item) => item.id === id)?.name)
    .filter(Boolean)
    .join(', '),
)
const tagLabel = computed(() =>
  props.filters.tagIds
    .map((id) => props.meta.tags.find((t) => t.id === id)?.name)
    .filter(Boolean)
    .join(', '),
)

const chips = computed(() => {
  const list: { key: string; prop: string; op: string; value: string }[] = []
  if (props.filters.status) list.push({ key: 'status', prop: props.words.status, op: 'Contains', value: statusLabel(props.filters.status, props.words) })
  if (props.filters.release === 'not_empty') list.push({ key: 'release', prop: props.words.release, op: 'Is not empty', value: '' })
  if (props.filters.release === 'empty') list.push({ key: 'release', prop: props.words.release, op: 'Is empty', value: '' })
  if (props.filters.finish === 'year') list.push({ key: 'finish', prop: props.words.finishDate, op: 'This year', value: '' })
  if (props.filters.finish === 'month') list.push({ key: 'finish', prop: props.words.finishDate, op: 'This month', value: '' })
  if (props.filters.finish === 'empty') list.push({ key: 'finish', prop: props.words.finishDate, op: 'Is empty', value: '' })
  if (props.filters.q.trim()) list.push({ key: 'q', prop: props.words.name, op: 'Contains', value: props.filters.q.trim() })
  if (props.filters.tagIds.length) list.push({ key: 'tag', prop: '标签', op: 'Contains', value: tagLabel.value })
  if (props.filters.mediumIds.length) list.push({ key: 'medium', prop: props.words.medium, op: 'Contains', value: mediumLabel.value })
  return list
})

const sortText = computed(() => (props.filters.sorts.length === 1 ? '1 sort' : `${props.filters.sorts.length} sorts`))
const ruleText = computed(() => {
  const n = chips.value.length
  if (n === 0) return 'Filter'
  return n === 1 ? '1 rule' : `${n} rules`
})

function patch(partial: Partial<Filters>) {
  emit('update', { ...props.filters, ...partial })
}

function toggle(key: string) {
  open.value = open.value === key ? '' : key
}

function remove(key: string) {
  if (key === 'status') patch({ status: '' })
  if (key === 'release') patch({ release: '' })
  if (key === 'finish') patch({ finish: '' })
  if (key === 'q') patch({ q: '' })
  if (key === 'medium') patch({ mediumIds: [] })
  if (key === 'tag') patch({ tagIds: [] })
  open.value = ''
}

function addFilter(key: string) {
  if (key === 'status' && !props.filters.status) patch({ status: 'done' })
  if (key === 'release' && !props.filters.release) patch({ release: 'not_empty' })
  if (key === 'finish' && !props.filters.finish) patch({ finish: 'year' })
  open.value = key
}

function toggleId(list: number[], id: number) {
  return list.includes(id) ? list.filter((item) => item !== id) : [...list, id]
}

function setSort(index: number, key: 'field' | 'dir', value: string) {
  const sorts = props.filters.sorts.map((sort, i) => (i === index ? { ...sort, [key]: value } : sort))
  patch({ sorts: sorts as Filters['sorts'] })
}

function addSort() {
  const used = new Set(props.filters.sorts.map((sort) => sort.field))
  const next = sortFields.value.find((field) => !used.has(field.id))
  if (!next || props.filters.sorts.length >= 3) return
  patch({ sorts: [...props.filters.sorts, { field: next.id, dir: 'desc' }] })
}

function removeSort(index: number) {
  const sorts = props.filters.sorts.filter((_, i) => i !== index)
  patch({ sorts: sorts.length ? sorts : [{ field: 'finish_date', dir: 'desc' }] })
}

function onDoc(event: MouseEvent) {
  if (!root.value?.contains(event.target as Node)) open.value = ''
}

onMounted(() => document.addEventListener('click', onDoc))
onBeforeUnmount(() => document.removeEventListener('click', onDoc))
</script>

<template>
  <div ref="root" class="filterbar" @click.stop>
    <div class="popwrap">
      <button type="button" class="tool-btn" @click="toggle('sort')">
        <svg viewBox="0 0 16 16" class="ico"><path d="M4 3v8M4 11 2 9M4 11l2-2M12 13V5M12 5 10 7M12 5l2 2" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" /></svg>
        {{ sortText }}
      </button>
      <div v-if="open === 'sort'" class="pop">
        <div v-for="(sort, index) in filters.sorts" :key="index" class="sort-row">
          <select :value="sort.field" @change="setSort(index, 'field', ($event.target as HTMLSelectElement).value)">
            <option v-for="field in sortFields" :key="field.id" :value="field.id">{{ field.label }}</option>
          </select>
          <select :value="sort.dir" @change="setSort(index, 'dir', ($event.target as HTMLSelectElement).value)">
            <option value="desc">降序</option>
            <option value="asc">升序</option>
          </select>
          <button type="button" class="icon-x" @click="removeSort(index)">×</button>
        </div>
        <button type="button" class="text-btn" :disabled="filters.sorts.length >= 3" @click="addSort">添加排序</button>
      </div>
    </div>

    <button type="button" class="tool-btn" @click="toggle('add')">
      <svg viewBox="0 0 16 16" class="ico"><path d="M2 4h12M4 8h8M6 12h4" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" /></svg>
      {{ ruleText }}
    </button>

    <div v-for="chip in chips" :key="chip.key" class="popwrap">
      <button type="button" class="chip" @click="toggle(chip.key)">
        <span class="k">{{ chip.prop }}</span>
        <span class="op">{{ chip.op }}</span>
        <span v-if="chip.value" class="v">{{ chip.value }}</span>
        <span class="x" @click.stop="remove(chip.key)">×</span>
      </button>
      <div v-if="open === chip.key" class="pop">
        <template v-if="chip.key === 'status'">
          <button v-for="status in statuses" :key="status.id" type="button" class="choice" :class="{ on: filters.status === status.id }" @click="patch({ status: status.id }); open = ''">{{ status.label }}</button>
        </template>
        <template v-else-if="chip.key === 'release'">
          <button type="button" class="choice" :class="{ on: filters.release === 'not_empty' }" @click="patch({ release: 'not_empty' }); open = ''">Is not empty</button>
          <button type="button" class="choice" :class="{ on: filters.release === 'empty' }" @click="patch({ release: 'empty' }); open = ''">Is empty</button>
        </template>
        <template v-else-if="chip.key === 'finish'">
          <button type="button" class="choice" :class="{ on: filters.finish === 'year' }" @click="patch({ finish: 'year' }); open = ''">This year</button>
          <button type="button" class="choice" :class="{ on: filters.finish === 'month' }" @click="patch({ finish: 'month' }); open = ''">This month</button>
          <button type="button" class="choice" :class="{ on: filters.finish === 'empty' }" @click="patch({ finish: 'empty' }); open = ''">Is empty</button>
        </template>
        <template v-else-if="chip.key === 'q'">
          <input :value="filters.q" class="pop-input" placeholder="包含文字" @input="patch({ q: ($event.target as HTMLInputElement).value })" />
        </template>
        <template v-else-if="chip.key === 'tag'">
          <label v-for="tag in meta.tags" :key="tag.id" class="check">
            <input type="checkbox" :checked="filters.tagIds.includes(tag.id)" @change="patch({ tagIds: toggleId(filters.tagIds, tag.id) })" />
            {{ tag.name }}
          </label>
        </template>
        <template v-else-if="chip.key === 'medium'">
          <label v-for="medium in mediumChoices" :key="medium.id" class="check">
            <input type="checkbox" :checked="filters.mediumIds.includes(medium.id)" @change="patch({ mediumIds: toggleId(filters.mediumIds, medium.id) })" />
            <i class="dot" :style="{ background: medium.color }"></i>
            {{ medium.name }}
          </label>
        </template>
      </div>
    </div>

    <div class="popwrap">
      <button type="button" class="quick" @click="toggle('q')">Aa {{ words.name }}</button>
      <div v-if="open === 'q' && !filters.q.trim()" class="pop">
        <input :value="filters.q" class="pop-input" placeholder="包含文字" @input="patch({ q: ($event.target as HTMLInputElement).value })" />
      </div>
    </div>
    <div class="popwrap">
      <button type="button" class="quick" @click="toggle('tag')">
        <svg viewBox="0 0 16 16" class="ico"><path d="M3 8.5 8.5 3H13v4.5L7.5 13Z" fill="none" stroke="currentColor" stroke-width="1.3" /><circle cx="10.5" cy="5.5" r="0.8" fill="currentColor" /></svg>
        标签
      </button>
      <div v-if="open === 'tag' && !filters.tagIds.length" class="pop">
        <label v-for="tag in meta.tags" :key="tag.id" class="check">
          <input type="checkbox" :checked="filters.tagIds.includes(tag.id)" @change="patch({ tagIds: toggleId(filters.tagIds, tag.id) })" />
          {{ tag.name }}
        </label>
      </div>
    </div>
    <div class="popwrap">
      <button type="button" class="quick" @click="toggle('medium')">
        <svg viewBox="0 0 16 16" class="ico"><rect x="2" y="3" width="12" height="10" rx="1.5" fill="none" stroke="currentColor" stroke-width="1.3" /><path d="M5 11V8M8 11V6M11 11V9" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" /></svg>
        {{ words.medium }}
      </button>
      <div v-if="open === 'medium' && !filters.mediumIds.length" class="pop">
        <label v-for="medium in mediumChoices" :key="medium.id" class="check">
          <input type="checkbox" :checked="filters.mediumIds.includes(medium.id)" @change="patch({ mediumIds: toggleId(filters.mediumIds, medium.id) })" />
          <i class="dot" :style="{ background: medium.color }"></i>
          {{ medium.name }}
        </label>
      </div>
    </div>

    <div class="popwrap">
      <button type="button" class="quick" @click="toggle('add')">+ Filter</button>
      <div v-if="open === 'add'" class="pop">
        <button type="button" class="choice" @click="addFilter('status')">{{ words.status }}</button>
        <button type="button" class="choice" @click="addFilter('release')">{{ words.release }}</button>
        <button type="button" class="choice" @click="addFilter('finish')">{{ words.finishDate }}</button>
        <button type="button" class="choice" @click="addFilter('q')">{{ words.name }}</button>
        <button type="button" class="choice" @click="addFilter('tag')">标签</button>
        <button type="button" class="choice" @click="addFilter('medium')">{{ words.medium }}</button>
      </div>
    </div>
  </div>
</template>
