<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <tr
    data-slot="table-row"
    :class="cn('border-border/70 hover:bg-muted/40 data-[state=selected]:bg-muted border-b transition-colors last:border-0 has-aria-expanded:bg-muted/40', props.class)"
  >
    <slot />
  </tr>
</template>
