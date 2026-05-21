<script setup lang="ts">
import { ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Password from 'primevue/password'
import { useToast } from 'primevue/usetoast'

import { useAuthStore } from '@/stores/auth'
import { getApiErrorMessage } from '@/utils/apiError'

const auth = useAuthStore()
const toast = useToast()

const currentPw = ref('')
const newPw = ref('')
const newPw2 = ref('')
const saving = ref(false)

async function submit() {
  if (newPw.value.length < 8) {
    toast.add({
      severity: 'warn',
      summary: 'Passwort zu kurz',
      detail: 'Das neue Passwort muss mindestens 8 Zeichen haben.',
      life: 10000,
    })
    return
  }
  if (newPw.value !== newPw2.value) {
    toast.add({
      severity: 'warn',
      summary: 'Abweichung',
      detail: 'Die neuen Passwörter stimmen nicht überein.',
      life: 10000,
    })
    return
  }
  saving.value = true
  try {
    await auth.changePassword(currentPw.value, newPw.value)
    currentPw.value = ''
    newPw.value = ''
    newPw2.value = ''
    toast.add({
      severity: 'success',
      summary: 'Passwort geändert',
      detail: 'Dein neues Passwort ist aktiv.',
      life: 10000,
    })
  } catch (e) {
    const detail = getApiErrorMessage(e)
    toast.add({
      severity: 'error',
      summary: 'Änderung fehlgeschlagen',
      ...(detail ? { detail, life: 10000 } : { detail: 'Bitte aktuelles Passwort prüfen.', life: 10000 }),
    })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="page">
    <Card>
      <template #title>Passwort ändern</template>
      <template #content>
        <p class="hint">
          Gib dein aktuelles Passwort und ein neues Passwort ein (mindestens 8 Zeichen).
        </p>
        <div class="form">
          <div class="field">
            <label for="cp-current">Aktuelles Passwort</label>
            <Password
              id="cp-current"
              v-model="currentPw"
              class="w-full"
              input-class="w-full"
              :feedback="false"
              toggle-mask
              autocomplete="current-password"
            />
          </div>
          <div class="field">
            <label for="cp-new">Neues Passwort</label>
            <Password
              id="cp-new"
              v-model="newPw"
              class="w-full"
              input-class="w-full"
              :feedback="false"
              toggle-mask
              autocomplete="new-password"
            />
          </div>
          <div class="field">
            <label for="cp-new2">Neues Passwort (Wiederholung)</label>
            <Password
              id="cp-new2"
              v-model="newPw2"
              class="w-full"
              input-class="w-full"
              :feedback="false"
              toggle-mask
              autocomplete="new-password"
            />
          </div>
          <Button
            label="Passwort speichern"
            icon="pi pi-check"
            :loading="saving"
            :disabled="!currentPw || !newPw || !newPw2"
            @click="submit"
          />
        </div>
      </template>
    </Card>
  </div>
</template>

<style scoped>
.page {
  max-width: 480px;
}
.hint {
  margin: 0 0 1rem;
  font-size: 0.9rem;
  color: #64748b;
  line-height: 1.45;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.field label {
  font-size: 0.85rem;
  color: #475569;
}
.w-full {
  width: 100%;
}
</style>
