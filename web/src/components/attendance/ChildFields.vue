<script setup lang="ts">
import { computed, ref, watch } from 'vue'

/** Eingabefelder für ein Kind: Vorname(n), Nachname (gespeichert wird nur der Anfangsbuchstabe), Geburtsmonat. */
const first = defineModel<string>('first', { required: true })
const last = defineModel<string>('last', { required: true })
/** Geburtsmonat YYYY-MM; '' = nicht angegeben bzw. unvollständig. */
const birth = defineModel<string>('birth', { required: true })

defineProps<{ testid?: string }>()
const emit = defineEmits<{ submit: [] }>()

const MONTHS = [
  'Januar',
  'Februar',
  'März',
  'April',
  'Mai',
  'Juni',
  'Juli',
  'August',
  'September',
  'Oktober',
  'November',
  'Dezember',
]
const thisYear = new Date().getFullYear()
const month = ref('')
const year = ref('')
const years = computed(() => {
  const list = Array.from({ length: 10 }, (_, i) => String(thisYear - i))
  if (year.value && !list.includes(year.value)) list.push(year.value)
  return list
})
watch(
  birth,
  (v) => {
    const [y, m] = (v ?? '').split('-')
    if (v) {
      year.value = y ?? ''
      month.value = m ?? ''
    }
  },
  { immediate: true },
)
watch([month, year], ([m, y]) => {
  birth.value = m && y ? `${y}-${m}` : ''
})

const incomplete = computed(() => !!month.value !== !!year.value)
const firstInput = ref<HTMLInputElement | null>(null)
defineExpose({
  focus: () => firstInput.value?.focus(),
  reset: () => {
    month.value = ''
    year.value = ''
  },
})
</script>

<template>
  <div class="fields" :data-testid="testid">
    <label class="f-first">
      <span>Vorname(n)</span>
      <input
        ref="firstInput"
        v-model="first"
        type="text"
        maxlength="80"
        placeholder="z. B. Anna Maria"
        data-testid="child-first"
        @keydown.enter.prevent="emit('submit')"
      />
    </label>
    <label class="f-last">
      <span>Nachname</span>
      <input
        v-model="last"
        type="text"
        maxlength="80"
        placeholder="z. B. Schulz"
        data-testid="child-last"
        @keydown.enter.prevent="emit('submit')"
      />
    </label>
    <div class="f-birth">
      <span>Geburtsmonat</span>
      <div class="birth-row">
        <select v-model="month" aria-label="Geburtsmonat" data-testid="child-birth-month">
          <option value="">Monat</option>
          <option v-for="(name, i) in MONTHS" :key="name" :value="String(i + 1).padStart(2, '0')">{{ name }}</option>
        </select>
        <select v-model="year" aria-label="Geburtsjahr" data-testid="child-birth-year">
          <option value="">Jahr</option>
          <option v-for="y in years" :key="y" :value="y">{{ y }}</option>
        </select>
      </div>
    </div>
    <small class="hint" :class="{ warn: incomplete }">
      <template v-if="incomplete">Bitte Monat und Jahr wählen.</template>
      <template v-else>Vom Nachnamen speichert die App nur den Anfangsbuchstaben („Anna Maria S.“).</template>
    </small>
  </div>
</template>

<style scoped>
.fields {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr) minmax(0, 1.5fr);
  gap: 0.5rem;
  width: 100%;
}
label,
.f-birth {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  font-size: 0.78rem;
  color: #475569;
  min-width: 0;
}
.birth-row {
  display: grid;
  grid-template-columns: 1.3fr 1fr;
  gap: 0.3rem;
}
input,
select {
  font: inherit;
  font-size: 0.95rem;
  padding: 0.45rem 0.55rem;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  background: #fff;
  min-width: 0;
  width: 100%;
  color: #0f172a;
}
.hint {
  grid-column: 1 / -1;
  font-size: 0.75rem;
  color: #64748b;
}
.hint.warn {
  color: #b45309;
}
@media (max-width: 768px) {
  .fields {
    grid-template-columns: 1fr 1fr;
  }
  .f-birth {
    grid-column: 1 / -1;
  }
}
</style>
