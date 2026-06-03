import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  scrollBehavior(to, _from, savedPosition) {
    if (to.hash) {
      return { el: to.hash, behavior: 'smooth' }
    }
    if (savedPosition) return savedPosition
    return { top: 0 }
  },
  routes: [
    {
      path: '/login',
      component: () => import('@/layouts/AuthLayout.vue'),
      children: [
        {
          path: '',
          name: 'login',
          component: () => import('@/views/LoginView.vue'),
        },
      ],
    },
    {
      path: '/',
      component: () => import('@/layouts/AppLayout.vue'),
      children: [
        {
          path: '',
          redirect: '/dashboard',
        },
        {
          path: 'dashboard',
          name: 'dashboard',
          component: () => import('@/views/DashboardView.vue'),
          meta: { title: 'Dashboard' },
        },
        {
          path: 'my/times',
          name: 'my-times',
          component: () => import('@/views/my/MyTimesView.vue'),
          meta: { title: 'Meine Zeiten' },
        },
        {
          path: 'my/balance',
          name: 'my-balance',
          component: () => import('@/views/my/MyBalanceView.vue'),
          meta: { title: 'Mein Saldo' },
        },
        {
          path: 'my/vacation',
          name: 'my-vacation',
          component: () => import('@/views/my/MyVacationView.vue'),
          meta: { title: 'Urlaub' },
        },
        {
          path: 'my/schedule',
          name: 'my-schedule',
          component: () => import('@/views/my/MyScheduleView.vue'),
          meta: { title: 'Mein Dienstplan' },
        },
        {
          path: 'my/password',
          name: 'change-password',
          component: () => import('@/views/my/ChangePasswordView.vue'),
          meta: { title: 'Passwort ändern' },
        },
        {
          path: 'employees',
          name: 'employees',
          component: () => import('@/views/employees/EmployeeListView.vue'),
          meta: { title: 'Mitarbeiter', roles: ['leitung', 'superadmin'] },
        },
        {
          path: 'employees/:id',
          name: 'employee-detail',
          component: () => import('@/views/employees/EmployeeDetailView.vue'),
          meta: { title: 'Mitarbeiter', roles: ['leitung', 'superadmin'] },
        },
        {
          path: 'employees/:id/edit',
          name: 'employee-edit',
          component: () => import('@/views/employees/EmployeeEditView.vue'),
          meta: { title: 'Mitarbeiter bearbeiten', roles: ['leitung', 'superadmin'] },
        },
        {
          path: 'groups',
          name: 'groups',
          component: () => import('@/views/groups/GroupsView.vue'),
          meta: { title: 'Gruppen', roles: ['leitung', 'superadmin'] },
        },
        {
          path: 'schedule',
          name: 'schedule',
          component: () => import('@/views/schedule/ScheduleEditorView.vue'),
          meta: { title: 'Dienstplan', roles: ['leitung', 'superadmin'] },
        },
        {
          path: 'absences',
          name: 'absences',
          component: () => import('@/views/absences/AbsencesView.vue'),
          meta: { title: 'Abwesenheiten', roles: ['leitung', 'superadmin'] },
        },
        {
          path: 'corrections',
          name: 'corrections',
          component: () => import('@/views/corrections/CorrectionsView.vue'),
          meta: { title: 'Korrekturen', roles: ['leitung', 'superadmin'] },
        },
        {
          path: 'closure-days',
          name: 'closure-days',
          component: () => import('@/views/closuredays/ClosureDaysView.vue'),
          meta: { title: 'Schließtage & Feiertage', roles: ['leitung', 'superadmin'] },
        },
        {
          path: 'reports',
          name: 'reports',
          component: () => import('@/views/reports/ReportsView.vue'),
          meta: { title: 'Berichte', roles: ['leitung', 'superadmin'] },
        },
        {
          path: 'import-status',
          name: 'import-status',
          component: () => import('@/views/leitung/ImportStatusView.vue'),
          meta: { title: 'Importstatus', roles: ['leitung', 'superadmin'] },
        },
        {
          path: 'schedule-gaps',
          name: 'schedule-gaps',
          component: () => import('@/views/leitung/ScheduleGapsView.vue'),
          meta: { title: 'Offene Dienstplan-Tage', roles: ['leitung', 'superadmin'] },
        },
        {
          path: 'admin/users',
          name: 'admin-users',
          component: () => import('@/views/admin/UsersView.vue'),
          meta: { title: 'Benutzer (Admin)', roles: ['superadmin'] },
        },
        {
          path: 'admin/holidays',
          name: 'admin-holidays',
          component: () => import('@/views/admin/HolidaysView.vue'),
          meta: { title: 'Feiertage (Admin)', roles: ['superadmin'] },
        },
        {
          path: 'admin/settings',
          name: 'admin-settings',
          component: () => import('@/views/admin/SettingsView.vue'),
          meta: { title: 'Einstellungen (Admin)', roles: ['superadmin'] },
        },
      ],
    },
  ],
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  if (to.name === 'login') {
    if (auth.isAuthenticated) return { name: 'dashboard' }
    return true
  }
  if (!auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  const allowed = to.meta.roles as string[] | undefined
  if (allowed?.length && auth.role && !allowed.includes(auth.role)) {
    return { name: 'dashboard' }
  }
  return true
})

export default router
