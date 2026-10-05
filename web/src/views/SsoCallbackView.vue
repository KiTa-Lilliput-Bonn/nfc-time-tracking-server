<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import ProgressSpinner from 'primevue/progressspinner'

import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const auth = useAuthStore()

onMounted(async () => {
  const params = new URLSearchParams(window.location.hash.replace(/^#/, ''))
  const token = params.get('token')
  // Token nicht in der Adresszeile/History stehen lassen.
  history.replaceState(history.state, '', window.location.pathname)
  if (!token) {
    await router.replace({ name: 'login', query: { sso_error: 'failed' } })
    return
  }
  try {
    await auth.loginWithToken(token)
    let redir = '/dashboard'
    try {
      const stored = sessionStorage.getItem('nfc_sso_redirect')
      sessionStorage.removeItem('nfc_sso_redirect')
      if (stored && stored.startsWith('/') && !stored.startsWith('//')) redir = stored
    } catch {
      /* ignore */
    }
    await router.replace(redir)
  } catch {
    await router.replace({ name: 'login', query: { sso_error: 'failed' } })
  }
})
</script>

<template>
  <div class="sso-wait" data-testid="sso-callback">
    <ProgressSpinner style="width: 2.5rem; height: 2.5rem" />
    <p>Anmeldung wird abgeschlossen …</p>
  </div>
</template>

<style scoped>
.sso-wait {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1rem;
  color: #e2e8f0;
}
</style>
