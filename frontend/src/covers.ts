import type { Cover } from './types'

export const palettes: Cover[] = [
  { a: '#f4a261', b: '#1d3557', motif: 'sun' },
  { a: '#6d28d9', b: '#e9d5ff', motif: 'crest' },
  { a: '#06d6a0', b: '#118ab2', motif: 'ink' },
  { a: '#111827', b: '#ea580c', motif: 'eclipse' },
  { a: '#4c1d95', b: '#38bdf8', motif: 'galaxy' },
  { a: '#b45309', b: '#fde68a', motif: 'crown' },
  { a: '#1e3a8a', b: '#60a5fa', motif: 'mech' },
  { a: '#fb7185', b: '#1f2937', motif: 'fox' },
  { a: '#0284c7', b: '#bae6fd', motif: 'sky' },
  { a: '#3f6212', b: '#d9f99d', motif: 'wind' },
  { a: '#312e81', b: '#f9a8d4', motif: 'castle' },
  { a: '#14532d', b: '#86efac', motif: 'leaf' },
]

export const motifNames: Record<string, string> = {
  sun: '日照',
  crest: '纹章',
  ink: '墨团',
  eclipse: '日蚀',
  galaxy: '星河',
  crown: '冠冕',
  mech: '机甲',
  fox: '折角',
  sky: '长空',
  wind: '风线',
  castle: '城垛',
  leaf: '叶片',
}
