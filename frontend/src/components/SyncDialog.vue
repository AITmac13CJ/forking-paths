<script setup lang="ts">
import { onBeforeUnmount, reactive, ref, watch } from 'vue'
import { finishSwitch, loginSync, matchCovers, runSync, saveGridKey, saveNPSSO, syncAccounts } from '../api'

const props = defineProps<{ open: boolean; running: string[] }>()
const emit = defineEmits<{ close: []; started: [] }>()

const providers = [
  { id: 'steam', name: 'Steam', hint: '跳到 Steam 官网登录，再回到这里。同步游玩时长和成就完成度。' },
  { id: 'psn', name: 'PSN', hint: '在弹出的窗口登录 PlayStation。页面变成文字后全选并复制，回到这里点完成。同步奖杯，白金会单独标出来。' },
  { id: 'xbox', name: 'Xbox', hint: '跳到微软账号登录，再回到这里。同步玩过的游戏和成就完成度。' },
  { id: 'switch', name: 'Switch', hint: '在新标签页登录任天堂。红色的「选择此人」不要左键点，右键它并复制链接地址，回到这里点完成。' },
]

const gateSteps: Record<string, string[]> = {
  psn: ['在弹出的窗口登录 PlayStation', '页面变成文字后，全选并复制', '回到花园，点完成'],
  switch: ['在新标签页登录任天堂', '看到红色的「选择此人」，不要左键点。右键它，选「复制链接地址」', '回到花园，点完成'],
}

const linked = reactive<Record<string, boolean>>({})
const savedAccount = reactive<Record<string, string>>({})
const busy = ref('')
const message = reactive<Record<string, string>>({})
const gridKey = ref('')
const gridEdit = ref(false)
const paste = reactive<Record<string, string>>({})
const gate = ref<{ id: string; url: string } | null>(null)
const gateStep = ref(1)
const gateMessage = ref('')
const needPaste = ref(false)
let official: Window | null = null
let officialTimer = 0

function stopOfficialWatch() {
  window.clearInterval(officialTimer)
  officialTimer = 0
}

function onGateFocus() {
  if (!gate.value) return
  gateStep.value = official && !official.closed ? 2 : 3
}

watch(gate, (value) => {
  window.removeEventListener('focus', onGateFocus)
  if (value) window.addEventListener('focus', onGateFocus)
})

onBeforeUnmount(() => {
  stopOfficialWatch()
  window.removeEventListener('focus', onGateFocus)
})

watch(() => props.open, (open) => {
  if (open) load().catch((err) => { message.covers = err instanceof Error ? err.message : '加载失败' })
})

async function load() {
  const rows = await syncAccounts()
  for (const row of rows) {
    linked[row.provider] = row.linked
    savedAccount[row.provider] = row.account
  }
}

function openOfficial(url: string, mode: 'popup' | 'tab' = 'popup') {
  if (mode === 'tab') {
    official = window.open(url, '_blank')
  } else {
    const width = 480
    const height = 720
    const left = window.screenX + Math.max(0, (window.outerWidth - width) / 2)
    const top = window.screenY + Math.max(0, (window.outerHeight - height) / 2)
    official = window.open(url, 'garden-official', `popup=yes,width=${width},height=${height},left=${Math.round(left)},top=${Math.round(top)}`)
  }
  stopOfficialWatch()
  if (!official) {
    gateStep.value = 1
    gateMessage.value = '浏览器拦住了弹出窗口。允许之后，再点一次打开官网。'
    return
  }
  gateStep.value = 1
  gateMessage.value = ''
  officialTimer = window.setInterval(() => {
    if (!official || official.closed) {
      stopOfficialWatch()
      gateStep.value = 3
    }
  }, 600)
}

function closeGate() {
  stopOfficialWatch()
  gate.value = null
  gateMessage.value = ''
  needPaste.value = false
}

function credentialReady(id: string, raw: string) {
  const text = raw.trim()
  if (!text || text.length > 8000) return false
  if (id === 'psn') return text.includes('npsso') || (text.length >= 32 && !/[\s{}]/.test(text))
  return text.includes('session_token_code') || text.includes('npf')
}

async function login(id: string) {
  if (id === 'psn' || id === 'switch') {
    message[id] = ''
    needPaste.value = false
    paste[id] = ''
    gateMessage.value = ''
    gateStep.value = 1
    try {
      const result = await loginSync(id)
      if (!result.url) throw new Error('没有拿到官网地址')
      gate.value = { id, url: result.url }
      openOfficial(result.url, id === 'switch' ? 'tab' : 'popup')
    } catch (err) {
      message[id] = err instanceof Error ? err.message : '登录失败'
    }
    return
  }
  busy.value = id
  message[id] = '正在前往官网…'
  try {
    const result = await loginSync(id)
    if (result.url) {
      window.location.href = result.url
      return
    }
    await load()
    message[id] = '已登录。'
  } catch (err) {
    message[id] = err instanceof Error ? err.message : '登录失败'
  } finally {
    busy.value = ''
  }
}

async function completeGate() {
  const current = gate.value
  if (!current) return
  let raw = (paste[current.id] || '').trim()
  if (!raw) {
    try {
      raw = (await navigator.clipboard.readText()).trim()
    } catch {
      needPaste.value = true
      gateMessage.value = '浏览器没有允许读取剪贴板，可以把结果贴在下面。'
      return
    }
  }
  if (!credentialReady(current.id, raw)) {
    needPaste.value = true
    gateMessage.value = '还没有看到登录结果。在官网窗口全选并复制后，再点完成。'
    return
  }
  busy.value = current.id
  gateMessage.value = ''
  try {
    if (current.id === 'psn') await saveNPSSO(raw)
    else await finishSwitch(raw)
    paste[current.id] = ''
    official?.close()
    closeGate()
    await load()
    message[current.id] = '已登录。'
  } catch (err) {
    gateMessage.value = err instanceof Error ? err.message : '登录失败'
  } finally {
    busy.value = ''
  }
}

