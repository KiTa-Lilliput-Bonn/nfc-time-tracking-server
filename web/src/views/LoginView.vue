<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import Card from 'primevue/card'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'

import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const route = useRoute()
const toast = useToast()
const auth = useAuthStore()

const username = ref('')
const password = ref('')
const loading = ref(false)

const showPwChange = ref(false)
const currentPw = ref('')
const newPw = ref('')
const newPw2 = ref('')
const pwLoading = ref(false)

async function onSubmit() {
  loading.value = true
  try {
    const data = await auth.login(username.value, password.value)
    if (data.user.must_change_password) {
      showPwChange.value = true
      currentPw.value = password.value
      return
    }
    await redirectAfterLogin()
  } catch {
    toast.add({ severity: 'error', summary: 'Anmeldung fehlgeschlagen', detail: 'Benutzername oder Passwort ungültig.', life: 10000 })
  } finally {
    loading.value = false
  }
}

async function redirectAfterLogin() {
  const redir = route.query.redirect as string | undefined
  await router.replace(redir && redir.startsWith('/') ? redir : '/dashboard')
}

async function submitPasswordChange() {
  if (newPw.value.length < 8) {
    toast.add({ severity: 'warn', summary: 'Passwort zu kurz', detail: 'Mindestens 8 Zeichen.', life: 10000 })
    return
  }
  if (newPw.value !== newPw2.value) {
    toast.add({ severity: 'warn', summary: 'Abweichung', detail: 'Die neuen Passwörter stimmen nicht überein.', life: 10000 })
    return
  }
  pwLoading.value = true
  try {
    await auth.changePassword(currentPw.value, newPw.value)
    showPwChange.value = false
    toast.add({ severity: 'success', summary: 'Passwort geändert', life: 10000 })
    await redirectAfterLogin()
  } catch {
    toast.add({ severity: 'error', summary: 'Fehler', detail: 'Passwort konnte nicht geändert werden.', life: 10000 })
  } finally {
    pwLoading.value = false
  }
}
</script>

<template>
  <Card class="login-card">
    <template #title>Anmeldung</template>
    <template #subtitle>NFC Zeiterfassung</template>
    <template #content>
      <form class="form" @submit.prevent="onSubmit">
        <div class="field">
          <label for="user">Benutzername</label>
          <InputText id="user" v-model="username" class="w-full" autocomplete="username" data-testid="login-user" />
        </div>
        <div class="field">
          <label for="pw">Passwort</label>
          <Password
            id="pw"
            v-model="password"
            class="w-full"
            input-class="w-full"
            :feedback="false"
            toggle-mask
            autocomplete="current-password"
            data-testid="login-password"
          />
        </div>
        <Button type="submit" label="Anmelden" class="w-full" :loading="loading" data-testid="login-submit" />
      </form>
    </template>
  </Card>

  <Dialog
    v-model:visible="showPwChange"
    modal
    header="Passwort ändern"
    :closable="false"
    :style="{ width: 'min(420px, 95vw)' }"
  >
    <p class="mb-3">Sie müssen Ihr Passwort beim ersten Login ändern.</p>
    <div class="field">
      <label>Neues Passwort</label>
      <Password v-model="newPw" class="w-full" input-class="w-full" :feedback="false" toggle-mask />
    </div>
    <div class="field">
      <label>Neues Passwort (Wiederholung)</label>
      <Password v-model="newPw2" class="w-full" input-class="w-full" :feedback="false" toggle-mask />
    </div>
    <template #footer>
      <Button label="Speichern" icon="pi pi-check" :loading="pwLoading" @click="submitPasswordChange" />
    </template>
  </Dialog>
</template>

<style scoped>
.login-card {
  width: 100%;
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
.mb-3 {
  margin-bottom: 0.75rem;
}
</style>
