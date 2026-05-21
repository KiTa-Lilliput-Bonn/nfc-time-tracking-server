<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Menu from 'primevue/menu'
import type { MenuItem } from 'primevue/menuitem'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const pageTitle = computed(() => (route.meta.title as string) || 'NFC Zeiterfassung')

/** Auf schmalen Viewports: Navigation ausgeblendet, per Button als Overlay. */
const navOpen = ref(false)

/** Referenz auf die Topbar, um deren Höhe als CSS-Variable bereitzustellen. */
const topbarRef = ref<HTMLElement | null>(null)
let topbarResizeObserver: ResizeObserver | null = null

function updateLayoutTopInset() {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  const el = topbarRef.value
  if (!el) {
    root.style.setProperty('--layout-top-inset', '0px')
    return
  }
  const cs = getComputedStyle(el)
  if (cs.position !== 'sticky' && cs.position !== 'fixed') {
    root.style.setProperty('--layout-top-inset', '0px')
    return
  }
  const h = el.getBoundingClientRect().height
  root.style.setProperty('--layout-top-inset', `${Math.round(h)}px`)
}

function toggleNav() {
  navOpen.value = !navOpen.value
}

function closeNav() {
  navOpen.value = false
}

watch(navOpen, (open) => {
  if (typeof document === 'undefined') return
  if (window.innerWidth > 768) return
  document.body.style.overflow = open ? 'hidden' : ''
})

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    userMenuRef.value?.hide()
    closeNav()
  }
}

function syncNavForViewport() {
  if (window.innerWidth > 768) {
    navOpen.value = false
    document.body.style.overflow = ''
  }
  updateLayoutTopInset()
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('resize', syncNavForViewport)
  updateLayoutTopInset()
  if (typeof ResizeObserver !== 'undefined' && topbarRef.value) {
    topbarResizeObserver = new ResizeObserver(() => updateLayoutTopInset())
    topbarResizeObserver.observe(topbarRef.value)
  }
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('resize', syncNavForViewport)
  document.body.style.overflow = ''
  topbarResizeObserver?.disconnect()
  topbarResizeObserver = null
  if (typeof document !== 'undefined') {
    document.documentElement.style.setProperty('--layout-top-inset', '0px')
  }
})

/** Aktive Navigation inkl. Unterpfaden (z. B. /employees/:id). */
function isActiveNav(targetPath: string): boolean {
  const path = route.path
  if (path === targetPath) return true
  if (targetPath === '/employees' && path.startsWith('/employees/')) return true
  if (targetPath === '/groups' && path.startsWith('/groups')) return true
  return false
}

function logout() {
  auth.logout()
  router.push('/login')
}

const isLeitung = computed(() => auth.role === 'leitung' || auth.role === 'superadmin')
const isSuper = computed(() => auth.role === 'superadmin')

const userMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const userMenuVisible = ref(false)

const userMenuModel = computed((): MenuItem[] => [
  {
    label: 'Abmelden',
    icon: 'pi pi-sign-out',
    command: () => logout(),
  },
])

function toggleUserMenu(event: Event) {
  userMenuRef.value?.toggle(event)
}

watch(
  () => route.fullPath,
  () => {
    closeNav()
    userMenuRef.value?.hide()
  },
)
</script>

