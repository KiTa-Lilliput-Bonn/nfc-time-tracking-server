import { defineStore } from 'pinia'
import { ref } from 'vue'
import { fetchPendingRequestCount } from '@/api/requests'

/** Anzahl offener Anträge für die Leitung (Badge in der Navigation). */
export const usePendingRequests = defineStore('pendingRequests', () => {
  const count = ref(0)

  async function refresh() {
    try {
      count.value = await fetchPendingRequestCount()
    } catch {
      // Badge ist nur ein Hinweis; Fehler nicht anzeigen.
    }
  }

  return { count, refresh }
})
