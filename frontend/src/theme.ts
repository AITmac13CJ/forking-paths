export const themes = [
  { id: 'day', name: '日间', swatch: '#6eae86' },
  { id: 'night', name: '夜间', swatch: '#6ee7c5' },
  { id: 'wild', name: '旷野', swatch: '#6b8f3e' },
  { id: 'isle', name: '浮岛', swatch: '#1f8a78' },
  { id: 'plain', name: '极简', swatch: '#161616' },
  { id: 'contrast', name: '高对比', swatch: '#ffe500' },
  { id: 'neon', name: '霓虹', swatch: '#ff2d95' },
  { id: 'pair', name: '相随', swatch: '#e23d32' },
] as const

export type ThemeId = (typeof themes)[number]['id']

const ids = new Set<string>(themes.map((item) => item.id))

export function readTheme(): ThemeId {
  const saved = localStorage.getItem('forking-paths-theme')
  const id: ThemeId = saved && ids.has(saved) ? (saved as ThemeId) : 'day'
  document.documentElement.dataset.theme = id
  return id
}
