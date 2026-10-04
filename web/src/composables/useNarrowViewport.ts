import { onMounted, onUnmounted, ref } from 'vue'

/** true auf schmalen Bildschirmen (Handy), gleiche Grenze wie die mobile Navigation in AppLayout. */
export function useNarrowViewport(maxWidth = 768) {
  const query = `(max-width: ${maxWidth}px)`
  const narrow = ref(typeof window !== 'undefined' && window.matchMedia(query).matches)
  let mql: MediaQueryList | null = null
  const onChange = (e: MediaQueryListEvent) => {
    narrow.value = e.matches
  }
  onMounted(() => {
    mql = window.matchMedia(query)
    narrow.value = mql.matches
    mql.addEventListener('change', onChange)
  })
  onUnmounted(() => {
    mql?.removeEventListener('change', onChange)
  })
  return narrow
}
