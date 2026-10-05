<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import Card from 'primevue/card'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'

import { useAuthStore } from '@/stores/auth'
import { SSO_START_URL, fetchSsoConfig, ssoErrorMessage, type SsoConfig } from '@/api/sso'

const router = useRouter()
const route = useRoute()
const toast = useToast()
const auth = useAuthStore()

const sso = ref<SsoConfig | null>(null)
/** Passwortformular: bei SSO ggf. nur als ausklappbarer Fallback für die Leitung. */
const showPasswordForm = ref(true)
const passwordLoginOffered = computed(() => !sso.value?.enabled || sso.value.password_login !== 'none')
const passwordFormCollapsible = computed(() => !!sso.value?.enabled && sso.value.password_login === 'admins')
const ssoError = computed(() => {
  const code = route.query.sso_error
  return typeof code === 'string' && code ? ssoErrorMessage(code) : ''
})

onMounted(async () => {
  sso.value = await fetchSsoConfig()
  showPasswordForm.value = !passwordFormCollapsible.value
})

function startSso() {
  const redir = route.query.redirect
  try {
    if (typeof redir === 'string' && redir.startsWith('/')) sessionStorage.setItem('nfc_sso_redirect', redir)
    else sessionStorage.removeItem('nfc_sso_redirect')
  } catch {
    /* ignore */
  }
  window.location.href = SSO_START_URL
}

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
  } catch (e: unknown) {
    const status = (e as { response?: { status?: number } })?.response?.status
    const detail =
      status === 403
        ? 'Die Anmeldung mit Passwort ist für dieses Konto abgeschaltet. Bitte über SSO anmelden.'
        : 'Benutzername oder Passwort ungültig.'
    toast.add({ severity: 'error', summary: 'Anmeldung fehlgeschlagen', detail, life: 10000 })
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
      <p v-if="ssoError" class="sso-error" role="alert" data-testid="sso-error">{{ ssoError }}</p>
      <template v-if="sso?.enabled">
        <Button
          type="button"
          :label="sso.button_label || 'Mit SSO anmelden'"
          icon="pi pi-sign-in"
          class="w-full"
          data-testid="login-sso"
          @click="startSso"
        />
        <button
          v-if="passwordLoginOffered && passwordFormCollapsible"
          type="button"
          class="pw-toggle"
          data-testid="login-password-toggle"
          @click="showPasswordForm = !showPasswordForm"
        >
          {{ showPasswordForm ? 'Passwort-Anmeldung ausblenden' : 'Mit Passwort anmelden (Leitung)' }}
        </button>
        <div v-else-if="passwordLoginOffered" class="divider"><span>oder mit Passwort</span></div>
      </template>
      <form v-if="passwordLoginOffered && showPasswordForm" class="form" @submit.prevent="onSubmit">
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
.sso-error {
  margin: 0 0 1rem;
  padding: 0.6rem 0.75rem;
  border-radius: 6px;
  background: #fef2f2;
  color: #991b1b;
  font-size: 0.9rem;
}
.divider {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin: 1.1rem 0;
  color: #64748b;
  font-size: 0.85rem;
}
.divider::before,
.divider::after {
  content: '';
  flex: 1;
  border-top: 1px solid #e2e8f0;
}
.pw-toggle {
  display: block;
  margin: 0.9rem auto 0.9rem;
  background: none;
  border: none;
  color: #475569;
  font-size: 0.85rem;
  text-decoration: underline;
  cursor: pointer;
}
.mb-3 {
  margin-bottom: 0.75rem;
}
</style>
