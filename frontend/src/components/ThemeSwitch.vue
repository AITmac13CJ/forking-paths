<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { themes, type ThemeId } from '../theme'

const theme = defineModel<ThemeId>({ required: true })
const open = ref(false)
const root = ref<HTMLElement | null>(null)

function choose(id: ThemeId) {
  theme.value = id
  open.value = false
}

function onDoc(event: MouseEvent) {
  if (!root.value?.contains(event.target as Node)) open.value = false
}

function onKey(event: KeyboardEvent) {
  if (event.key === 'Escape') open.value = false
}

onMounted(() => {
  document.addEventListener('click', onDoc)
  document.addEventListener('keydown', onKey)
})

onUnmounted(() => {
  document.removeEventListener('click', onDoc)
  document.removeEventListener('keydown', onKey)
})
</script>

<template>
  <div ref="root" class="theme-menu">
    <button
      type="button"
      class="theme-btn theme-current"
      :aria-expanded="open"
      aria-haspopup="listbox"
      aria-label="切换主题"
      @click="open = !open"
    >{{ themes.find((item) => item.id === theme)?.name }}</button>
    <div v-if="open" class="pop theme-pop" role="listbox" aria-label="主题">
      <button
        v-for="item in themes"
        :key="item.id"
        type="button"
        class="choice"
        role="option"
        :class="{ on: item.id === theme }"
        :aria-selected="item.id === theme"
        @click="choose(item.id)"
      >
        <i class="theme-swatch" :style="{ background: item.swatch }" />
        {{ item.name }}
      </button>
    </div>
  </div>
</template>
