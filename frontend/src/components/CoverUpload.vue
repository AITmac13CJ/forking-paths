<script setup lang="ts">
import { ref } from 'vue'
import { uploadCover } from '../api'

defineProps<{ label: string }>()
const model = defineModel<string>({ required: true })

const input = ref<HTMLInputElement | null>(null)
const uploading = ref(false)
const error = ref('')

function pick() {
  input.value?.click()
}

function onChoose(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (file) void send(file)
  if (input.value) input.value.value = ''
}

function onDrop(event: DragEvent) {
  const file = event.dataTransfer?.files?.[0]
  if (file) void send(file)
}

async function send(file: File) {
  error.value = ''
  uploading.value = true
  try {
    model.value = await uploadCover(file)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '上传失败'
  } finally {
    uploading.value = false
  }
}
</script>

<template>
  <label class="field">
    {{ label }}
    <div class="upload-row" @dragover.prevent @drop.prevent="onDrop">
      <input v-model="model" type="text" placeholder="图片地址，或上传" @input="error = ''" />
      <button type="button" class="upload-btn" :disabled="uploading" @click="pick">
        {{ uploading ? '上传中' : '上传' }}
      </button>
      <input
        ref="input"
        class="upload-file"
        type="file"
        accept="image/jpeg,image/png,image/webp,image/gif"
        @change="onChoose"
      />
    </div>
    <p v-if="error" class="upload-err">{{ error }}</p>
  </label>
</template>
