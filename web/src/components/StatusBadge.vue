<script setup lang="ts">
// 状态标签：语义色调只用于网关状态，不做装饰。
import { Badge } from '@/components/ui/badge'

defineProps<{
  tone?: 'ok' | 'warn' | 'bad' | 'idle' | 'outline' | 'secondary'
}>()
</script>

<template>
  <Badge :variant="tone || 'outline'" class="font-normal">
    <slot />
  </Badge>
</template>
