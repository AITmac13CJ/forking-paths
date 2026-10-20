import type { Entry, EntryInput, Filters, Medium, Meta, Stats, Tag, Via } from './types'

export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
  })
  if (!res.ok) {
    let msg = '请求失败'
    try {
      const body = await res.json()
      if (body?.error) msg = body.error
    } catch {
      /* 非 JSON 错误页 */
    }
    throw new ApiError(msg, res.status)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

export function getMeta() {
  return request<Meta>('/api/meta')
}

export function getStats(year: number, pathId = 0) {
  return request<Stats>(`/api/stats?year=${year}&pathId=${pathId}`)
}

export function listEntries(filters: Filters, signal?: AbortSignal) {
  const p = new URLSearchParams()
  if (filters.pathId) p.set('pathId', String(filters.pathId))
  if (filters.q.trim()) p.set('q', filters.q.trim())
  if (filters.status) p.set('status', filters.status)
  if (filters.release) p.set('release', filters.release)
  if (filters.finish) p.set('finish', filters.finish)
  for (const id of filters.mediumIds) p.append('mediumId', String(id))
  for (const id of filters.tagIds) p.append('tagId', String(id))
  for (const sort of filters.sorts) p.append('sort', `${sort.field}:${sort.dir}`)
  return request<{ entries: Entry[]; total: number }>(`/api/entries?${p.toString()}`, { signal })
}

export function createEntry(input: EntryInput) {
  return request<Entry>('/api/entries', { method: 'POST', body: JSON.stringify(input) })
}

export function updateEntry(id: number, input: EntryInput) {
  return request<Entry>(`/api/entries/${id}`, { method: 'PUT', body: JSON.stringify(input) })
}

export function deleteEntry(id: number) {
  return request<void>(`/api/entries/${id}`, { method: 'DELETE' })
}

export function createTag(name: string) {
  return request<Tag>('/api/tags', { method: 'POST', body: JSON.stringify({ name }) })
}

export function createMedium(pathId: number, name: string) {
  return request<Medium>('/api/media', { method: 'POST', body: JSON.stringify({ pathId, name }) })
}

export function createVia(pathId: number, name: string) {
  return request<Via>('/api/vias', { method: 'POST', body: JSON.stringify({ pathId, name }) })
}

export async function uploadCover(file: File): Promise<string> {
  const body = new FormData()
  body.append('file', file)
  const res = await fetch('/api/covers', { method: 'POST', body })
  if (!res.ok) {
    let msg = '上传失败'
    try {
      const data = await res.json()
      if (data?.error) msg = data.error
    } catch {
      /* 非 JSON 错误页 */
    }
    throw new Error(msg)
  }
  const data = await res.json() as { url: string }
  return data.url
}

export type SyncAccount = { provider: string; linked: boolean; account: string }
export type SyncItem = { ref: string; title: string; minutes: number; complete: boolean; action: string }
export type SyncResult = { updated: number; created: number; items: SyncItem[] }
export type SyncJob = {
  id: number
  kind: string
  status: 'running' | 'done' | 'error'
  message: string
  done: number
  total: number
  updated: number
  created: number
}

export function syncAccounts() {
  return request<SyncAccount[]>('/api/sync')
}

export function saveSyncAccount(provider: string, account: string, secret: string) {
  return request<{ ok: boolean }>(`/api/sync/${provider}`, {
    method: 'PUT',
    body: JSON.stringify({ account, secret }),
  })
}

export function loginSync(provider: string) {
  return request<{ account?: string; url?: string }>(`/api/sync/${provider}/login`, { method: 'POST' })
}

export function savePSNProfile(onlineId: string) {
  return request<{ account: string }>('/api/sync/psn/profile', {
    method: 'POST',
    body: JSON.stringify({ onlineId }),
  })
}

export function saveSwitchCode(friendCode: string) {
  return request<{ account: string }>('/api/sync/switch/code', {
    method: 'POST',
    body: JSON.stringify({ friendCode }),
  })
}

export function saveNPSSO(npsso: string) {
  return request<{ account: string }>('/api/sync/psn/npsso', {
    method: 'POST',
    body: JSON.stringify({ npsso }),
  })
}

export function finishSwitch(url: string) {
  return request<{ account: string }>('/api/sync/switch/finish', {
    method: 'POST',
    body: JSON.stringify({ url }),
  })
}

export function runSync(provider: string) {
  return request<SyncJob>(`/api/sync/${provider}`, { method: 'POST' })
}

export function listJobs() {
  return request<SyncJob[]>('/api/sync/jobs')
}

export function ackJob(id: number) {
  return request<{ ok: boolean }>(`/api/sync/jobs/${id}/ack`, { method: 'POST' })
}

export function saveGridKey(secret: string) {
  return request<{ ok: boolean }>('/api/sync/covers/key', {
    method: 'PUT',
    body: JSON.stringify({ secret }),
  })
}

export function matchCovers() {
  return request<SyncJob>('/api/sync/covers', { method: 'POST' })
}

export function getMe() {
  return request<{ username: string; public: boolean }>('/api/auth/me')
}

export function setGardenPublic(on: boolean) {
  return request<{ public: boolean }>('/api/me/public', { method: 'PUT', body: JSON.stringify({ public: on }) })
}

export interface PlazaPerson {
  id: number
  username: string
  total: number
  minutes: number
  entries: Entry[]
}

export function listPlaza() {
  return request<{ people: PlazaPerson[] }>('/api/plaza')
}

export function getPlaza(id: number) {
  return request<PlazaPerson>(`/api/plaza/${id}`)
}

export function registerAccount(username: string, password: string, recovery: string) {
  return request<{ username: string }>('/api/auth/register', {
    method: 'POST',
    body: JSON.stringify({ username, password, recovery }),
  })
}

export function loginAccount(username: string, password: string) {
  return request<{ username: string }>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
}

export function resetAccount(username: string, recovery: string, password: string) {
  return request<{ username: string }>('/api/auth/forgot', {
    method: 'POST',
    body: JSON.stringify({ username, recovery, password }),
  })
}

export function logoutAccount() {
  return request<void>('/api/auth/logout', { method: 'POST' })
}

export function importSync(provider: string, games: SyncItem[]) {
  return request<{ created: number }>(`/api/sync/${provider}/import`, {
    method: 'POST',
    body: JSON.stringify({
      games: games.map((game) => ({
        ref: game.ref,
        title: game.title,
        minutes: game.minutes,
        complete: game.complete,
      })),
    }),
  })
}
