<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import { useToast } from 'primevue/usetoast'

import { fetchEmployees } from '@/api/employees'
import { fetchGroups } from '@/api/groups'
import {
  deleteChildPattern,
  fetchKibizPlanningBasis,
  putChildPattern,
  putKibizOptions,
  putKibizRates,
  putQualification,
} from '@/api/management'
import type {
  CareHours,
  ChildCount,
  ChildPattern,
  Employee,
  GroupForm,
  KibizOptions,
  KibizRate,
  Qualification,
  UserGroup,
} from '@/types/api'
import { getApiErrorMessage } from '@/utils/apiError'
import { formatGermanDate, isoWeekAndYear, mondayOfISOWeek, toISODateLocal } from '@/utils/dates'
import {
  CARE_HOURS,
  GROUP_FORMS,
  GROUP_FORM_LABELS,
  QUALIFICATION_LABELS,
  childCategoryLabel,
  childKey,
  childTotal,
  effectiveQualification,
  normalizeChildCounts,
} from '@/utils/kibiz'

/** Planungsgrundlagen der Leitung für die KiBiz-Rechnung im Dienstplan. */

type Tab = 'people' | 'children' | 'rates' | 'options'
const TABS: { k: Tab; l: string }[] = [
  { k: 'people', l: 'Personen' },
  { k: 'children', l: 'Kinder' },
  { k: 'rates', l: 'KiBiz' },
  { k: 'options', l: 'Optionen' },
]

const toast = useToast()
const route = useRoute()
const router = useRouter()

const tab = ref<Tab>(TABS.some((t) => t.k === route.query.tab) ? (route.query.tab as Tab) : 'people')
watch(tab, (t) => void router.replace({ query: { ...route.query, tab: t } }))

const loading = ref(true)
const loadError = ref('')
const employees = ref<Employee[]>([])
const groups = ref<UserGroup[]>([])
const qualifications = ref<Record<number, Qualification>>({})
const rates = ref<KibizRate[]>([])
const patterns = ref<ChildPattern[]>([])
const options = ref<KibizOptions>({ count_team_meetings: false })

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const [emps, grps, basis] = await Promise.all([fetchEmployees(), fetchGroups(), fetchKibizPlanningBasis()])
    employees.value = emps
    groups.value = grps
    qualifications.value = Object.fromEntries(basis.qualifications.map((q) => [q.user_id, q.qualification]))
    rates.value = basis.rates.map((r) => ({ ...r }))
    patterns.value = basis.child_patterns
    options.value = basis.options
  } catch (e) {
    loadError.value = getApiErrorMessage(e) ?? 'Planungsgrundlagen konnten nicht geladen werden.'
  } finally {
    loading.value = false
  }
}

onMounted(load)

function fail(e: unknown, fallback: string) {
  toast.add({ severity: 'error', summary: getApiErrorMessage(e) ?? fallback, life: 5000 })
}

// ---------- Personen ----------

const peopleSections = computed(() => {
  const list = employees.value
    .filter((e) => e.active && e.role !== 'superadmin')
    .sort((a, b) => a.display_name.localeCompare(b.display_name, 'de'))
  const out = groups.value
    .map((g) => ({ title: g.name, employees: list.filter((e) => e.group_id === g.id) }))
    .filter((s) => s.employees.length)
  const known = new Set(groups.value.map((g) => g.id))
  const orphan = list.filter((e) => e.group_id == null || !known.has(e.group_id))
  if (orphan.length) out.push({ title: 'Ohne Gruppe (zählt keiner Gruppe)', employees: orphan })
  return out
})

const missingCount = computed(
  () => employees.value.filter((e) => e.active && e.role !== 'superadmin' && e.group_id != null && !effectiveQualification(e, qualifications.value)).length,
)

async function setQualification(uid: number, value: string) {
  const q = value as Qualification | ''
  const before = qualifications.value[uid]
  if (q) qualifications.value[uid] = q
  else delete qualifications.value[uid]
  try {
    await putQualification(uid, q)
  } catch (e) {
    if (before) qualifications.value[uid] = before
    else delete qualifications.value[uid]
    fail(e, 'Qualifikation konnte nicht gespeichert werden.')
  }
}

