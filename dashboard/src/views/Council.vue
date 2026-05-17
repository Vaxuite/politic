<script setup>
import { ref, watch, computed } from 'vue'
import { api } from '../api.js'
import { Bar } from 'vue-chartjs'
import {
  Chart as ChartJS, CategoryScale, LinearScale, BarElement, Tooltip, Legend,
} from 'chart.js'

ChartJS.register(CategoryScale, LinearScale, BarElement, Tooltip, Legend)

const props = defineProps({ code: String })
const data = ref(null)
const error = ref(null)

async function load() {
  data.value = null; error.value = null
  try { data.value = await api.council(props.code) }
  catch (e) { error.value = e.message }
}
watch(() => props.code, load, { immediate: true })

const partyColours = {
  Labour: '#d4222b', Conservative: '#1f6cd1', 'Liberal Democrats': '#f1a51c',
  'Reform UK': '#12b6cf', Green: '#5cb462', Independent: '#777', Other: '#bbb',
}

const summaryChart = computed(() => {
  if (!data.value?.summary_2026?.length) return null
  return {
    labels: data.value.summary_2026.map(e => e.party),
    datasets: [{
      label: 'Share %',
      data: data.value.summary_2026.map(e => e.share),
      backgroundColor: data.value.summary_2026.map(e => partyColours[e.party] ?? '#888'),
    }],
  }
})
const chartOpts = {
  responsive: true, maintainAspectRatio: false,
  plugins: { legend: { display: false }, tooltip: { callbacks: { label: (ctx) => `${ctx.parsed.y.toFixed(1)}%` } } },
  scales: { y: { min: 0, title: { display: true, text: 'Share (%)' } } },
}
</script>

<template>
  <div v-if="error" class="error">Couldn't load council: {{ error }}</div>
  <div v-else-if="!data" class="loading">Loading…</div>
  <template v-else>
    <p class="crumb">Local authority</p>
    <h2 class="title">{{ data.name }}</h2>
    <p class="subtitle">{{ data.ward_count }} wards · <span class="pill">{{ data.code }}</span></p>

    <div v-if="data.summary_2026?.length" class="card">
      <h2>2026 local election — council-wide vote share</h2>
      <div class="grid-2">
        <div class="chart-wrap" style="height: 300px">
          <Bar v-if="summaryChart" :data="summaryChart" :options="chartOpts" />
        </div>
        <table>
          <thead><tr><th>Party</th><th class="num">Votes</th><th class="num">Share</th></tr></thead>
          <tbody>
            <tr v-for="e in data.summary_2026" :key="e.party">
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
      <p class="loading">No 2026 results available for this council.</p>
    </div>

    <div class="card">
      <h2>Wards</h2>
      <table>
        <thead>
          <tr>
            <th>Ward</th>
            <th>Constituency</th>
            <th>Winners 2026</th>
            <th>Top party 2026</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="w in data.wards" :key="w.ward_code + w.ward_name">
            <td>
              <router-link v-if="w.ward_code" :to="{ name: 'ward', params: { code: w.ward_code } }">{{ w.ward_name }}</router-link>
              <span v-else>{{ w.ward_name }}</span>
            </td>
            <td>{{ (w.constituencies || []).join(' / ') || '—' }}</td>
            <td>{{ (w.seat_winners || []).join(', ') || '—' }}</td>
            <td>
              <template v-if="w.local_2026 && w.local_2026.length">
                <span class="winner-bar" :style="{ width: '10px', background: partyColours[w.local_2026[0].party] ?? '#888' }"></span>
                {{ w.local_2026[0].party }} ({{ w.local_2026[0].share.toFixed(0) }}%)
              </template>
              <span v-else>—</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </template>
</template>