async function sync(id: string) {
  message[id] = ''
  try {
    await runSync(id)
    message[id] = '已开始，进度在右上角。'
    emit('started')
  } catch (err) {
    message[id] = err instanceof Error ? err.message : '同步失败'
  }
}

async function saveGrid() {
  busy.value = 'grid'
  message.covers = ''
  try {
    await saveGridKey(gridKey.value.trim())
    gridKey.value = ''
    gridEdit.value = false
    await load()
    message.covers = '密钥已保存。'
  } catch (err) {
    message.covers = err instanceof Error ? err.message : '密钥保存失败'
  } finally {
    busy.value = ''
  }
}

function openGridKey() {
  window.open('https://www.steamgriddb.com/profile/preferences', 'steamgriddb', 'width=960,height=760')
}

async function covers() {
  message.covers = ''
  try {
    await matchCovers()
    message.covers = '已开始，进度在右上角。'
    emit('started')
  } catch (err) {
    message.covers = err instanceof Error ? err.message : '匹配失败'
  }
}

</script>

<template>
  <div v-if="open" class="overlay" @mousedown.self="emit('close')">
    <div class="dialog sync-dialog" role="dialog" aria-label="同步账户">
      <div class="dialog-head">
        <h2>同步账户</h2>
        <button type="button" class="icon-x" aria-label="关闭" @click="emit('close')">×</button>
      </div>
      <div class="sync-body">
        <section v-for="item in providers" :key="item.id" class="sync-block">
          <header>
            <strong>{{ item.name }}</strong>
            <span v-if="linked[item.id]" class="sync-ok">已登录{{ savedAccount[item.id] ? ' ' + savedAccount[item.id] : '' }}</span>
          </header>
          <p>{{ item.hint }}</p>
          <div class="sync-fields">
            <button type="button" class="tool-btn" :disabled="busy === item.id" @click="login(item.id)">
              {{ busy === item.id && !linked[item.id] ? '等待登录' : (linked[item.id] ? '重新登录' : '登录') }}
            </button>
            <button type="button" class="new-btn" :disabled="busy === item.id || !linked[item.id] || running.includes(item.id)" @click="sync(item.id)">
              {{ running.includes(item.id) ? '同步中' : '同步' }}
            </button>
          </div>
          <p v-if="message[item.id]" class="sync-msg">{{ message[item.id] }}</p>
        </section>
        <section class="sync-block">
          <header>
            <strong>封面</strong>
            <span v-if="linked.steamgriddb" class="sync-ok">密钥已保存</span>
          </header>
          <p>给所有平台还没有封面的游戏配图。已经有封面的不动，也不跟着某一次平台同步走。</p>
          <div class="sync-fields">
            <template v-if="!linked.steamgriddb || gridEdit">
              <label>
                SteamGridDB 密钥
                <input v-model="gridKey" type="password" autocomplete="off" placeholder="个人设置里的密钥" />
              </label>
              <button type="button" class="tool-btn" :disabled="busy === 'grid'" @click="saveGrid">保存密钥</button>
              <button type="button" class="tool-btn" @click="openGridKey">打开密钥页面</button>
            </template>
            <template v-else>
              <button type="button" class="new-btn" :disabled="running.includes('covers')" @click="covers">
                {{ running.includes('covers') ? '匹配中' : '匹配封面' }}
              </button>
              <button type="button" class="tool-btn" :disabled="busy !== ''" @click="gridEdit = true">更换密钥</button>
            </template>
          </div>
          <p v-if="message.covers" class="sync-msg">{{ message.covers }}</p>
        </section>
      </div>
    </div>
  </div>
  <div v-if="gate" class="overlay gate-overlay" @mousedown.self="closeGate">
    <div class="dialog gate-dialog" role="dialog" aria-labelledby="gate-title">
      <div class="dialog-head">
        <h2 id="gate-title">{{ gate.id === 'psn' ? '登录 PlayStation' : '登录任天堂' }}</h2>
        <button type="button" class="icon-x" aria-label="关闭" @click="closeGate">×</button>
      </div>
      <div class="gate-body">
        <div class="gate-window" aria-hidden="true">
          <div class="gate-window-bar"><span></span><span></span><span></span></div>
          <p>{{ gate.id === 'psn' ? 'PlayStation' : 'Nintendo' }}</p>
        </div>
        <ol class="gate-steps">
          <li v-for="(step, index) in gateSteps[gate.id]" :key="step" :class="{ on: gateStep === index + 1, done: gateStep > index + 1 }">{{ step }}</li>
        </ol>
        <p v-if="gateMessage" class="sync-msg">{{ gateMessage }}</p>
        <label v-if="needPaste" class="gate-fallback">
          登录结果
          <input v-model="paste[gate.id]" type="text" autocomplete="off" spellcheck="false" placeholder="官网窗口里的那一段" />
        </label>
      </div>
      <div class="dialog-foot">
        <button type="button" class="tool-btn" @click="openOfficial(gate.url, gate.id === 'switch' ? 'tab' : 'popup')">打开官网</button>
        <span class="spacer"></span>
        <button type="button" class="new-btn" :disabled="busy === gate.id" @click="completeGate">
          {{ busy === gate.id ? '正在确认' : '完成' }}
        </button>
      </div>
    </div>
  </div>
</template>
