<script setup lang="ts">
// 确认弹窗：把危险/不可逆动作收进一个明确的选择，不依赖浏览器原生 confirm。
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

withDefaults(
  defineProps<{
    open: boolean
    title: string
    description?: string
    confirmText?: string
    cancelText?: string
    destructive?: boolean
    busy?: boolean
  }>(),
  { confirmText: '确认', cancelText: '取消' },
)

const emit = defineEmits<{
  (e: 'update:open', value: boolean): void
  (e: 'confirm'): void
}>()
</script>

<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogContent class="max-w-md">
      <DialogHeader class="border-b border-border px-6 py-4 pr-14">
        <DialogTitle>{{ title }}</DialogTitle>
        <DialogDescription v-if="description">{{ description }}</DialogDescription>
      </DialogHeader>
      <div v-if="$slots.default" class="px-6 py-4 text-sm">
        <slot />
      </div>
      <DialogFooter class="border-t border-border px-6 py-3.5">
        <Button variant="outline" :disabled="busy" @click="emit('update:open', false)">
          {{ cancelText }}
        </Button>
        <Button
          :variant="destructive ? 'destructive' : 'default'"
          :disabled="busy"
          @click="emit('confirm')"
        >
          {{ busy ? '处理中…' : confirmText }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