// ---------- Kinder ----------

const todayISO = toISODateLocal(new Date())
const thisMonday = (() => {
  const w = isoWeekAndYear(new Date())
  return toISODateLocal(mondayOfISOWeek(w.year, w.week))
})()

function versionsOf(gid: number): ChildPattern[] {
  return patterns.value.filter((p) => p.group_id === gid).sort((a, b) => b.valid_from.localeCompare(a.valid_from))
}

function currentVersion(gid: number): ChildPattern | null {
  return versionsOf(gid).find((p) => p.valid_from <= todayISO) ?? null
}

const editGroupId = ref<number | null>(null)
const editValidFrom = ref('')
const editRows = ref<ChildCount[]>([])
const addForm = ref<GroupForm>('III')
const addHours = ref<CareHours>(35)
const patternSaving = ref(false)

function startEdit(gid: number, from?: ChildPattern) {
  const base = from ?? currentVersion(gid) ?? versionsOf(gid)[0] ?? null
  editGroupId.value = gid
  editValidFrom.value = from ? from.valid_from : thisMonday
  editRows.value = base ? base.counts.map((c) => ({ ...c })) : [{ group_form: 'III', care_hours: 35, count: 0 }]
}

function step(c: ChildCount, d: number) {
  c.count = Math.max(0, Math.min(200, (Number(c.count) || 0) + d))
}

function addRow() {
  const c: ChildCount = { group_form: addForm.value, care_hours: addHours.value, count: 1 }
  if (editRows.value.some((r) => childKey(r) === childKey(c))) return
  editRows.value = [...editRows.value, c]
}

const editValid = computed(() => /^\d{4}-\d{2}-\d{2}$/.test(editValidFrom.value))

async function savePattern() {
  if (editGroupId.value == null || !editValid.value) return
  patternSaving.value = true
  const p: ChildPattern = {
    group_id: editGroupId.value,
    valid_from: editValidFrom.value,
    counts: normalizeChildCounts(editRows.value.map((c) => ({ ...c, count: Number(c.count) || 0 }))),
  }
  try {
    await putChildPattern(p)
    patterns.value = [...patterns.value.filter((x) => !(x.group_id === p.group_id && x.valid_from === p.valid_from)), p]
    editGroupId.value = null
    toast.add({ severity: 'success', summary: 'Kinder-Wochenmuster gespeichert', life: 2500 })
  } catch (e) {
    fail(e, 'Muster konnte nicht gespeichert werden.')
  } finally {
    patternSaving.value = false
  }
}

async function removeVersion(p: ChildPattern) {
  if (!window.confirm(`Muster ab ${formatGermanDate(p.valid_from)} löschen?`)) return
  try {
    await deleteChildPattern(p.group_id, p.valid_from)
    patterns.value = patterns.value.filter((x) => x !== p)
  } catch (e) {
    fail(e, 'Muster konnte nicht gelöscht werden.')
  }
}

function countsSummary(counts: ChildCount[]): string {
  return normalizeChildCounts(counts)
    .map((c) => `${c.count}× ${childCategoryLabel(c)}`)
    .join(', ')
}

// ---------- KiBiz-Tabelle ----------

const ratesSaving = ref(false)

function rateEk(r: KibizRate): number {
  return Math.max(0, (Number(r.total_hours) || 0) - (Number(r.fachkraft_min_hours) || 0))
}

function fmt(n: number): string {
  return n.toLocaleString('de-DE', { maximumFractionDigits: 2 })
}

async function saveRates() {
  ratesSaving.value = true
  try {
    rates.value = await putKibizRates(
      rates.value.map((r) => ({
        ...r,
        children: Number(r.children),
        leitung_hours: Number(r.leitung_hours),
        total_hours: Number(r.total_hours),
        fachkraft_min_hours: Number(r.fachkraft_min_hours),
      })),
    )
    toast.add({ severity: 'success', summary: 'KiBiz-Tabelle gespeichert', life: 2500 })
  } catch (e) {
    fail(e, 'KiBiz-Tabelle konnte nicht gespeichert werden.')
  } finally {
    ratesSaving.value = false
  }
}

// ---------- Optionen ----------

