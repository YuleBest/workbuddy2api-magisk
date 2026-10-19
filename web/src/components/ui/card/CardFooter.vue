<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <div
    data-slot="card-footer"
    :class="cn('border-t border-border px-5 py-3.5 group-data-[size=sm]/card:px-4 flex items-center', props.class)"
  >
    <slot />
  </div>
</template>
