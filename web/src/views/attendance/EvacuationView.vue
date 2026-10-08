<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'

import { childName, fetchEvacuation, type EvacuationData } from '@/api/attendance'
import { getApiErrorMessage } from '@/utils/apiError'

/**
 * Evakuierung: alle jetzt anwesenden Kinder (gekommen, nicht gegangen) und alle eingestempelten
 * Mitarbeitenden zum Abhaken. Haken bleiben nur auf diesem Gerät (auch nach Neuladen).
 */
const router = useRouter()
const data = ref<EvacuationData | null>(null)
const error = ref('')
const loading = ref(false)
const checked = ref<Record<string, boolean>>({})

const storageKey = computed(() => `nfc_evacuation_${data.value?.date ?? ''}`)

async function load() {
  loading.value = true
  try {
    data.value = await fetchEvacuation()
    error.value = ''
    try {
      checked.value = JSON.parse(sessionStorage.getItem(storageKey.value) ?? '{}')
    } catch {
      checked.value = {}
    }
  } catch (e) {
    error.value = getApiErrorMessage(e) ?? 'Laden fehlgeschlagen'
  } finally {
    loading.value = false
  }
}
onMounted(load)

watch(
  checked,
  (v) => {
    try {
      sessionStorage.setItem(storageKey.value, JSON.stringify(v))
    } catch {
      /* ignore */
    }
  },
  { deep: true },
)

function toggle(key: string) {
  checked.value = { ...checked.value, [key]: !checked.value[key] }
}

const keys = computed(() => [
  ...(data.value?.groups.flatMap((g) => g.children.map((c) => `c${c.id}`)) ?? []),
  ...(data.value?.staff.map((s) => `s${s.id}`) ?? []),
])
const total = computed(() => keys.value.length)
const done = computed(() => keys.value.filter((k) => checked.value[k]).length)
const childCount = computed(() => data.value?.groups.reduce((n, g) => n + g.children.length, 0) ?? 0)

function reset() {
  if (!window.confirm('Alle Haken entfernen?')) return
  checked.value = {}
}

function groupDone(g: EvacuationData['groups'][number]) {
  return g.children.filter((c) => checked.value[`c${c.id}`]).length
}
</script>

<template>
  <div class="evac">
    <div class="top">
      <button type="button" class="back" @click="router.push({ name: 'attendance' })">
        <span class="pi pi-arrow-left" aria-hidden="true" /> Anwesenheit
      </button>
      <Button icon="pi pi-refresh" label="Aktualisieren" text size="small" :loading="loading" @click="load" />
    </div>

    <div class="banner" :class="{ 'banner--done': total > 0 && done === total }" data-testid="evac-banner">
      <div class="big">
        <b data-testid="evac-done">{{ done }}</b> von <b data-testid="evac-total">{{ total }}</b> abgehakt
      </div>
      <div class="meta">
        {{ childCount }} Kinder · {{ data?.staff.length ?? 0 }} Mitarbeitende · Stand {{ data?.time ?? '–' }} Uhr
      </div>
      <div class="progress" aria-hidden="true">
        <span :style="{ width: total ? `${(done / total) * 100}%` : '0%' }" />
      </div>
    </div>

    <p v-if="error" class="error">{{ error }}</p>

    <section v-for="g in data?.groups ?? []" :key="g.id" class="block">
      <h2>
        {{ g.name }} <small>{{ groupDone(g) }}/{{ g.children.length }}</small>
      </h2>
      <p v-if="!g.children.length" class="muted">Keine Kinder als anwesend eingetragen.</p>
      <ul class="list">
        <li v-for="c in g.children" :key="c.id">
          <button
            type="button"
            :class="['row', { on: checked[`c${c.id}`] }]"
            :aria-pressed="!!checked[`c${c.id}`]"
            :data-testid="`evac-child-${c.id}`"
            @click="toggle(`c${c.id}`)"
          >
            <span class="box"><span v-if="checked[`c${c.id}`]" class="pi pi-check" aria-hidden="true" /></span>
            <span class="label">{{ childName(c) }}</span>
          </button>
        </li>
      </ul>
    </section>

    <section class="block">
      <h2>
        Mitarbeitende im Haus <small>{{ data?.staff.filter((s) => checked[`s${s.id}`]).length ?? 0 }}/{{ data?.staff.length ?? 0 }}</small>
      </h2>
      <p class="muted small">Alle, die jetzt eingestempelt und noch nicht ausgestempelt sind.</p>
      <p v-if="data && !data.staff.length" class="muted">Niemand eingestempelt.</p>
      <ul class="list">
        <li v-for="s in data?.staff ?? []" :key="s.id">
          <button
            type="button"
            :class="['row', { on: checked[`s${s.id}`] }]"
            :aria-pressed="!!checked[`s${s.id}`]"
            :data-testid="`evac-staff-${s.id}`"
            @click="toggle(`s${s.id}`)"
          >
            <span class="box"><span v-if="checked[`s${s.id}`]" class="pi pi-check" aria-hidden="true" /></span>
            <span class="label">{{ s.display_name }}</span>
            <small v-if="s.group_name" class="grp">{{ s.group_name }}</small>
          </button>
        </li>
      </ul>
    </section>

    <div class="foot">
      <Button label="Haken zurücksetzen" severity="secondary" outlined size="small" :disabled="!done" @click="reset" />
    </div>
  </div>
