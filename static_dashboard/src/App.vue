<script setup>
import SearchBar from './components/SearchBar.vue'
</script>

<template>
  <header class="app-header">
    <h1><router-link to="/">UK Politics Dashboard</router-link></h1>
    <SearchBar />
  </header>
  <main class="layout">
    <router-view />
  </main>
</template>
