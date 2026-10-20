<script setup lang="ts">
import { computed, ref } from 'vue'
import { loginAccount, registerAccount, resetAccount } from '../api'

const emit = defineEmits<{ authed: [username: string] }>()

const mode = ref<'login' | 'register' | 'forgot'>('login')
const username = ref('')
const password = ref('')
const confirm = ref('')
const recovery = ref('')
const error = ref('')
const busy = ref(false)

const title = computed(() => {
  if (mode.value === 'register') return '注册'
  if (mode.value === 'forgot') return '修改密码'
  return '登录'
})

function switchMode(next: 'login' | 'register' | 'forgot') {
  mode.value = next
  error.value = ''
  password.value = ''
  confirm.value = ''
}

async function submit() {
  error.value = ''
  if (password.value !== confirm.value && mode.value !== 'login') {
    error.value = '两次密码不一致'
    return
  }
  busy.value = true
  try {
    const name = username.value.trim()
    if (mode.value === 'register') {
      const user = await registerAccount(name, password.value, recovery.value.trim())
      emit('authed', user.username)
      return
    }
    if (mode.value === 'forgot') {
      const user = await resetAccount(name, recovery.value.trim(), password.value)
      emit('authed', user.username)
      return
    }
    const user = await loginAccount(name, password.value)
    emit('authed', user.username)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '没有成功'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <form class="auth-card" @submit.prevent="submit">
    <h1>{{ title }}</h1>
    <p class="auth-lead">用自己喜欢的账号进出这座花园。记录只属于这个账号。</p>
    <label>
      账号
      <input v-model="username" autocomplete="username" maxlength="24" required />
    </label>
    <label v-if="mode !== 'login'">
      找回暗号
      <input v-model="recovery" autocomplete="off" maxlength="40" required />
      <span class="hint">忘记密码时用它确认是你。请自己记住，这里不会发邮件。</span>
    </label>
    <label>
      {{ mode === 'forgot' ? '新密码' : '密码' }}
      <input v-model="password" type="password" :autocomplete="mode === 'login' ? 'current-password' : 'new-password'" required />
      <span v-if="mode !== 'login'" class="hint">至少 8 位，包含大写、小写、数字和符号。</span>
    </label>
    <label v-if="mode !== 'login'">
      再输入一次密码
      <input v-model="confirm" type="password" autocomplete="new-password" required />
    </label>
    <p v-if="error" class="banner">{{ error }}</p>
    <button type="submit" class="new-btn" :disabled="busy">{{ busy ? '请稍等' : title }}</button>
    <div class="auth-links">
      <button v-if="mode !== 'login'" type="button" @click="switchMode('login')">去登录</button>
      <button v-if="mode !== 'register'" type="button" @click="switchMode('register')">注册新账号</button>
      <button v-if="mode !== 'forgot'" type="button" @click="switchMode('forgot')">忘记密码</button>
    </div>
  </form>
</template>
