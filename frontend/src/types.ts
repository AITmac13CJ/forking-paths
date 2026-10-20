export interface Path {
  id: number
  slug: string
  name: string
  mediumLabel: string
  viaLabel: string
  releaseLabel: string
  spendLabel: string
  finishTimeLabel: string
  finishDateLabel: string
  doneWord: string
  doingTitle: string
  statusWant: string
  statusDoing: string
  statusDone: string
  statusHold: string
  badges: string[]
}

export interface Medium {
  id: number
  pathId: number
  name: string
  color: string
}

export interface Via {
  id: number
  pathId: number
  name: string
}

export interface Tag {
  id: number
  name: string
}

export interface Cover {
  a: string
  b: string
  motif: string
}

export interface Entry {
  id: number
  title: string
  path: Path
  medium: Medium | null
  via: Via | null
  tags: Tag[]
  releaseDate: string
  status: string
  spendMinutes: number
  finishMinutes: number | null
  finishDate: string
  rating: number | null
  badges: string[]
  progressEarned: number | null
  progressTotal: number | null
  cover: Cover
  coverUrl: string
  coverLandscapeUrl: string
  notes: string
}

export interface EntryInput {
  title: string
  pathId: number
  mediumId: number
  viaId: number | null
  tagIds: number[]
  releaseDate: string
  status: string
  spendMinutes: number
  finishMinutes: number | null
  finishDate: string
  rating: number | null
  badges: string[]
  progressEarned: number | null
  progressTotal: number | null
  cover: Cover
  coverUrl: string
  coverLandscapeUrl: string
  notes: string
}

export interface Meta {
  paths: Path[]
  media: Medium[]
  vias: Via[]
  tags: Tag[]
  motifs: string[]
}

export interface Words {
  name: string
  status: string
  medium: string
  via: string
  release: string
  spend: string
  finishTime: string
  finishDate: string
  want: string
  doing: string
  done: string
  hold: string
  doneWord: string
  doingTitle: string
  undone: string
}

export const gardenWords: Words = {
  name: '名称',
  status: '状态',
  medium: '载体',
  via: '经由',
  release: '面世',
  spend: '耗时',
  finishTime: '抵达用时',
  finishDate: '抵达日期',
  want: '想去',
  doing: '途中',
  done: '已至',
  hold: '搁下',
  doneWord: '抵达',
  doingTitle: '仍在途中',
  undone: '未至',
}

export function wordsOf(path?: Path | null): Words {
  if (!path) return gardenWords
  return {
    name: '名称',
    status: '状态',
    medium: path.mediumLabel,
    via: path.viaLabel,
    release: path.releaseLabel,
    spend: path.spendLabel,
    finishTime: path.finishTimeLabel,
    finishDate: path.finishDateLabel,
    want: path.statusWant,
    doing: path.statusDoing,
    done: path.statusDone,
    hold: path.statusHold,
    doneWord: path.doneWord,
    doingTitle: path.doingTitle,
    undone: `未${path.doneWord}`,
  }
}

export function statusLabel(kind: string, words: Words): string {
  if (kind === 'done') return words.done
  if (kind === 'doing') return words.doing
  if (kind === 'hold') return words.hold
  return words.want
}

export type SortField =
  | 'finish_date'
  | 'release_date'
  | 'rating'
  | 'spend_minutes'
  | 'finish_minutes'
  | 'title'

export interface Sort {
  field: SortField
  dir: 'asc' | 'desc'
}

export interface Filters {
  pathId: number
  q: string
  status: string
  release: '' | 'not_empty' | 'empty'
  finish: '' | 'year' | 'month' | 'empty'
  mediumIds: number[]
  tagIds: number[]
  sorts: Sort[]
}

export interface DayMark {
  date: string
  count: number
}

export interface Highlight {
  title: string
  coverUrl: string
  color: string
  medium: string
  path: string
  minutes: number
  rating: number | null
  finishDate: string
}

export interface RankItem {
  title: string
  coverUrl: string
  color: string
  medium: string
  minutes: number
  rating: number | null
}

export interface PathShare {
  name: string
  slug: string
  color: string
  count: number
  minutes: number
}

export interface Stats {
  year: number
  done: number
  doing: number
  want: number
  spendMinutes: number
  finishMinutes: number
  avgRating: number | null
  platinum: number
  marked: number
  byMedium: { name: string; color: string; count: number }[]
  byMonth: { month: number; count: number; minutes: number }[]
  topTags: { name: string; count: number }[]
  recent: { title: string; finishDate: string; medium: string; path: string; rating: number | null }[]
  activeDays: number
  prevDone: number
  prevSpend: number
  avgSpend: number
  days: DayMark[]
  longest: Highlight | null
  favorite: Highlight | null
  ranking: RankItem[]
  byPath: PathShare[]
  byBadge: { name: string; count: number }[]
  byVia: { name: string; count: number }[]
}

export type ViewId = 'plaza' | 'all' | 'month' | 'year' | 'playing' | 'stats'

export function preset(view: ViewId, pathId = 0): Filters {
  const base: Filters = {
    pathId,
    q: '',
    status: '',
    release: '',
    finish: '',
    mediumIds: [],
    tagIds: [],
    sorts: [{ field: 'finish_date', dir: 'desc' }],
  }
  if (view === 'year') {
    return {
      ...base,
      status: 'done',
      release: 'not_empty',
      finish: 'year',
      sorts: [
        { field: 'finish_date', dir: 'desc' },
        { field: 'rating', dir: 'desc' },
      ],
    }
  }
  if (view === 'month') {
    return { ...base, status: 'done', finish: 'month' }
  }
  if (view === 'playing') {
    return { ...base, status: 'doing', sorts: [{ field: 'spend_minutes', dir: 'desc' }] }
  }
  return base
}

export function emptyMeta(): Meta {
  return { paths: [], media: [], vias: [], tags: [], motifs: [] }
}
