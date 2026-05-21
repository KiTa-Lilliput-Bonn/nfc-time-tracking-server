<template>
  <div class="auth-shell">
    <div class="auth-card">
      <router-view />
    </div>
  </div>
</template>

<style scoped>
.auth-shell {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(160deg, #0f172a 0%, #1e293b 50%, #334155 100%);
  padding: 1rem;
}
.auth-card {
  width: 100%;
  max-width: 420px;
}
</style>
