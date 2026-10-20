export function fmtDate(value: string): string {
  if (!value) return ''
  return value.replaceAll('-', '/')
}

export function fmtDur(mins: number | null | undefined): string {
  if (mins == null) return ''
  const h = Math.floor(mins / 60)
  const m = mins % 60
  if (h <= 0) return `${m} 分钟`
  if (m === 0) return `${h} 小时`
  return `${h} 小时 ${m} 分钟`
}