async function saveOptions() {
  try {
    options.value = await putKibizOptions(options.value)
  } catch (e) {
    fail(e, 'Optionen konnten nicht gespeichert werden.')
  }
}
</script>

<template>
  <div class="basis" data-testid="planning-basis">
    <div class="top">
      <Button icon="pi pi-arrow-left" text rounded aria-label="Zurück zum Dienstplan" @click="router.push({ name: 'schedule' })" />
      <div>
        <p>Daraus rechnet der Dienstplan je Gruppe die nötigen Fachkraft- und Personalstunden nach KiBiz.</p>
      </div>
    </div>

    <div class="seg" role="tablist">
      <button
        v-for="t in TABS"
        :key="t.k"
        type="button"
        role="tab"
        :aria-selected="tab === t.k"
        :class="{ on: tab === t.k }"
        :data-testid="'basis-tab-' + t.k"
        @click="tab = t.k"
      >
        {{ t.l }}
      </button>
    </div>

    <p v-if="loadError" class="err">{{ loadError }}</p>
    <p v-else-if="loading" class="muted">Laden…</p>

    <!-- Personen -->
    <template v-else-if="tab === 'people'">
      <p class="intro">
        Fachkräfte und Ergänzungskräfte zählen für ihre Stammgruppe, unabhängig vom Konto. Wer mit Leitungskonto in der
        Gruppe arbeitet, bekommt „Fachkraft“. Leitung, Hauswirtschaft und Sonstige (z. B. Praktikum) zählen nicht.
        <template v-if="missingCount"><br /><b>{{ missingCount }} Personen</b> in Gruppen haben noch keine Angabe.</template>
      </p>
      <section v-for="sec in peopleSections" :key="sec.title" class="card">
        <h3>{{ sec.title }}</h3>
        <label v-for="e in sec.employees" :key="e.id" class="row">
          <span class="name">
            {{ e.display_name }}
            <small v-if="e.role === 'leitung'">Leitungskonto</small>
          </span>
          <select
            :value="effectiveQualification(e, qualifications) ?? ''"
            :class="{ missing: !effectiveQualification(e, qualifications) && e.group_id != null }"
            :data-testid="'qualification-' + e.id"
            @change="setQualification(e.id, ($event.target as HTMLSelectElement).value)"
          >
            <option value="">– nicht angegeben –</option>
            <option v-for="(l, k) in QUALIFICATION_LABELS" :key="k" :value="k">{{ l }}</option>
          </select>
        </label>
      </section>
    </template>

    <!-- Kinder -->
    <template v-else-if="tab === 'children'">
      <p class="intro">
        Festes Wochenmuster je Gruppe: wie viele Kinder welcher Gruppenform mit welcher gebuchten Betreuungszeit.
        Für einzelne Tage oder Wochen passt du die Zahl direkt im Dienstplan an. Neue Kinder oder Wechsel (z. B. mit dem
        3. Geburtstag) trägst du als neues Muster mit „gültig ab“ ein.
      </p>
      <section v-for="g in groups" :key="g.id" class="card" :data-testid="'pattern-group-' + g.id">
        <h3>
          {{ g.name }}
          <span v-if="currentVersion(g.id)" class="tot">{{ childTotal(currentVersion(g.id)!.counts) }} Kinder</span>
        </h3>

        <template v-if="editGroupId === g.id">
          <ul class="rows">
            <li v-for="c in editRows" :key="childKey(c)">
              <span class="cat">{{ childCategoryLabel(c) }}</span>
              <button type="button" class="st" :aria-label="`${childCategoryLabel(c)} weniger`" @click="step(c, -1)">−</button>
              <input
                v-model.number="c.count"
                type="number"
                min="0"
                max="200"
                inputmode="numeric"
                class="n"
                :data-testid="`pattern-count-${c.group_form}-${c.care_hours}`"
              />
              <button type="button" class="st" :aria-label="`${childCategoryLabel(c)} mehr`" @click="step(c, 1)">+</button>
            </li>
          </ul>
          <div class="add">
            <select v-model="addForm" aria-label="Gruppenform">
              <option v-for="f in GROUP_FORMS" :key="f" :value="f">{{ GROUP_FORM_LABELS[f] }}</option>
            </select>
            <select v-model.number="addHours" aria-label="Betreuungszeit">
              <option v-for="h in CARE_HOURS" :key="h" :value="h">{{ h }} h</option>
            </select>
            <Button label="Hinzufügen" size="small" text icon="pi pi-plus" @click="addRow" />
          </div>
          <label class="vf">
            Gültig ab
            <input v-model="editValidFrom" type="date" data-testid="pattern-valid-from" />
          </label>
          <div class="btns">
            <Button label="Abbrechen" severity="secondary" outlined @click="editGroupId = null" />
            <Button
              label="Speichern"
              data-testid="pattern-save"
              :disabled="!editValid || patternSaving"
              :loading="patternSaving"
              @click="savePattern"
            />
          </div>
        </template>
        <template v-else>
          <p v-if="!versionsOf(g.id).length" class="muted">Noch kein Muster. Ohne Kinder rechnet der Dienstplan keinen Bedarf.</p>
          <ul v-else class="versions">
            <li v-for="p in versionsOf(g.id)" :key="p.valid_from" :class="{ cur: p === currentVersion(g.id) }">
              <span>
                <b>ab {{ formatGermanDate(p.valid_from) }}</b> · {{ childTotal(p.counts) }} Kinder
                <small>{{ countsSummary(p.counts) || 'keine Kinder' }}</small>
              </span>
              <Button icon="pi pi-pencil" text rounded size="small" aria-label="Bearbeiten" @click="startEdit(g.id, p)" />
              <Button icon="pi pi-trash" text rounded size="small" severity="danger" aria-label="Löschen" @click="removeVersion(p)" />
            </li>
          </ul>
          <Button
            :label="versionsOf(g.id).length ? 'Neues Muster ab …' : 'Muster anlegen'"
            size="small"
            outlined
            icon="pi pi-plus"
            :data-testid="'pattern-new-' + g.id"
            @click="startEdit(g.id)"
          />
        </template>
      </section>
      <p v-if="!groups.length" class="muted">Es gibt noch keine Gruppen.</p>
    </template>

    <!-- KiBiz-Tabelle -->
    <template v-else-if="tab === 'rates'">
      <p class="intro">
        Werte je Gruppe und Woche aus der Anlage zu § 33 KiBiz (Fassung ab 01.08.2020, gilt bis 31.07.2027). Jedes Kind
        zählt mit seinem Anteil: Stunden ÷ Kinderzahl. Von den Gesamtstunden müssen mindestens die Fachkraftstunden von
        Fachkräften kommen, den Rest dürfen Ergänzungskräfte übernehmen. Leitungsstunden zählen nicht zur Betreuung.
      </p>
      <section class="card rates">
        <table>
          <thead>
            <tr>
              <th>Form</th>
              <th>Kinder</th>
              <th>Gesamt h</th>
              <th>davon Fachkraft h</th>
              <th>Rest h</th>
              <th>Leitung h</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in rates" :key="r.group_form + r.care_hours">
              <th>{{ childCategoryLabel(r) }}</th>
              <td><input v-model.number="r.children" type="number" min="1" step="1" inputmode="decimal" /></td>
              <td><input v-model.number="r.total_hours" type="number" min="0" step="0.5" inputmode="decimal" /></td>
              <td><input v-model.number="r.fachkraft_min_hours" type="number" min="0" step="0.5" inputmode="decimal" /></td>
              <td class="ek">{{ fmt(rateEk(r)) }}</td>
              <td><input v-model.number="r.leitung_hours" type="number" min="0" step="0.5" inputmode="decimal" /></td>
            </tr>
          </tbody>
        </table>
        <div class="btns">
          <Button label="Tabelle speichern" :loading="ratesSaving" :disabled="ratesSaving" @click="saveRates" />
        </div>
      </section>
    </template>

    <!-- Optionen -->
    <template v-else>
      <section class="card">
        <label class="opt">
          <input v-model="options.count_team_meetings" type="checkbox" @change="saveOptions" />
          <span>
            <b>Teamsitzungen als Betreuung zählen</b>
            <small>Aus: Zeit in Teamsitzungen wird von der Schicht abgezogen.</small>
          </span>
        </label>
      </section>
    </template>
  </div>
