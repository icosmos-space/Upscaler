<script lang="ts" setup>
import {onMounted, onUnmounted, ref, watch} from 'vue'
import {cn} from '@/lib/utils'

const props = defineProps<{
  beforeSrc: string
  afterSrc: string
  class?: string
}>()

const position = ref(50)
const dragging = ref(false)
const root = ref<HTMLElement | null>(null)

function setFromClientX(clientX: number) {
  const el = root.value
  if (!el) {
    return
  }
  const rect = el.getBoundingClientRect()
  if (rect.width <= 0) {
    return
  }
  position.value = Math.max(0, Math.min(100, ((clientX - rect.left) / rect.width) * 100))
}

function onPointerDown(event: PointerEvent) {
  dragging.value = true
  ;(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId)
  setFromClientX(event.clientX)
}

function onPointerMove(event: PointerEvent) {
  if (!dragging.value) {
    return
  }
  setFromClientX(event.clientX)
}

function onPointerUp() {
  dragging.value = false
}

function onKey(event: KeyboardEvent) {
  if (event.key === 'ArrowLeft') {
    position.value = Math.max(0, position.value - 2)
  } else if (event.key === 'ArrowRight') {
    position.value = Math.min(100, position.value + 2)
  }
}

watch(
  () => [props.beforeSrc, props.afterSrc],
  () => {
    position.value = 50
  },
)

onMounted(() => {
  window.addEventListener('pointerup', onPointerUp)
})

onUnmounted(() => {
  window.removeEventListener('pointerup', onPointerUp)
})
</script>

<template>
  <div
    ref="root"
    :class="cn('bg-background relative w-full cursor-ew-resize select-none overflow-hidden rounded-lg border', props.class)"
    role="slider"
    tabindex="0"
    :aria-valuenow="Math.round(position)"
    aria-valuemin="0"
    aria-valuemax="100"
    aria-label="处理前后对比"
    @pointerdown="onPointerDown"
    @pointermove="onPointerMove"
    @keydown="onKey"
  >
    <img
      :src="afterSrc"
      alt="处理后"
      class="pointer-events-none absolute inset-0 size-full object-contain"
      draggable="false"
    />
    <img
      :src="beforeSrc"
      alt="处理前"
      class="pointer-events-none absolute inset-0 size-full object-contain"
      :style="{ clipPath: `inset(0 ${100 - position}% 0 0)` }"
      draggable="false"
    />

    <div class="bg-background/85 pointer-events-none absolute top-3 left-3 rounded-md px-2 py-1 text-xs font-medium shadow-sm">
      处理前
    </div>
    <div class="bg-background/85 pointer-events-none absolute top-3 right-3 rounded-md px-2 py-1 text-xs font-medium shadow-sm">
      处理后
    </div>

    <div
      class="bg-primary absolute inset-y-0 z-10 w-0.5 -translate-x-1/2"
      :style="{ left: `${position}%` }"
    >
      <div
        class="bg-primary text-primary-foreground absolute top-1/2 left-1/2 flex size-7 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full text-xs shadow"
      >
        ⟷
      </div>
    </div>
  </div>
</template>
