<script setup>
import { ref, watch, computed } from 'vue'
import { db } from '../db.js'
import { Bar } from 'vue-chartjs'
import {
  Chart as ChartJS, CategoryScale, LinearScale, BarElement, Title, Tooltip, Legend,
} from 'chart.js'

ChartJS.register(CategoryScale, LinearScale, BarElement, Title, Tooltip, Legend)

const props = defineProps({ code: String })
const data = ref(null)
const error = ref(null)

async function load() {
  data.value = null; error.value = null
  try { data.value = await db.constituency(props.code) }
  catch (e) { error.value = e.message }
}
watch(() => props.code, load, { immediate: true })

const partyColours = {
  Labour: '#d4222b', 'Labour and Co-operative': '#d4222b',
  Conservative: '#1f6cd1', 'Liberal Democrats': '#f1a51c', 'Liberal Democrat': '#f1a51c',
  'Reform UK': '#12b6cf', 'Brexit Party': '#12b6cf',
  'UK Independence Party (UKIP)': '#6b2887',
  'Green Party': '#5cb462', 'Scottish Green Party': '#5cb462',
  'Scottish National Party (SNP)': '#f8d44c', 'Scottish National Party': '#f8d44c',
  'Plaid Cymru - The Party of Wales': '#3f8c2d',
}
function colourFor(p) { return partyColours[p] ?? '#888' }

const winnerByYear = computed(() => {
  if (!data.value) return []
  return data.value.results.map(r => ({
    year: r.year,
    winner: r.candidates[0],
    summary: r.result_summary,
  }))
})

function chartFor(block) {
  const total = block.valid_votes
  return {
    labels: block.candidates.map(c => `${c.party}`),
    datasets: [{
      label: 'Share %',
      data: block.candidates.map(c => total > 0 ? (c.votes * 100 / total) : 0),
      backgroundColor: block.candidates.map(c => colourFor(c.party)),
    }],
  }
}
const chartOpts = {
  indexAxis: 'y',
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: false },
    tooltip: { callbacks: { label: (ctx) => `${ctx.parsed.x.toFixed(1)}%` } },
  },
  scales: { x: { min: 0, max: 100, title: { display: true, text: 'Share (%)' } } },
}

function pct(votes, total) {
  if (!total) return '—'
  return ((votes * 100) / total).toFixed(1) + '%'
}
</script>

<template>
  <div v-if="error" class="error">Couldn't load constituency: {{ error }}</div>
  <div v-else-if="!data" class="loading">Loading…</div>
  <template v-else>
    <p class="crumb">{{ data.country }}<span v-if="data.region"> · {{ data.region }}</span></p>
    <h2 class="title">{{ data.name }}</h2>
    <p class="subtitle">Westminster constituency · <span class="pill">{{ data.code }}</span></p>

    <div class="card">
      <h2>Recent winners</h2>
      <table>
        <thead>
          <tr><th>Year</th><th>Winner</th><th>Party</th><th>Result</th></tr>
        </thead>
        <tbody>
          <tr v-for="w in winnerByYear" :key="w.year">
            <td>{{ w.year }}</td>
            <td>{{ w.winner.given_name }} {{ w.winner.family_name }}</td>
            <td>
              <span class="winner-bar" :style="{ width: '10px', background: colourFor(w.winner.party) }"></span>
              {{ w.winner.party }}
            </td>
            <td>{{ w.summary }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-for="block in data.results" :key="block.polling_date" class="card">
      <h2>{{ block.year }} general election — {{ block.result_summary }}</h2>
      <p class="subtitle" style="margin-bottom: 0.5rem">
        Electorate {{ block.electorate.toLocaleString() }} · valid votes {{ block.valid_votes.toLocaleString() }}
        <span v-if="block.majority"> · majority {{ block.majority.toLocaleString() }}</span>
      </p>
      <div class="grid-2">
        <div class="chart-wrap" style="height: 320px">
          <Bar :data="chartFor(block)" :options="chartOpts" />
        </div>
        <table>
          <thead>
            <tr><th>#</th><th>Candidate</th><th>Party</th><th class="num">Votes</th><th class="num">Share</th></tr>
          </thead>
          <tbody>
            <tr v-for="c in block.candidates" :key="c.position + c.family_name">
              <td>{{ c.position }}</td>
              <td>{{ c.given_name }} {{ c.family_name }}<span v-if="c.sitting_mp"> *</span></td>
              <td>{{ c.party }}</td>
              <td class="num">{{ c.votes.toLocaleString() }}</td>
              <td class="num">{{ pct(c.votes, block.valid_votes) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div class="card">
      <h2>Wards in this constituency ({{ data.wards.length }})</h2>
      <table>
        <thead>
          <tr><th>Ward</th><th>Council</th><th>Split?</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="w in data.wards" :key="w.code + w.lad_code">
            <td><router-link :to="{ name: 'ward', params: { code: w.code } }">{{ w.name }}</router-link></td>
            <td><router-link :to="{ name: 'council', params: { code: w.lad_code } }">{{ w.lad_name }}</router-link></td>
            <td>
              <span v-if="w.split_ward" class="pill">split{{ w.primary === false ? ' · minor' : '' }}</span>
              <span v-else>—</span>
            </td>
            <td><span class="meta">{{ w.code }}</span></td>
          </tr>
        </tbody>
      </table>
    </div>
  </template>
</template>
