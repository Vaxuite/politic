<script setup>
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { db } from '../db.js'

const router = useRouter()
const query = ref('')
const hits = ref([])
const open = ref(false)
const loading = ref(false)
let token = 0

watch(query, async (q) => {
  const mine = ++token
  if (!q || q.trim().length < 2) {
    hits.value = []
    open.value = false
    return
  }
  loading.value = true
  try {
    const res = await db.search(q.trim())
    if (mine !== token) return
    hits.value = res.hits
    open.value = true
  } catch {
    if (mine === token) hits.value = []
  } finally {
    if (mine === token) loading.value = false
  }
})

function go(hit) {
  open.value = false
  query.value = ''
  router.push({ name: hit.type, params: { code: hit.code } })
}

function close() {
  setTimeout(() => { open.value = false }, 150)
}
</script>

<template>
  <div class="search">
    <input
      v-model="query"
      placeholder="Search constituency, ward or council…"
      @focus="open = hits.length > 0"
      @blur="close"
    />
    <div v-if="open" class="search-results">
      <div v-if="loading" class="meta" style="padding:0.5rem 0.75rem">Searching…</div>
      <div v-else-if="!hits.length" class="meta" style="padding:0.5rem 0.75rem">No matches.</div>
      <a v-else v-for="hit in hits" :key="hit.type + hit.code" href="#" @click.prevent="go(hit)">
        <span>
          <span class="tag">{{ hit.type }}</span>
          &nbsp;{{ hit.name }}
        </span>
        <span class="meta">{{ hit.context }}</span>
      </a>
    </div>
  </div>
</template>
