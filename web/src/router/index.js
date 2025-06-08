import { createRouter, createWebHistory } from 'vue-router';
import Login from '../views/Login.vue';
import MainLayout from '../views/MainLayout.vue';
import { hubApi } from '../api/index.js';
import ComponentDetail from '../components/ComponentDetail.vue';

const routes = [
  {
    path: '/',
    name: 'Login',
    component: Login,
  },
  {
    path: '/dashboard',
    redirect: '/app'
  },
  {
    path: '/app',
    component: MainLayout,
    meta: { requiresAuth: true }
  }
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

router.beforeEach(async (to, from, next) => {
  const loggedIn = !!localStorage.getItem('auth_token');

  if (to.matched.some(record => record.meta.requiresAuth)) {
    if (!loggedIn) {
      next({ name: 'Login' });
    } else {
      try {
        await hubApi.verifyToken();
        next();
      } catch (error) {
        hubApi.clearToken();
        next({ name: 'Login' });
      }
    }
  } else if (to.name === 'Login' && loggedIn) {
    next({ path: '/app' });
  }
  else {
    next();
  }
});

export default router; 