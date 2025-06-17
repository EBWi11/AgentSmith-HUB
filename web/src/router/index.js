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
    meta: { requiresAuth: true },
    children: [
      {
        path: 'inputs/:id',
        name: 'InputDetail',
        props: true,
        meta: { requiresAuth: true, componentType: 'inputs' }
      },
      {
        path: 'outputs/:id',
        name: 'OutputDetail',
        props: true,
        meta: { requiresAuth: true, componentType: 'outputs' }
      },
      {
        path: 'rulesets/:id',
        name: 'RulesetDetail',
        props: true,
        meta: { requiresAuth: true, componentType: 'rulesets' }
      },
      {
        path: 'plugins/:id',
        name: 'PluginDetail',
        props: true,
        meta: { requiresAuth: true, componentType: 'plugins' }
      },
      {
        path: 'projects/:id',
        name: 'ProjectDetail',
        props: true,
        meta: { requiresAuth: true, componentType: 'projects' }
      },
      {
        path: 'cluster',
        name: 'Cluster',
        meta: { requiresAuth: true, componentType: 'cluster' }
      },
      {
        path: 'pending-changes',
        name: 'PendingChanges',
        meta: { requiresAuth: true, componentType: 'pending-changes' }
      },
      {
        path: 'load-local-components',
        name: 'LoadLocalComponents',
        meta: { requiresAuth: true, componentType: 'load-local-components' }
      }
    ]
  }
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

router.beforeEach((to, from, next) => {
  // Simplify route guard logic to prevent infinite refresh
  // Removed token validation logic as it may cause continuous page refresh when token is invalid
  // Now only checks if token exists, without validating its validity
  // Invalid token will be handled during API requests
  const loggedIn = !!localStorage.getItem('auth_token');

  if (to.matched.some(record => record.meta.requiresAuth)) {
    // Routes requiring authentication
    if (!loggedIn) {
      next({ name: 'Login' });
    } else {
      // Pass through if token exists, without validation
      next();
    }
  } else if (to.name === 'Login' && loggedIn) {
    // Redirect logged-in users to app page when accessing login page
    next({ path: '/app' });
  } else {
    // Pass through normally for other cases
    next();
  }
});

export default router; 