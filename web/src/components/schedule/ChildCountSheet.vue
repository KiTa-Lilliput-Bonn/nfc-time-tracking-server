<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Drawer from 'primevue/drawer'

import type { CareHours, ChildCount, GroupForm } from '@/types/api'
import {
  CARE_HOURS,
  GROUP_FORMS,
  childCategoryLabel,
  childKey,
  childTotal,
  normalizeChildCounts,
  sameChildCounts,
} from '@/utils/kibiz'

/** Kinderzahl einer Gruppe für einen Tag oder die ganze Woche anpassen (Abweichung vom Wochenmuster). */
const props = defineProps<{
  groupName: string
  /** z. B. „Mi 07.10.“; leer = nur ganze Woche möglich */
  dayLabel: string
  /** Aktuelle Kinder (Tag bzw. Montag der Woche) */
  counts: ChildCount[]
  pattern: ChildCount[]
  /** Weicht der Tag bzw. irgendein Tag der Woche vom Muster ab? */
  adjusted: boolean
  saving: boolean
}>()

const visible = defineModel<boolean>('visible', { default: false })

const emit = defineEmits<{
  save: [counts: ChildCount[], scope: 'day' | 'week']
  reset: [scope: 'day' | 'week']
}>()

const rows = ref<ChildCount[]>([])
const scope = ref<'day' | 'week'>('day')
const addForm = ref<GroupForm>('III')
const addHours = ref<CareHours>(35)

watch(
  () => [visible.value, props.counts, props.pattern] as const,
  ([v]) => {
    if (!v) return
    const byKey = new Map<string, ChildCount>()
    for (const c of [...props.pattern, ...props.counts]) byKey.set(childKey(c), { ...c, count: 0 })
    for (const c of props.counts) byKey.set(childKey(c), { ...c })
    rows.value = normalizeChildCountsKeepZero([...byKey.values()])
    scope.value = props.dayLabel ? 'day' : 'week'
  },
  { immediate: true },
)

function normalizeChildCountsKeepZero(list: ChildCount[]): ChildCount[] {
  return [...list].sort(
    (a, b) => GROUP_FORMS.indexOf(a.group_form) - GROUP_FORMS.indexOf(b.group_form) || a.care_hours - b.care_hours,
  )
}

function patternCount(c: ChildCount): number {
  return props.pattern.find((p) => childKey(p) === childKey(c))?.count ?? 0
}

function step(c: ChildCount, d: number) {
  c.count = Math.max(0, Math.min(200, c.count + d))
}

const addable = computed(() =>
  GROUP_FORMS.flatMap((f) => CARE_HOURS.map((h) => ({ group_form: f, care_hours: h }))).filter(
    (c) => !rows.value.some((r) => childKey(r) === childKey(c)),
  ),
)

function addRow() {
  const c = { group_form: addForm.value, care_hours: addHours.value, count: 1 }
  if (rows.value.some((r) => childKey(r) === childKey(c))) return
  rows.value = normalizeChildCountsKeepZero([...rows.value, c])
}

const total = computed(() => childTotal(rows.value))
const patternTotal = computed(() => childTotal(props.pattern))
const changed = computed(() => !sameChildCounts(rows.value, props.counts))
const equalsPattern = computed(() => sameChildCounts(rows.value, props.pattern))

function onSave() {
  if (equalsPattern.value) emit('reset', scope.value)
  else emit('save', normalizeChildCounts(rows.value), scope.value)
}
</script>

