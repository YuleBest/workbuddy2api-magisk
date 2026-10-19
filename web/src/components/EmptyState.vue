<script setup lang="ts">
// 空状态：图标 + 一句结论 + 可执行的下一步。
import type { Component } from 'vue'
import { Inbox } from '@lucide/vue'

defineProps<{ icon?: Component; title: string }>()
</script>

<template>
  <div class="flex flex-col gap-4 px-5 py-10 sm:flex-row sm:items-center sm:px-6">
    <div
      class="grid size-10 shrink-0 place-items-center rounded-full border border-border bg-muted text-muted-foreground"
    >
      <component :is="icon || Inbox" class="size-4" aria-hidden="true" />
    </div>
    <div class="min-w-0">
      <p class="font-serif text-[0.98rem] leading-snug">{{ title }}</p>
      <div v-if="$slots.default" class="mt-1 max-w-[65ch] text-xs leading-relaxed text-muted-foreground">
        <slot />
      </div>
    </div>
    <div v-if="$slots.action" class="sm:ml-auto">
      <slot name="action" />
    </div>
  </div>
</template>
