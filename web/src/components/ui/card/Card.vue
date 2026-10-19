<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = withDefaults(defineProps<{
  class?: HTMLAttributes['class']
  size?: 'default' | 'sm'
}>(), {
  size: 'default',
})
</script>

<template>
  <div
    data-slot="card"
    :data-size="size"
    :class="cn('bg-card text-card-foreground gap-0 overflow-hidden rounded-md py-0 text-sm shadow-page ring-1 ring-border group/card flex flex-col', props.class)"
  >
    <slot />
  </div>
</template>