</template>

<style scoped>
.evac {
  max-width: 900px;
  margin: 0 auto;
}
.top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.6rem;
}
.back {
  border: none;
  background: transparent;
  color: #334155;
  font: inherit;
  cursor: pointer;
  padding: 0.4rem 0;
}
.banner {
  position: sticky;
  top: calc(var(--layout-top-inset, 0px) + 0.25rem);
  z-index: 5;
  background: #b91c1c;
  color: #fff;
  border-radius: 14px;
  padding: 0.9rem 1rem;
  box-shadow: 0 6px 18px rgba(185, 28, 28, 0.25);
}
.banner--done {
  background: #15803d;
  box-shadow: 0 6px 18px rgba(21, 128, 61, 0.25);
}
.big {
  font-size: 1.5rem;
}
.big b {
  font-size: 1.9rem;
}
.meta {
  font-size: 0.9rem;
  opacity: 0.9;
  margin-top: 0.15rem;
}
.progress {
  height: 6px;
  background: rgba(255, 255, 255, 0.3);
  border-radius: 3px;
  margin-top: 0.6rem;
  overflow: hidden;
}
.progress span {
  display: block;
  height: 100%;
  background: #fff;
  transition: width 0.2s ease;
}
.block h2 {
  font-size: 1.1rem;
  margin: 1.2rem 0 0.5rem;
}
.block h2 small {
  font-weight: 500;
  color: #64748b;
  margin-left: 0.3rem;
}
.list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 0.5rem;
}
.row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  border: 2px solid #e2e8f0;
  background: #fff;
  border-radius: 12px;
  padding: 0.8rem 0.9rem;
  font: inherit;
  font-size: 1.1rem;
  text-align: left;
  cursor: pointer;
  min-height: 3.6rem;
}
.row.on {
  border-color: #16a34a;
  background: #f0fdf4;
}
.row.on .label {
  color: #15803d;
}
.box {
  flex: 0 0 auto;
  width: 1.8rem;
  height: 1.8rem;
  border-radius: 8px;
  border: 2px solid #94a3b8;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #fff;
}
.row.on .box {
  background: #16a34a;
  border-color: #16a34a;
  color: #fff;
}
.label {
  flex: 1;
  font-weight: 600;
  color: #0f172a;
}
.grp {
  color: #64748b;
  font-size: 0.8rem;
}
.muted {
  color: #64748b;
}
.small {
  font-size: 0.85rem;
  margin-top: -0.3rem;
}
.error {
  color: #b91c1c;
}
.foot {
  margin: 1.5rem 0 1rem;
  display: flex;
  justify-content: center;
}
@media (max-width: 600px) {
  .list {
    grid-template-columns: 1fr;
  }
}
</style>
