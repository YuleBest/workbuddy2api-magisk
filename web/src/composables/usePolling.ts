// usePolling 自动刷新：按全局刷新间隔定期拉数据；页面隐藏时暂停，
// 回到前台立即补一次（后台开着不动不该持续打网关）。
import { onMounted, onUnmounted, watch } from 'vue'
import { state } from '../store'

export function usePolling(load: () => Promise<void> | void) {
  let timer: number | undefined

  const tick = () => {
    if (document.hidden) return
    void load()
  }

  const restart = () => {
    if (timer) window.clearInterval(timer)
    timer = undefined
    if (state.refreshSec > 0) {
      timer = window.setInterval(tick, state.refreshSec * 1000)
    }
  }

  const onVisible = () => {
    if (!document.hidden) tick()
  }

  onMounted(() => {
    tick()
    restart()
    document.addEventListener('visibilitychange', onVisible)
  })

  onUnmounted(() => {
    if (timer) window.clearInterval(timer)
    document.removeEventListener('visibilitychange', onVisible)
  })

  watch(() => state.refreshSec, restart)

  return { tick }
}