<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api.js'
import PartyShareChart from '../components/PartyShareChart.vue'

const data = ref(null)
const error = ref(null)

onMounted(async () => {
  try {
    data.value = await api.partyShare()
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
    <div v-else-if="!data" class="loading">Loading…</div>
    <PartyShareChart v-else :data="data" />
  </div>

  <div class="card">
    <h2>How to use this dashboard</h2>
    <p>Use the search bar above to jump straight to any UK Westminster constituency, local ward, or council. Constituency pages include the full
      result for each general election since 2010 and the list of wards inside the constituency. Council pages summarise the 2026 local election by ward.</p>
  </div>
</template>
