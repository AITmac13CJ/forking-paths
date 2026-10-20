<script setup lang="ts">
import { computed, useId } from 'vue'
import type { Cover } from '../types'

const props = defineProps<{ cover: Cover }>()
const gid = 'c' + useId().replace(/[^a-zA-Z0-9_-]/g, '')
const motif = computed(() => props.cover?.motif || 'sun')
</script>

<template>
  <svg viewBox="0 0 400 250" preserveAspectRatio="xMidYMid slice" aria-hidden="true">
    <defs>
      <linearGradient :id="gid" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0%" :stop-color="cover.a || '#1d3557'" />
        <stop offset="100%" :stop-color="cover.b || '#f4a261'" />
      </linearGradient>
    </defs>
    <rect width="400" height="250" :fill="`url(#${gid})`" />

    <g v-if="motif === 'sun'" fill="none">
      <circle cx="292" cy="78" r="36" fill="#fff" opacity="0.92" />
      <circle cx="292" cy="78" r="58" stroke="#fff" stroke-width="8" opacity="0.45" />
      <path d="M0 168c48-36 96-20 144-4s96 8 144-20 80-18 112 6v100H0Z" fill="#000" opacity="0.22" />
      <path d="M0 198c70-28 120 10 190-8s110-6 210 18v42H0Z" fill="#000" opacity="0.28" />
    </g>

    <g v-else-if="motif === 'crest'">
      <path d="M200 28 268 78 248 168 200 214 152 168 132 78Z" fill="#fff" opacity="0.88" />
      <path d="M200 58 236 88 224 146 200 174 176 146 164 88Z" :fill="cover.a" opacity="0.9" />
      <path d="M40 210h320" stroke="#fff" stroke-width="10" opacity="0.55" />
    </g>

    <g v-else-if="motif === 'ink'">
      <circle cx="150" cy="120" r="70" fill="#fff" opacity="0.28" />
      <circle cx="230" cy="100" r="84" fill="#fff" opacity="0.38" />
      <circle cx="250" cy="160" r="48" fill="#000" opacity="0.18" />
      <circle cx="120" cy="170" r="26" fill="#fff" opacity="0.7" />
    </g>

    <g v-else-if="motif === 'eclipse'">
      <circle cx="210" cy="125" r="78" fill="#000" opacity="0.55" />
      <circle cx="248" cy="112" r="78" :fill="cover.b" />
      <circle cx="210" cy="125" r="78" fill="none" stroke="#fff" stroke-width="3" opacity="0.35" />
    </g>

    <g v-else-if="motif === 'galaxy'">
      <circle cx="200" cy="125" r="90" fill="none" stroke="#fff" stroke-width="2" opacity="0.35" />
      <circle cx="200" cy="125" r="58" fill="none" stroke="#fff" stroke-width="6" opacity="0.45" />
      <circle cx="286" cy="78" r="4" fill="#fff" />
      <circle cx="96" cy="70" r="3" fill="#fff" />
      <circle cx="318" cy="168" r="2.5" fill="#fff" />
      <circle cx="70" cy="160" r="3" fill="#fff" />
      <circle cx="200" cy="125" r="10" fill="#fff" />
    </g>

    <g v-else-if="motif === 'crown'" fill="#fff">
      <path d="M70 170 110 78 160 140 200 52 240 140 290 78 330 170Z" opacity="0.92" />
      <rect x="78" y="176" width="244" height="18" rx="4" opacity="0.92" />
    </g>

    <g v-else-if="motif === 'mech'" fill="#fff">
      <rect x="78" y="64" width="150" height="110" rx="8" opacity="0.9" />
      <rect x="228" y="96" width="92" height="78" opacity="0.55" />
      <rect x="108" y="88" width="70" height="18" :fill="cover.a" />
      <path d="M78 196h244" stroke="#fff" stroke-width="8" opacity="0.4" />
    </g>

    <g v-else-if="motif === 'fox'" fill="#fff">
      <path d="M64 190 150 46l46 78 28-52 112 118Z" opacity="0.9" />
      <path d="M150 46 186 18 176 92Z" opacity="0.55" />
    </g>

    <g v-else-if="motif === 'sky'">
      <circle cx="300" cy="64" r="28" fill="#fff" opacity="0.9" />
      <path d="M0 150h400" stroke="#fff" stroke-width="10" opacity="0.35" />
      <path d="M0 182h400" stroke="#fff" stroke-width="6" opacity="0.45" />
      <path d="M0 208h400" stroke="#000" stroke-width="8" opacity="0.12" />
    </g>

    <g v-else-if="motif === 'wind'" fill="none" stroke="#fff" stroke-linecap="round">
      <path d="M30 80c80 40 120-40 200-10s110 20 150-20" stroke-width="10" opacity="0.85" />
      <path d="M20 130c90 30 140-36 220 0s100 16 140-24" stroke-width="8" opacity="0.65" />
      <path d="M40 180c70 20 130-20 190 8s90 10 130-16" stroke-width="6" opacity="0.45" />
    </g>

    <g v-else-if="motif === 'castle'" fill="#fff">
      <rect x="70" y="120" width="50" height="90" opacity="0.9" />
      <rect x="140" y="78" width="120" height="132" opacity="0.92" />
      <rect x="280" y="108" width="54" height="102" opacity="0.75" />
      <rect x="168" y="110" width="22" height="34" :fill="cover.a" />
      <rect x="210" y="110" width="22" height="34" :fill="cover.a" />
    </g>

    <g v-else fill="#fff">
      <ellipse cx="168" cy="150" rx="46" ry="80" transform="rotate(-28 168 150)" opacity="0.9" />
      <ellipse cx="230" cy="132" rx="34" ry="70" transform="rotate(18 230 132)" opacity="0.55" />
    </g>
  </svg>
</template>
