<script setup>
import { ref, watch, computed } from 'vue'
import { db } from '../db.js'
import { Doughnut } from 'vue-chartjs'
import {
  Chart as ChartJS, ArcElement, Tooltip, Legend,
} from 'chart.js'

ChartJS.register(ArcElement, Tooltip, Legend)

const props = defineProps({ code: String })
const data = ref(null)
const error = ref(null)

async function load() {
  data.value = null; error.value = null
  try { data.value = await db.ward(props.code) }
  catch (e) { error.value = e.message }
}
watch(() => props.code, load, { immediate: true })

const partyColours = {
  Labour: '#d4222b', Conservative: '#1f6cd1', 'Liberal Democrats': '#f1a51c',
  'Reform UK': '#12b6cf', Green: '#5cb462', Independent: '#777', Other: '#bbb',
}

const chartData = computed(() => {
  if (!data.value?.local_2026) return null
  const entries = data.value.local_2026.votes
  return {
    labels: entries.map(e => e.party),
    datasets: [{
      data: entries.map(e => e.votes),
      backgroundColor: entries.map(e => partyColours[e.party] ?? '#888'),
    }],
  }
})

const chartOpts = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { position: 'right' },
    tooltip: {
      callbacks: {
        label: (ctx) => {
          const e = data.value.local_2026.votes[ctx.dataIndex]
          return `${e.party}: ${e.votes.toLocaleString()} (${e.share.toFixed(1)}%)`
        },
      },
    },
  },
}
</script>

<template>
  <div v-if="error" class="error">Couldn't load ward: {{ error }}</div>
  <div v-else-if="!data" class="loading">Loading…</div>
  <template v-else>
    <p class="crumb">Ward in
      <router-link :to="{ name: 'council', params: { code: data.lad_code } }">{{ data.lad_name }}</router-link>
    </p>
    <h2 class="title">{{ data.name }}</h2>
    <p class="subtitle">Local government ward · <span class="pill">{{ data.code }}</span></p>

    <div class="card">
      <h2>Westminster constituency</h2>
      <table>
        <thead><tr><th>Constituency</th><th>Notes</th></tr></thead>
        <tbody>
          <tr v-for="p in data.constituencies" :key="p.code">
            <td><router-link :to="{ name: 'constituency', params: { code: p.code } }">{{ p.name }}</router-link></td>
            <td>
              <span v-if="!p.split">whole ward</span>
              <span v-else-if="p.primary" class="pill">split · majority area</span>
              <span v-else class="pill">split · minor area</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="data.local_2026" class="card">
      <h2>2026 local election</h2>
      <p class="subtitle" style="margin-bottom: 0.5rem">
        {{ data.local_2026.seats ? data.local_2026.seats + ' seat' + (data.local_2026.seats === 1 ? '' : 's') : 'Seats: —' }}
        · Winners: <span v-for="(w, i) in data.local_2026.seat_winners" :key="i">{{ w }}{{ i < data.local_2026.seat_winners.length - 1 ? ', ' : '' }}</span>
      </p>
      <div class="grid-2">
        <div class="chart-wrap" style="height: 300px">
          <Doughnut v-if="chartData" :data="chartData" :options="chartOpts" />
        </div>
        <table>
          <thead><tr><th>Party</th><th class="num">Votes</th><th class="num">Share</th></tr></thead>
          <tbody>
            <tr v-for="e in data.local_2026.votes" :key="e.party">
              <td>
                <span class="winner-bar" :style="{ width: '10px', background: partyColours[e.party] ?? '#888' }"></span>
                {{ e.party }}
              </td>
              <td class="num">{{ e.votes.toLocaleString() }}</td>
              <td class="num">{{ e.share.toFixed(1) }}%</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <div v-else class="card">
      <h2>2026 local election</h2>
      <p class="loading">No 2026 ward result available for this ward.</p>
    </div>
  </template>
</template>
