<script setup lang="ts">
// 积分条：分段刻度，段数固定便于横向比对不同账号的余量。
// 相对刻度（max 取池内最高积分）表达"谁更富裕"，绝对数字在右侧以等宽字体给出。
import { computed } from 'vue'
import { n } from '../format'

const props = withDefaults(defineProps<{ value: number; max?: number; segments?: number }>(), {
  max: 0,
  segments: 12,
})

const filled = computed(() => {
  const max = props.max > 0 ? props.max : props.value
  if (max <= 0) return 0
  const ratio = Math.min(1, Math.max(0, props.value / max))
  // 有余量但不足一段时也点亮一段，避免"显示为空但实际能用"的误判。
  return ratio > 0 ? Math.max(1, Math.round(ratio * props.segments)) : 0
})

// 低于总量的 1/5 视为紧张：琥珀色；为 0 用红色（该号已耗尽，等签到恢复）。
const tone = computed(() => {
  const max = props.max > 0 ? props.max : props.value
  if (props.value <= 0) return 'empty'
  if (max > 0 && props.value / max < 0.2) return 'low'
  return ''
})
</script>

<template>
  <div class="credit">
    <div class="row">
      <span class="val">{{ n(value) }}</span>
      <span class="unit">积分</span>
    </div>
    <div class="track" role="img" :aria-label="`剩余 ${n(value)} 积分`">
      <span
        v-for="i in segments"
        :key="i"
        class="seg"
        :class="{ on: i <= filled, [tone]: tone && i <= filled }"
      ></span>
    </div>
  </div>
</template>