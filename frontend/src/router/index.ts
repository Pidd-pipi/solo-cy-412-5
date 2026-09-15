import {createRouter, createWebHashHistory} from 'vue-router';
import Login from '../pages/Login.vue';
import Dashboard from '../pages/Dashboard.vue';
import Repairs from '../pages/Repairs.vue';
import Payments from '../pages/Payments.vue';
import Announcements from '../pages/Announcements.vue';
import Facilities from '../pages/Facilities.vue';
import Inspections from '../pages/Inspections.vue';
import Profile from '../pages/Profile.vue';
import {installGuards} from './guards';

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {path: '/login', component: Login},
    {path: '/', redirect: '/dashboard'},
    {path: '/dashboard', component: Dashboard, meta: {permission: 'repair:manage'}},
    {path: '/repairs', component: Repairs, meta: {permission: 'repair:manage'}},
    {path: '/payments', component: Payments},
    {path: '/announcements', component: Announcements, meta: {permission: 'announcement:publish'}},
    {path: '/facilities', component: Facilities, meta: {permission: 'inspection:manage'}},
    {path: '/inspections', component: Inspections, meta: {permission: 'inspection:manage'}},
    {path: '/profile', component: Profile}
  ]
});
installGuards(router);
export default router;
