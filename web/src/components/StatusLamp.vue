<script setup lang="ts">
// 状态灯：可用（绿）/ 冷却（琥珀）/ 禁用（红）/ 未知（灰）。
// live=true 时呼吸闪烁，表示这个账号此刻正在承载流量。
defineProps<{ state: 'ok' | 'warn' | 'bad' | 'idle'; live?: boolean }>()
</script>

<template>
  <span class="lamp" :class="[state, { live: live && state === 'ok' }]" aria-hidden="true"></span>
</template>