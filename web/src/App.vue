<script setup lang="ts">
import Toast from 'primevue/toast'
</script>

<template>
  <Toast position="top-center" />
  <router-view />
</template>

<style>
/* Verhindert seitliches „Springen“ der Seite, wenn Modals den Body-Scroll sperren (Scrollbar verschwindet). */
html {
  scrollbar-gutter: stable;
}
html,
body,
#app {
  margin: 0;
  min-height: 100vh;
}
</style>