<template>
  <div class="layout" :class="{ 'layout--nav-open': navOpen }">
    <button type="button" class="nav-backdrop" tabindex="-1" aria-hidden="true" @click="closeNav" />
    <aside class="sidebar" aria-label="Hauptnavigation">
      <div class="sidebar-head">
        <div class="brand">NFC Zeiterfassung</div>
        <button type="button" class="btn-sidebar-close" aria-label="Menü schließen" @click="closeNav">
          ×
        </button>
      </div>
      <nav id="app-sidebar-nav" class="nav">
        <RouterLink
          class="nav-item"
          active-class=""
          :class="{ 'nav-item--active': isActiveNav('/dashboard') }"
          to="/dashboard"
        >
          Dashboard
        </RouterLink>
        <span class="nav-group">Persönlich</span>
        <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/my/times') }" to="/my/times">
          Meine Zeiten
        </RouterLink>
        <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/my/balance') }" to="/my/balance">
          Mein Saldo
        </RouterLink>
        <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/my/vacation') }" to="/my/vacation">
          Urlaub
        </RouterLink>
        <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/my/schedule') }" to="/my/schedule">
          Dienstplan
        </RouterLink>
        <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/my/password') }" to="/my/password">
          Passwort ändern
        </RouterLink>

        <template v-if="isLeitung">
          <span class="nav-group">Leitung</span>
          <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/employees') }" to="/employees">
            Mitarbeiter
          </RouterLink>
          <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/groups') }" to="/groups">
            Gruppen
          </RouterLink>
          <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/schedule') }" to="/schedule">
            Dienstplan-Editor
          </RouterLink>
          <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/absences') }" to="/absences">
            Abwesenheiten
          </RouterLink>
          <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/corrections') }" to="/corrections">
            Korrekturen
          </RouterLink>
          <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/closure-days') }" to="/closure-days">
            Schließtage
          </RouterLink>
          <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/reports') }" to="/reports">
            Berichte
          </RouterLink>
          <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/import-status') }" to="/import-status">
            Importstatus
          </RouterLink>
        </template>

        <template v-if="isSuper">
          <span class="nav-group">Admin</span>
          <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/admin/users') }" to="/admin/users">
            Benutzer
          </RouterLink>
          <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/admin/holidays') }" to="/admin/holidays">
            Feiertage
          </RouterLink>
          <RouterLink class="nav-item" active-class="" :class="{ 'nav-item--active': isActiveNav('/admin/settings') }" to="/admin/settings">
            Einstellungen
          </RouterLink>
        </template>
      </nav>
    </aside>
    <div class="main">
      <header ref="topbarRef" class="topbar">
        <div class="topbar-left">
          <button
            type="button"
            class="btn-menu"
            :aria-label="navOpen ? 'Menü schließen' : 'Menü öffnen'"
            :aria-expanded="navOpen"
            aria-controls="app-sidebar-nav"
            @click="toggleNav"
          >
            <span class="btn-menu-bars" aria-hidden="true" />
          </button>
          <div class="title-wrap">
            <h1 class="title">{{ pageTitle }}</h1>
          </div>
        </div>
        <div class="topbar-right">
          <div class="user user--desktop">
            <span class="name">{{ auth.user?.display_name || '—' }}</span>
            <span class="role-pill">{{ auth.role }}</span>
            <button type="button" class="btn-logout" @click="logout">Abmelden</button>
          </div>
          <div class="user-compact">
            <Menu
              ref="userMenuRef"
              :model="userMenuModel"
              popup
              append-to="body"
              :base-z-index="500"
              aria-label="Benutzerkonto-Menü"
              :pt="{ root: { id: 'app-user-menu-panel' } }"
              @show="userMenuVisible = true"
              @hide="userMenuVisible = false"
            >
              <template #start>
                <div class="user-menu-head">
                  <div class="user-menu-name">{{ auth.user?.display_name || '—' }}</div>
                  <span class="role-pill">{{ auth.role }}</span>
                </div>
              </template>
            </Menu>
            <button
              type="button"
              class="btn-user-menu"
              aria-label="Benutzerkonto"
              aria-haspopup="menu"
              :aria-expanded="userMenuVisible"
              aria-controls="app-user-menu-panel"
              @click="toggleUserMenu"
            >
              <span class="pi pi-user" aria-hidden="true" />
            </button>
          </div>
        </div>
      </header>
      <main class="content">
        <router-view />
      </main>
    </div>
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  min-height: 100vh;
  background: #f8fafc;
}
.sidebar {
  width: 240px;
  background: #0f172a;
  color: #e2e8f0;
  padding: 1.25rem 0;
  flex-shrink: 0;
}
.sidebar-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0 0.75rem 0.75rem;
  margin-bottom: 0.75rem;
  border-bottom: 1px solid #334155;
}
.brand {
  font-weight: 700;
  font-size: 1rem;
  padding: 0 0.25rem;
  min-width: 0;
}
.btn-sidebar-close {
  display: none;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: 2.25rem;
  height: 2.25rem;
  border: none;
  border-radius: 6px;
  background: #1e293b;
  color: #e2e8f0;
  font-size: 1.35rem;
  line-height: 1;
  cursor: pointer;
}
.btn-sidebar-close:hover {
  background: #334155;
}
.nav {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  padding: 0 0.5rem;
}
.nav-group {
  font-size: 0.7rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #94a3b8;
  margin: 0.75rem 0.5rem 0.25rem;
}
.nav-item {
  display: block;
  text-align: left;
  text-decoration: none;
  border: none;
  background: transparent;
  color: #e2e8f0;
  padding: 0.5rem 0.75rem;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.9rem;
}
.nav-item:hover {
  background: #1e293b;
}
.nav-item--active {
  background: #334155;
  color: #f8fafc;
  font-weight: 600;
}
.nav-item.nav-item--active:hover {
  background: #475569;
}
.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 1rem 1.5rem;
  background: #fff;
  border-bottom: 1px solid #e2e8f0;
  /* Sticky in jeder Größe. Seiten mit eigenen sticky-Köpfen (z. B. Dienstplan) verwenden
     die globale CSS-Variable --layout-top-inset, damit ihre Köpfe unter der Topbar kleben. */
  position: sticky;
  top: 0;
  z-index: 100;
}
.topbar-left {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  min-width: 0;
  flex: 1;
}
.btn-menu {
  display: none;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: 2.5rem;
  height: 2.5rem;
  padding: 0;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  background: #fff;
  cursor: pointer;
}
.btn-menu:hover {
  background: #f1f5f9;
}
.btn-menu-bars {
  display: block;
  width: 1.1rem;
  height: 1px;
  background: #0f172a;
  box-shadow:
    0 -5px 0 #0f172a,
    0 5px 0 #0f172a;
}
.nav-backdrop {
  display: none;
}
.title-wrap {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.2rem;
  min-width: 0;
}
.title {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 600;
  color: #0f172a;
}
.topbar-right {
  display: flex;
  align-items: center;
  gap: 1rem;
  flex-shrink: 1;
  min-width: 0;
}
.user {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
.user--desktop {
  min-width: 0;
}
.user-compact {
  display: none;
  align-items: center;
  flex-shrink: 0;
}
.btn-user-menu {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2.5rem;
  height: 2.5rem;
  padding: 0;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  background: #fff;
  cursor: pointer;
  color: #0f172a;
  font-size: 1.15rem;
}
.btn-user-menu:hover {
  background: #f1f5f9;
}
.user-menu-head {
  padding: 0.65rem 0.85rem 0.5rem;
  border-bottom: 1px solid #e2e8f0;
  max-width: min(280px, 85vw);
}
.user-menu-name {
  font-weight: 600;
  color: #0f172a;
  font-size: 0.95rem;
  margin-bottom: 0.35rem;
  word-break: break-word;
}
.name {
  color: #475569;
  font-size: 0.9rem;
}
.role-pill {
  font-size: 0.75rem;
  background: #e0e7ff;
  color: #3730a3;
  padding: 0.2rem 0.5rem;
  border-radius: 999px;
  text-transform: uppercase;
}
.btn-logout {
  border: 1px solid #cbd5e1;
  background: #fff;
  padding: 0.35rem 0.75rem;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.85rem;
}
.btn-logout:hover {
  background: #f1f5f9;
}
.content {
  flex: 1;
  padding: 1.5rem;
}
@media (max-width: 600px) {
  .user--desktop {
    display: none;
  }
  .user-compact {
    display: flex;
  }
}
@media (min-width: 601px) and (max-width: 768px) {
  .user--desktop .name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}
@media (max-width: 768px) {
  .topbar {
    /* Über Mobile-Sidebar (z-index: 300) und Backdrop (z-index: 250) liegen. */
    z-index: 400;
  }
  .btn-menu {
    display: inline-flex;
  }
  .btn-sidebar-close {
    display: inline-flex;
  }
  .nav-backdrop {
    display: block;
    position: fixed;
    inset: 0;
    z-index: 250;
    margin: 0;
    padding: 0;
    border: none;
    background: rgba(15, 23, 42, 0.45);
    cursor: pointer;
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.2s ease;
  }
  .layout--nav-open .nav-backdrop {
    opacity: 1;
    pointer-events: auto;
  }
  .sidebar {
    position: fixed;
    left: 0;
    top: 0;
    height: 100vh;
    height: 100dvh;
    width: min(280px, 88vw);
    z-index: 300;
    transform: translateX(-100%);
    transition: transform 0.2s ease;
    overflow-y: auto;
    box-shadow: 4px 0 24px rgba(0, 0, 0, 0.18);
    padding-top: 1rem;
  }
  .layout--nav-open .sidebar {
    transform: translateX(0);
  }
  .main {
    width: 100%;
  }
}
@media (min-width: 769px) {
  .nav-backdrop {
    display: none !important;
  }
  .sidebar {
    position: sticky;
    top: 0;
    align-self: flex-start;
    height: 100vh;
    height: 100dvh;
    overflow-y: auto;
  }
}
</style>
