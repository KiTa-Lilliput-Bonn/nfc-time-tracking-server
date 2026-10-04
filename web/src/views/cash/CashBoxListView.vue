<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Card from 'primevue/card'

import { fetchCashBoxes } from '@/api/groupCash'
import type { CashBoxListItem } from '@/types/api'
import { useAuthStore } from '@/stores/auth'
import { formatEuro } from '@/utils/money'

const router = useRouter()
const auth = useAuthStore()
const boxes = ref<CashBoxListItem[]>([])
const loading = ref(true)
const err = ref('')

const isLeitung = computed(() => auth.role === 'leitung' || auth.role === 'superadmin')

onMounted(async () => {
  try {
    boxes.value = await fetchCashBoxes()
    // Kassenwarte mit genau einer Kasse landen direkt in ihrer Kasse.
    if (!isLeitung.value && boxes.value.length === 1) {
      await router.replace({ name: 'cash-box', params: { groupId: boxes.value[0].group_id } })
    }
  } catch {
    err.value = 'Gruppenkassen konnten nicht geladen werden.'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="page">
    <p v-if="err" class="err">{{ err }}</p>
    <p v-else-if="loading" class="muted">Laden…</p>
    <p v-else-if="!boxes.length" class="muted">
      <template v-if="isLeitung">Es gibt noch keine Gruppen. Gruppen werden unter „Gruppen“ angelegt.</template>
      <template v-else>Sie sind für keine Gruppenkasse als Kassenwart eingetragen.</template>
    </p>
    <div v-else class="grid">
      <RouterLink
        v-for="b in boxes"
        :key="b.group_id"
        :to="{ name: 'cash-box', params: { groupId: b.group_id } }"
        class="box-link"
        data-testid="cash-box-card"
      >
        <Card class="box">
          <template #title>{{ b.group_name }}</template>
          <template #content>
            <dl class="figures">
              <div>
                <dt>Kassenstand</dt>
                <dd :class="{ neg: b.summary.balance_cents < 0 }">{{ formatEuro(b.summary.balance_cents) }}</dd>
              </div>
              <div>
                <dt>Ansparkonto</dt>
                <dd>{{ formatEuro(b.summary.savings_cents) }}</dd>
              </div>
            </dl>
            <p class="muted small">
              Kassenwarte:
              <template v-if="b.keepers.length">{{ b.keepers.map((k) => k.display_name).join(', ') }}</template>
              <template v-else>noch keine</template>
            </p>
          </template>
        </Card>
      </RouterLink>
    </div>
  </div>
</template>

<style scoped>
.page {
  max-width: 960px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 1rem;
}
.box-link {
  text-decoration: none;
  color: inherit;
}
.box {
  height: 100%;
  transition: box-shadow 0.15s ease;
}
.box-link:hover .box {
  box-shadow: 0 4px 16px rgba(15, 23, 42, 0.12);
}
.figures {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.5rem;
  margin: 0 0 0.75rem;
}
.figures dt {
  font-size: 0.8rem;
  color: #64748b;
}
.figures dd {
  margin: 0;
  font-size: 1.2rem;
  font-weight: 600;
  color: #0f172a;
}
.figures dd.neg {
  color: #b91c1c;
}
.muted {
  color: #64748b;
}
.small {
  font-size: 0.85rem;
  margin: 0;
}
.err {
  color: #b91c1c;
}
</style>
