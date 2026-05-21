<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const title = computed(() => (route.meta.title as string) || 'Seite')
</script>

<template>
  <div class="placeholder">
    <h2 class="h">{{ title }}</h2>
    <p class="muted">Ansicht in Task 16+ der Implementierung.</p>
  </div>
</template>

<style scoped>
.placeholder {
  background: #fff;
  border-radius: 8px;
  padding: 2rem;
  border: 1px solid #e2e8f0;
}
.h {
  margin: 0 0 0.5rem;
  font-size: 1.25rem;
  color: #0f172a;
}
.muted {
  margin: 0;
  color: #64748b;
}
</style>
