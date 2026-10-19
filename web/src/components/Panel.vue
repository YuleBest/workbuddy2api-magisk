<script setup lang="ts">
// 通用章节面板：统一 Card 的纸面观感、页眉节奏与反色章节。
// flush 用于表格类内容：不套内边距，让表头贴住卡片边缘。
import type { HTMLAttributes } from 'vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    title?: string
    note?: string
    tone?: 'paper' | 'ink'
    flush?: boolean
    class?: HTMLAttributes['class']
  }>(),
  { tone: 'paper' },
)
</script>

<template>
  <Card
    :class="
      cn(
        props.class,
        tone === 'ink'
          ? 'surface-ink ring-[color:var(--inverse-line)] shadow-none'
          : 'ring-border shadow-page',
      )
    "
  >
    <CardHeader
      v-if="title || note || $slots.actions"
      class="flex flex-row flex-wrap items-center gap-x-3 gap-y-2"
    >
      <div class="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-1">
        <CardTitle v-if="title">{{ title }}</CardTitle>
        <CardDescription v-if="note" :class="tone === 'ink' ? 'ink-muted' : ''">
          {{ note }}
        </CardDescription>
      </div>
      <div v-if="$slots.actions" class="ml-auto flex flex-wrap items-center gap-2">
        <slot name="actions" />
      </div>
    </CardHeader>

    <CardContent v-if="!flush">
      <slot />
    </CardContent>
    <slot v-else />
  </Card>
</template>