</template>

<style scoped>
.basis {
  max-width: 760px;
  margin: 0 auto;
  padding: 0 0 2rem;
}
.top {
  display: flex;
  gap: 0.4rem;
  align-items: flex-start;
}
.top p {
  margin: 0.45rem 0 0.7rem;
  color: #64748b;
  font-size: 0.85rem;
}
.seg {
  display: flex;
  background: #f1f5f9;
  border-radius: 12px;
  padding: 3px;
  margin-bottom: 0.8rem;
}
.seg button {
  flex: 1;
  border: none;
  background: transparent;
  border-radius: 9px;
  padding: 0.5rem 0.3rem;
  font: inherit;
  font-size: 0.9rem;
  color: #475569;
  cursor: pointer;
}
.seg button.on {
  background: #fff;
  color: #0f172a;
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.12);
}
.intro {
  font-size: 0.85rem;
  color: #475569;
  margin: 0 0 0.8rem;
}
.muted {
  color: #64748b;
  font-size: 0.85rem;
}
.err {
  color: #b91c1c;
}
.card {
  background: #fff;
  border-radius: 14px;
  padding: 0.8rem 0.9rem;
  margin-bottom: 0.8rem;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.06);
}
.card h3 {
  margin: 0 0 0.4rem;
  font-size: 0.8rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: #475569;
  display: flex;
  justify-content: space-between;
}
.tot {
  text-transform: none;
  letter-spacing: 0;
  font-weight: 500;
}
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.6rem;
  padding: 0.4rem 0;
  border-bottom: 1px solid #f1f5f9;
}
.row:last-child {
  border-bottom: none;
}
.name small {
  display: block;
  color: #64748b;
  font-size: 0.75rem;
}
select,
input[type='date'] {
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  padding: 0.4rem 0.45rem;
  background: #fff;
  font: inherit;
  font-size: 0.9rem;
  max-width: 55%;
}
select.missing {
  border-color: #f59e0b;
  background: #fffbeb;
}
.rows,
.versions {
  list-style: none;
  margin: 0 0 0.5rem;
  padding: 0;
}
.rows li {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.35rem 0;
}
.cat {
  flex: 1;
  font-weight: 600;
}
.st {
  width: 2.3rem;
  height: 2.3rem;
  border-radius: 999px;
  border: 1px solid #cbd5e1;
  background: #fff;
  font-size: 1.15rem;
  cursor: pointer;
}
input.n {
  width: 3.4rem;
  text-align: center;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  padding: 0.35rem 0.2rem;
  font: inherit;
  font-size: 1.05rem;
}
.add {
  display: flex;
  gap: 0.4rem;
  align-items: center;
  flex-wrap: wrap;
}
.vf {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  margin-top: 0.7rem;
  font-size: 0.9rem;
}
.btns {
  display: flex;
  gap: 0.6rem;
  justify-content: flex-end;
  margin-top: 0.8rem;
}
.versions li {
  display: flex;
  align-items: center;
  gap: 0.2rem;
  padding: 0.35rem 0;
  border-bottom: 1px solid #f1f5f9;
  color: #64748b;
}
.versions li.cur {
  color: #0f172a;
}
.versions li > span {
  flex: 1;
  font-size: 0.9rem;
}
.versions small {
  display: block;
  font-size: 0.78rem;
  color: #64748b;
}
.rates {
  overflow-x: auto;
}
.rates table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.85rem;
}
.rates th,
.rates td {
  padding: 0.3rem 0.25rem;
  text-align: left;
  white-space: nowrap;
}
.rates thead th {
  font-size: 0.72rem;
  color: #64748b;
  font-weight: 600;
  white-space: normal;
}
.rates input {
  width: 3.4rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  padding: 0.25rem 0.3rem;
  font: inherit;
}
.rates .ek {
  color: #64748b;
}
.opt {
  display: flex;
  gap: 0.7rem;
  align-items: flex-start;
  padding: 0.5rem 0;
}
.opt input {
  margin-top: 0.25rem;
  width: 1.1rem;
  height: 1.1rem;
}
.opt small {
  display: block;
  color: #64748b;
  font-size: 0.8rem;
}
</style>
