<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'

import { fetchCashReceipt } from '@/api/groupCash'
import type { CashReceipt } from '@/types/api'

const props = defineProps<{ groupId: number; receipt: CashReceipt | null }>()
const emit = defineEmits<{ (e: 'close'): void }>()

const url = ref('')
const loading = ref(false)
const err = ref('')

const isImage = computed(() => !!props.receipt?.content_type.startsWith('image/'))

function revoke() {
  if (url.value) URL.revokeObjectURL(url.value)
  url.value = ''
}

watch(
  () => props.receipt,
  async (r) => {
    revoke()
    err.value = ''
    if (!r) return
    loading.value = true
    try {
      const blob = await fetchCashReceipt(props.groupId, r.id)
      url.value = URL.createObjectURL(new Blob([blob], { type: r.content_type }))
    } catch {
      err.value = 'Beleg konnte nicht geladen werden.'
    } finally {
      loading.value = false
    }
  },
)

onUnmounted(revoke)
</script>

<template>
  <Dialog
    :visible="!!receipt"
    modal
    :header="receipt?.filename ?? 'Beleg'"
    :style="{ width: 'min(900px, 96vw)' }"
    :breakpoints="{ '600px': '100vw' }"
    @update:visible="(v: boolean) => !v && emit('close')"
  >
    <p v-if="loading" class="muted">Laden…</p>
    <p v-else-if="err" class="err">{{ err }}</p>
    <template v-else-if="url">
      <img v-if="isImage" :src="url" :alt="receipt?.filename" class="preview" data-testid="receipt-image" />
      <iframe v-else :src="url" class="pdf" :title="receipt?.filename" />
    </template>
    <template #footer>
      <a v-if="url" :href="url" target="_blank" rel="noopener" class="p-button p-button-text">Im neuen Tab öffnen</a>
      <a v-if="url" :href="url" :download="receipt?.filename" class="p-button p-button-secondary p-button-outlined">Herunterladen</a>
      <Button label="Schließen" @click="emit('close')" />
    </template>
  </Dialog>
</template>

<style scoped>
.preview {
  display: block;
  max-width: 100%;
  max-height: 75vh;
  margin: 0 auto;
}
.pdf {
  width: 100%;
  height: 75vh;
  border: 1px solid #e2e8f0;
}
.p-button {
  text-decoration: none;
}
.muted {
  color: #64748b;
}
.err {
  color: #b91c1c;
}
</style>
