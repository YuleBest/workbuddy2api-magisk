<script setup lang="ts">
// 页头：标题 + 一句"这页能干什么"的说明 + 右侧动作区。
defineProps<{ title: string; lede?: string }>()
</script>

<template>
  <header class="page-head">
    <div>
      <h1>{{ title }}</h1>
      <p v-if="lede" class="lede">{{ lede }}</p>
    </div>
    <div class="actions">
      <slot name="actions" />
    </div>
  </header>
</template>