<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import { useToast } from 'primevue/usetoast'

import {
  childName,
  createChild,
  createGroupAccount,
  deleteChild,
  deleteGroupAccount,
  fetchChildren,
  fetchGroupAccounts,
  patchChild,
  patchGroupAccount,
  resetGroupAccountPassword,
  type Child,
  type GroupAccount,
} from '@/api/attendance'
import { fetchGroups } from '@/api/groups'
import type { UserGroup } from '@/types/api'
import { getApiErrorMessage } from '@/utils/apiError'
import { copyTextToClipboard } from '@/utils/clipboard'

/** Leitung: Kinder je Gruppe pflegen und Gruppenaccounts für Geräte im Flur anlegen. */
const router = useRouter()
const toast = useToast()

const groups = ref<UserGroup[]>([])
const children = ref<Child[]>([])
const accounts = ref<GroupAccount[]>([])
const loading = ref(true)

async function load() {
  try {
    ;[groups.value, children.value, accounts.value] = await Promise.all([
      fetchGroups(),
      fetchChildren(),
      fetchGroupAccounts(),
    ])
  } catch (e) {
    fail('Laden fehlgeschlagen', e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

function fail(summary: string, e: unknown) {
  toast.add({ severity: 'error', summary, detail: getApiErrorMessage(e), life: 6000 })
}

const showInactive = ref(false)
function childrenOf(gid: number) {
  return children.value.filter((c) => c.group_id === gid && (showInactive.value || c.active))
}
const inactiveCount = computed(() => children.value.filter((c) => !c.active).length)

/** Eingabe je Gruppe: eine Zeile pro Kind, „Vorname Nachname“. */
const addText = ref<Record<number, string>>({})
const adding = ref<number | null>(null)

function parseLine(line: string): { first_name: string; last_name: string } | null {
  const t = line.trim().replace(/\s+/g, ' ')
  if (!t) return null
  if (t.includes(',')) {
    const [last, first] = t.split(',').map((s) => s.trim())
    return { first_name: first || last!, last_name: first ? last! : '' }
  }
  const i = t.indexOf(' ')
  return i < 0 ? { first_name: t, last_name: '' } : { first_name: t.slice(0, i), last_name: t.slice(i + 1) }
}

async function addChildren(gid: number) {
  const lines = (addText.value[gid] ?? '').split('\n').map(parseLine).filter((x) => !!x)
  if (!lines.length) return
  adding.value = gid
  try {
    for (const l of lines) {
      children.value.push(await createChild({ group_id: gid, ...l }))
    }
    addText.value[gid] = ''
    toast.add({ severity: 'success', summary: lines.length === 1 ? 'Kind angelegt' : `${lines.length} Kinder angelegt`, life: 2500 })
  } catch (e) {
    fail('Nicht angelegt', e)
  } finally {
    adding.value = null
  }
}

const editId = ref<number | null>(null)
const eFirst = ref('')
const eLast = ref('')
const eGroup = ref(0)

function startEdit(c: Child) {
  editId.value = c.id
  eFirst.value = c.first_name
  eLast.value = c.last_name
  eGroup.value = c.group_id
}

async function saveEdit(c: Child) {
  try {
    const u = await patchChild(c.id, { first_name: eFirst.value, last_name: eLast.value, group_id: eGroup.value })
    Object.assign(c, u)
    editId.value = null
  } catch (e) {
    fail('Nicht gespeichert', e)
  }
}

async function toggleActive(c: Child) {
  try {
    Object.assign(c, await patchChild(c.id, { active: !c.active }))
  } catch (e) {
    fail('Nicht gespeichert', e)
  }
}

async function removeChild(c: Child) {
  if (!window.confirm(`${childName(c)} mit allen Anwesenheitszeiten und Meldungen endgültig löschen?`)) return
  try {
    await deleteChild(c.id)
    children.value = children.value.filter((x) => x.id !== c.id)
    editId.value = null
  } catch (e) {
    fail('Nicht gelöscht', e)
  }
}

function accountOf(gid: number) {
  return accounts.value.find((a) => a.group_id === gid)
}

const newUser = ref<Record<number, string>>({})
const shownPassword = ref<{ gid: number; username: string; password: string } | null>(null)

function suggestUser(g: UserGroup) {
  const base = g.name
    .toLowerCase()
    .replace(/ä/g, 'ae')
    .replace(/ö/g, 'oe')
    .replace(/ü/g, 'ue')
    .replace(/ß/g, 'ss')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
  return `gruppe-${base || g.id}`
}

async function createAccount(g: UserGroup) {
  const name = (newUser.value[g.id] ?? suggestUser(g)).trim()
  try {
    const res = await createGroupAccount(g.id, name)
    accounts.value.push(res.account)
    shownPassword.value = { gid: g.id, username: res.account.username, password: res.password }
  } catch (e) {
    fail('Nicht angelegt', e)
  }
}

async function resetPassword(a: GroupAccount) {
  if (!window.confirm(`Neues Passwort für ${a.username}? Angemeldete Geräte werden abgemeldet.`)) return
  try {
    const pw = await resetGroupAccountPassword(a.id)
    shownPassword.value = { gid: a.group_id, username: a.username, password: pw }
  } catch (e) {
    fail('Nicht geändert', e)
  }
}

async function toggleAccount(a: GroupAccount) {
  try {
    Object.assign(a, await patchGroupAccount(a.id, { active: !a.active }))
  } catch (e) {
    fail('Nicht gespeichert', e)
  }
}

async function removeAccount(a: GroupAccount) {
  if (!window.confirm(`Gruppenaccount ${a.username} löschen?`)) return
  try {
    await deleteGroupAccount(a.id)
    accounts.value = accounts.value.filter((x) => x.id !== a.id)
  } catch (e) {
    fail('Nicht gelöscht', e)
  }
}

async function copyPassword() {
  if (!shownPassword.value) return
  const ok = await copyTextToClipboard(shownPassword.value.password)
  toast.add({ severity: ok ? 'success' : 'warn', summary: ok ? 'Kopiert' : 'Kopieren nicht möglich', life: 2000 })
}
</script>

<template>
  <div class="manage">
    <button type="button" class="back" @click="router.push({ name: 'attendance' })">
      <span class="pi pi-arrow-left" aria-hidden="true" /> Anwesenheit
    </button>

    <p v-if="loading" class="muted">Lädt …</p>
    <p v-else-if="!groups.length" class="muted">Zuerst unter „Gruppen“ Gruppen anlegen.</p>

    <div v-if="inactiveCount" class="opts">
      <label><input v-model="showInactive" type="checkbox" /> Abgemeldete Kinder zeigen ({{ inactiveCount }})</label>
    </div>

    <section v-for="g in groups" :key="g.id" class="group" :data-testid="`manage-group-${g.id}`">
      <h2>
        {{ g.name }} <small>{{ childrenOf(g.id).filter((c) => c.active).length }} Kinder</small>
      </h2>

      <div class="cols">
        <div class="col">
          <ul class="kids">
            <li v-for="c in childrenOf(g.id)" :key="c.id" :class="{ off: !c.active }">
              <template v-if="editId === c.id">
                <div class="edit">
                  <input v-model="eFirst" type="text" placeholder="Vorname" aria-label="Vorname" />
                  <input v-model="eLast" type="text" placeholder="Nachname" aria-label="Nachname" />
                  <select v-model.number="eGroup" aria-label="Gruppe">
                    <option v-for="og in groups" :key="og.id" :value="og.id">{{ og.name }}</option>
                  </select>
                  <div class="edit-btns">
                    <Button label="Löschen" severity="danger" text size="small" @click="removeChild(c)" />
                    <span class="grow" />
                    <Button label="Abbrechen" severity="secondary" text size="small" @click="editId = null" />
                    <Button label="Speichern" size="small" :disabled="!eFirst.trim()" @click="saveEdit(c)" />
                  </div>
                </div>
              </template>
              <template v-else>
                <span class="kid-name">{{ childName(c) }}<em v-if="!c.active"> · abgemeldet</em></span>
                <button
                  type="button"
                  class="link"
                  :title="c.active ? 'Kind erscheint nicht mehr in der Liste, Daten bleiben' : 'Wieder anmelden'"
                  @click="toggleActive(c)"
                >
                  {{ c.active ? 'Abmelden' : 'Anmelden' }}
                </button>
                <button type="button" class="icon" :aria-label="`${childName(c)} bearbeiten`" @click="startEdit(c)">
                  <span class="pi pi-pencil" aria-hidden="true" />
                </button>
              </template>
            </li>
          </ul>
          <div class="add">
            <textarea
              v-model="addText[g.id]"
              rows="2"
              placeholder="Kinder hinzufügen: eine Zeile pro Kind, z. B. „Mia Schulz“"
              :data-testid="`manage-add-${g.id}`"
            />
            <Button
              label="Hinzufügen"
              icon="pi pi-plus"
              size="small"
              :loading="adding === g.id"
              :disabled="!(addText[g.id] ?? '').trim()"
              :data-testid="`manage-add-btn-${g.id}`"
              @click="addChildren(g.id)"
            />
          </div>
        </div>

        <div class="col account">
          <h3>Gruppenaccount</h3>
          <p class="muted small">Für ein Gerät im Flur: sieht nur die Anwesenheitsliste dieser Gruppe.</p>
          <template v-if="accountOf(g.id)">
            <div class="acc" :class="{ off: !accountOf(g.id)!.active }">
              <span class="pi pi-tablet" aria-hidden="true" />
              <b>{{ accountOf(g.id)!.username }}</b>
              <em v-if="!accountOf(g.id)!.active">gesperrt</em>
            </div>
            <div class="acc-btns">
              <Button label="Neues Passwort" size="small" outlined @click="resetPassword(accountOf(g.id)!)" />
              <Button
                :label="accountOf(g.id)!.active ? 'Sperren' : 'Entsperren'"
                size="small"
                severity="secondary"
                text
                @click="toggleAccount(accountOf(g.id)!)"
              />
              <Button label="Löschen" size="small" severity="danger" text @click="removeAccount(accountOf(g.id)!)" />
            </div>
          </template>
          <div v-else class="acc-new">
            <input
              :value="newUser[g.id] ?? suggestUser(g)"
              type="text"
              aria-label="Benutzername"
              :data-testid="`manage-account-user-${g.id}`"
              @input="newUser[g.id] = ($event.target as HTMLInputElement).value"
            />
            <Button label="Anlegen" size="small" :data-testid="`manage-account-create-${g.id}`" @click="createAccount(g)" />
          </div>
          <div v-if="shownPassword?.gid === g.id" class="pw" data-testid="manage-account-password">
            <div>
              Anmelden mit <b>{{ shownPassword.username }}</b> und Passwort
              <code>{{ shownPassword.password }}</code>
            </div>
            <small>Das Passwort wird nur jetzt angezeigt. Das Gerät bleibt danach angemeldet.</small>
            <div class="acc-btns">
              <Button label="Kopieren" icon="pi pi-copy" size="small" text @click="copyPassword" />
              <Button label="Fertig" size="small" text severity="secondary" @click="shownPassword = null" />
            </div>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.manage {
  max-width: 1100px;
  margin: 0 auto;
}
.back {
  border: none;
  background: transparent;
  color: #334155;
  font: inherit;
  cursor: pointer;
  padding: 0.4rem 0;
  margin-bottom: 0.5rem;
}
.opts {
  font-size: 0.85rem;
  color: #475569;
  margin-bottom: 0.5rem;
}
.group {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 14px;
  padding: 1rem;
  margin-bottom: 1rem;
}
.group h2 {
  margin: 0 0 0.75rem;
  font-size: 1.1rem;
}
.group h2 small {
  font-weight: 400;
  color: #64748b;
  margin-left: 0.3rem;
  font-size: 0.85rem;
}
.cols {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
  gap: 1.25rem;
}
.kids {
  list-style: none;
  margin: 0 0 0.6rem;
  padding: 0;
}
.kids li {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.35rem 0;
  border-bottom: 1px solid #f1f5f9;
}
.kids li.off .kid-name {
  color: #94a3b8;
}
.kid-name {
  flex: 1;
  font-weight: 600;
}
.kid-name em {
  font-weight: 400;
  font-style: normal;
  font-size: 0.8rem;
}
.link {
  border: none;
  background: transparent;
  color: #4f46e5;
  font: inherit;
  font-size: 0.8rem;
  cursor: pointer;
}
.icon {
  border: none;
  background: transparent;
  width: 2.1rem;
  height: 2.1rem;
  border-radius: 8px;
  cursor: pointer;
  color: #475569;
}
.icon:hover {
  background: #f1f5f9;
}
.edit {
  width: 100%;
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 0.4rem;
}
.edit-btns {
  grid-column: 1 / -1;
  display: flex;
  gap: 0.3rem;
}
.grow {
  flex: 1;
}
input[type='text'],
select,
textarea {
  font: inherit;
  font-size: 0.95rem;
  padding: 0.45rem 0.55rem;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  background: #fff;
  min-width: 0;
  width: 100%;
}
.add {
  display: flex;
  gap: 0.5rem;
  align-items: flex-start;
}
.add textarea {
  flex: 1;
  resize: vertical;
}
.account h3 {
  margin: 0 0 0.25rem;
  font-size: 0.95rem;
}
.acc {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin: 0.5rem 0;
}
.acc.off b {
  color: #94a3b8;
}
.acc em {
  font-style: normal;
  font-size: 0.75rem;
  background: #fee2e2;
  color: #991b1b;
  border-radius: 999px;
  padding: 0.05rem 0.45rem;
}
.acc-btns {
  display: flex;
  gap: 0.25rem;
  flex-wrap: wrap;
}
.acc-new {
  display: flex;
  gap: 0.4rem;
  margin-top: 0.5rem;
}
.pw {
  margin-top: 0.75rem;
  background: #fefce8;
  border: 1px solid #fde68a;
  border-radius: 10px;
  padding: 0.65rem 0.75rem;
  font-size: 0.9rem;
}
.pw code {
  font-size: 1.05rem;
  background: #fff;
  padding: 0.05rem 0.35rem;
  border-radius: 6px;
  border: 1px solid #e2e8f0;
}
.pw small {
  display: block;
  color: #92400e;
  margin-top: 0.3rem;
}
.muted {
  color: #64748b;
}
.small {
  font-size: 0.82rem;
  margin: 0;
}
@media (max-width: 768px) {
  .cols {
    grid-template-columns: 1fr;
  }
  .edit {
    grid-template-columns: 1fr 1fr;
  }
  .edit select {
    grid-column: 1 / -1;
  }
}
</style>
