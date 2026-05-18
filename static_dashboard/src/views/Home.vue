<script setup>
import { onMounted, ref } from 'vue'
import { db } from '../db.js'
import PartyShareChart from '../components/PartyShareChart.vue'

const data = ref(null)
const error = ref(null)

onMounted(async () => {
  try {
    data.value = await db.partyShare()
  } catch (e) {
    error.value = e.message
  }
})
</script>

<template>
  <p class="crumb">General elections, 2010 – 2024</p>
  <h2 class="title">Party share of the vote</h2>
  <p class="subtitle">National share of valid votes cast at each UK general election, by main party.</p>

  <div class="card">
    <h2>Trend</h2>
    <div v-if="error" class="error">Couldn't load data: {{ error }}</div>
    <div v-else-if="!data" class="loading">Loading DuckDB and querying data…</div>
    <PartyShareChart v-else :data="data" />
  </div>

  <div class="card">
    <h2>How this works</h2>
    <p>This dashboard is fully static — everything runs in your browser. The page boots
      <a href="https://duckdb.org/docs/api/wasm/overview" target="_blank" rel="noopener">DuckDB WASM</a>,
      registers the three data files served from <code>/data/</code>, and runs the same SQL the Go service did.
      No backend; nothing leaves your browser.</p>
    <p>Use the search bar above to jump straight to any UK Westminster constituency, local ward or council.</p>
  </div>
</template>
