<script setup lang="ts">
// 状态灯：可用（暖绿）/ 冷却（赭）/ 禁用（砖红）/ 未知（暖灰）。
// live=true 时呼吸闪烁，表示这个账号此刻正在承载流量。
defineProps<{ state: 'ok' | 'warn' | 'bad' | 'idle'; live?: boolean }>()
</script>

<template>
  <span class="lamp" :class="[state, { live: live && state === 'ok' }]" aria-hidden="true"></span>
</template>
