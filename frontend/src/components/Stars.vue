<script setup lang="ts">
const props = defineProps<{ value: number | null; interactive?: boolean }>()
const emit = defineEmits<{ rate: [value: number] }>()

const star = 'M12 2.4 14.9 8.7 21.7 9.6 16.7 14.4 18 21.1 12 17.8 6 21.1 7.3 14.4 2.3 9.6 9.1 8.7Z'

function width(i: number) {
  const value = props.value ?? 0
  if (value >= i) return '100%'
  if (value >= i - 0.5) return '50%'
  return '0%'
}
</script>

<template>
  <div class="stars" :class="{ live: interactive }">
    <span
      v-for="i in 5"
      :key="i"
      class="star"
      @click.stop="interactive && emit('rate', i)"
    >
      <svg viewBox="0 0 24 24" class="star-bg"><path :d="star" /></svg>
      <span class="star-fg" :style="{ width: width(i) }">
        <svg viewBox="0 0 24 24"><path :d="star" /></svg>
      </span>
    </span>
  </div>
</template>
