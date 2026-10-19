<script setup lang="ts">
// 页头：标题 + 一句"这页能干什么"的说明 + 右侧动作区。
defineProps<{ title: string; lede?: string }>()
</script>

<template>
  <header class="mb-7 border-b border-border pb-6 sm:mb-8">
    <div class="flex flex-wrap items-end gap-x-8 gap-y-4">
      <div class="min-w-0">
        <h1 class="font-serif text-[1.85rem] leading-[1.15] tracking-[-0.02em] sm:text-[2.15rem]">
          {{ title }}
        </h1>
        <p
          v-if="lede"
          class="mt-2 max-w-[76ch] text-[0.875rem] leading-relaxed text-muted-foreground text-pretty"
        >
          {{ lede }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2 sm:ml-auto">
        <slot name="actions" />
      </div>
    </div>
  </header>
</template>
