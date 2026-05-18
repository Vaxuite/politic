import { createRouter, createWebHashHistory } from 'vue-router'
import Home from './views/Home.vue'
import Constituency from './views/Constituency.vue'
import Ward from './views/Ward.vue'
import Council from './views/Council.vue'

// Hash history so GitHub Pages serves the same index.html for every URL
// without needing a 404.html fallback.
export default createRouter({
  history: createWebHashHistory(import.meta.env.BASE_URL),
  routes: [
    { path: '/', name: 'home', component: Home },
    { path: '/constituency/:code', name: 'constituency', component: Constituency, props: true },
    { path: '/ward/:code', name: 'ward', component: Ward, props: true },
    { path: '/council/:code', name: 'council', component: Council, props: true },
  ],
})