<template>
  <Drawer
    v-model:visible="visible"
    position="bottom"
    class="shift-sheet"
    :show-close-icon="false"
    :block-scroll="true"
    :pt="{ root: { 'data-testid': 'children-sheet' } }"
  >
    <template #container="{ closeCallback }">
      <div class="sheet">
        <button type="button" class="grab" aria-label="Schließen" @click="closeCallback" />
        <div class="head">
          <strong>Kinder · {{ groupName }}</strong>
          <span>{{ total }} Kinder<template v-if="patternTotal"> · Muster {{ patternTotal }}</template></span>
        </div>
        <p class="sub">Gruppenform und gebuchte Betreuungszeit je Kind, ohne Namen.</p>

        <ul class="rows">
          <li v-for="c in rows" :key="childKey(c)" :data-testid="`children-row-${c.group_form}-${c.care_hours}`">
            <span class="cat">
              {{ childCategoryLabel(c) }}
              <small v-if="c.count !== patternCount(c)">Muster {{ patternCount(c) }}</small>
            </span>
            <button type="button" class="st" :aria-label="`${childCategoryLabel(c)} weniger`" @click="step(c, -1)">−</button>
            <b class="n" data-testid="children-count">{{ c.count }}</b>
            <button type="button" class="st" :aria-label="`${childCategoryLabel(c)} mehr`" @click="step(c, 1)">+</button>
          </li>
        </ul>

        <div v-if="addable.length" class="add">
          <select v-model="addForm" aria-label="Gruppenform">
            <option v-for="f in GROUP_FORMS" :key="f" :value="f">Gruppenform {{ f }}</option>
          </select>
          <select v-model.number="addHours" aria-label="Betreuungszeit">
            <option v-for="h in CARE_HOURS" :key="h" :value="h">{{ h }} h</option>
          </select>
          <Button label="Hinzufügen" size="small" text icon="pi pi-plus" @click="addRow" />
        </div>

        <div v-if="dayLabel" class="scope" role="radiogroup" aria-label="Gilt für">
          <button type="button" :class="{ on: scope === 'day' }" data-testid="children-scope-day" @click="scope = 'day'">
            Nur {{ dayLabel }}
          </button>
          <button type="button" :class="{ on: scope === 'week' }" data-testid="children-scope-week" @click="scope = 'week'">
            Ganze Woche
          </button>
        </div>

        <div class="btns">
          <Button
            v-if="adjusted"
            label="Auf Muster zurücksetzen"
            severity="secondary"
            outlined
            data-testid="children-reset"
            :disabled="saving"
            @click="emit('reset', scope)"
          />
          <Button
            label="Speichern"
            class="btn-save"
            data-testid="children-save"
            :disabled="saving || (!changed && scope === 'day')"
            :loading="saving"
            @click="onSave"
          />
        </div>
      </div>
    </template>
  </Drawer>
</template>

<style scoped>
.sheet {
  padding: 0.5rem 1.1rem calc(1.25rem + env(safe-area-inset-bottom, 0px));
  max-height: 88dvh;
  overflow-y: auto;
}
.grab {
  display: block;
  width: 44px;
  height: 5px;
  border: none;
  border-radius: 3px;
  background: #cbd5e1;
  margin: 0.25rem auto 0.9rem;
  padding: 0;
  cursor: pointer;
}
.head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 0.5rem;
  flex-wrap: wrap;
}
.head strong {
  font-size: 1.15rem;
}
.head span {
  font-size: 0.85rem;
  color: #64748b;
}
.sub {
  font-size: 0.82rem;
  color: #64748b;
  margin: 0.2rem 0 0.7rem;
}
.rows {
  list-style: none;
  margin: 0;
  padding: 0;
}
.rows li {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  padding: 0.45rem 0;
  border-bottom: 1px solid #f1f5f9;
}
.cat {
  flex: 1;
  font-weight: 600;
}
.cat small {
  display: block;
  font-weight: 400;
  color: #b45309;
  font-size: 0.75rem;
}
.st {
  width: 2.4rem;
  height: 2.4rem;
  border-radius: 999px;
  border: 1px solid #cbd5e1;
  background: #fff;
  font-size: 1.2rem;
  cursor: pointer;
}
.n {
  min-width: 2ch;
  text-align: center;
  font-size: 1.15rem;
  font-variant-numeric: tabular-nums;
}
.add {
  display: flex;
  gap: 0.4rem;
  align-items: center;
  margin-top: 0.6rem;
  flex-wrap: wrap;
}
.add select {
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  padding: 0.35rem 0.4rem;
  background: #fff;
  font: inherit;
  font-size: 0.85rem;
}
.scope {
  display: flex;
  background: #f1f5f9;
  border-radius: 10px;
  padding: 3px;
  margin-top: 0.9rem;
}
.scope button {
  flex: 1;
  border: none;
  background: transparent;
  border-radius: 8px;
  padding: 0.45rem;
  font: inherit;
  font-size: 0.85rem;
  color: #475569;
  cursor: pointer;
}
.scope button.on {
  background: #fff;
  color: #0f172a;
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.12);
}
.btns {
  display: flex;
  gap: 0.6rem;
  margin-top: 0.9rem;
}
.btns :deep(.p-button) {
  flex: 1;
}
</style>
