<script setup>
import { computed } from 'vue'
import { Line } from 'vue-chartjs'
import {
  Chart as ChartJS, CategoryScale, LinearScale, PointElement,
  LineElement, Title, Tooltip, Legend,
} from 'chart.js'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend)

const props = defineProps({
  data: { type: Object, required: true }, // { elections, series }
})

const partyColours = {
  Labour: '#d4222b',
  Conservative: '#1f6cd1',
  'Liberal Democrats': '#f1a51c',
  'Reform UK': '#12b6cf',
  UKIP: '#6b2887',
  Green: '#5cb462',
  SNP: '#f8d44c',
  'Plaid Cymru': '#3f8c2d',
  DUP: '#b34a00',
  'Sinn Féin': '#1e6b3c',
  SDLP: '#3a8a4c',
  UUP: '#3760b0',
  Alliance: '#f9c441',
  Other: '#888888',
}

const chartData = computed(() => ({
  labels: props.data.elections.map(d => new Date(d).getFullYear()),
  datasets: props.data.series.map(s => ({
    label: s.party,
    data: s.shares,
    borderColor: partyColours[s.party] ?? '#444',
    backgroundColor: partyColours[s.party] ?? '#444',
    tension: 0.25,
    borderWidth: 2,
    pointRadius: 3,
  })),
}))

const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index', intersect: false },
  scales: {
    y: {
      title: { display: true, text: 'Share of vote (%)' },
      min: 0,
    },
    x: { title: { display: true, text: 'General election' } },
  },
  plugins: {
    legend: { position: 'bottom' },
    tooltip: {
      callbacks: {
        label: (ctx) => `${ctx.dataset.label}: ${ctx.parsed.y.toFixed(1)}%`,
      },
    },
  },
}
</script>

<template>
  <div class="chart-wrap">
    <Line :data="chartData" :options="chartOptions" />
  </div>
</template>
