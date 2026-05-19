<script setup>
import { onMounted, ref } from 'vue'
import { db } from '../db.js'
import PartyShareChart from '../components/PartyShareChart.vue'

const data = ref(null)
const seats = ref(null)
const error = ref(null)

onMounted(async () => {
  try {
    const [shareRes, seatsRes] = await Promise.all([db.partyShare(), db.seatsWon()])
    data.value = shareRes
    seats.value = seatsRes
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
    <h2>Vote share</h2>
    <div v-if="error" class="error">Couldn't load data: {{ error }}</div>
    <div v-else-if="!data" class="loading">Loading DuckDB and querying data…</div>
    <PartyShareChart v-else :data="data" />
  </div>

  <div class="card">
    <h2>Seats won</h2>
    <div v-if="error" class="error">Couldn't load data: {{ error }}</div>
    <div v-else-if="!seats" class="loading">Loading…</div>
    <PartyShareChart
      v-else
      :data="seats"
      value-key="seats"
      y-label="Seats won"
      unit="seats"
    />
  </div>
</template>
