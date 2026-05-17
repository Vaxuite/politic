import { createRouter, createWebHistory } from 'vue-router'
import Home from './views/Home.vue'
import Constituency from './views/Constituency.vue'
import Ward from './views/Ward.vue'
import Council from './views/Council.vue'

export default createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: Home },
    { path: '/constituency/:code', name: 'constituency', component: Constituency, props: true },
    { path: '/ward/:code', name: 'ward', component: Ward, props: true },
    { path: '/council/:code', name: 'council', component: Council, props: true },
  ],
})
