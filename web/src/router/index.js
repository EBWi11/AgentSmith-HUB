import { createRouter, createWebHistory } from 'vue-router';
import Login from '../views/Login.vue';
import MainLayout from '../views/MainLayout.vue';
import Dashboard from '../views/Dashboard.vue';
import ComponentDetail from '../components/ComponentDetail.vue';
import ErrorLogs from '../views/ErrorLogs.vue';
import { hubApi } from '../api/index.js';

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
        path: '',
        name: 'Dashboard',
        component: Dashboard,
        meta: { requiresAuth: true, componentType: 'home' }
      },
      {
        path: 'inputs/:id',
        name: 'InputDetail',
        component: ComponentDetail,
        props: route => ({ 
          id: route.params.id,
          type: 'inputs',
          isEdit: true
        }),
        meta: { requiresAuth: true, componentType: 'inputs' }
      },
      {
        path: 'outputs/:id',
        name: 'OutputDetail',
        component: ComponentDetail,
        props: route => ({ 
          id: route.params.id,
          type: 'outputs',
          isEdit: true
        }),
        meta: { requiresAuth: true, componentType: 'outputs' }
      },
      {
        path: 'rulesets/:id',
        name: 'RulesetDetail',
        component: ComponentDetail,
        props: route => ({ 
          id: route.params.id,
          type: 'rulesets',
          isEdit: true
        }),
        meta: { requiresAuth: true, componentType: 'rulesets' }
      },
      {
        path: 'plugins/:id',
        name: 'PluginDetail',
        component: ComponentDetail,
        props: route => ({ 
          id: route.params.id,
          type: 'plugins',
          isEdit: true
        }),
        meta: { requiresAuth: true, componentType: 'plugins' }
      },
      {
        path: 'projects/:id',
        name: 'ProjectDetail',
        component: ComponentDetail,
        props: route => ({ 
          id: route.params.id,
          type: 'projects',
          isEdit: true
        }),
        meta: { requiresAuth: true, componentType: 'projects' }
      },
      {
        path: 'cluster',
        name: 'Cluster',
        component: () => import('../components/ClusterStatus.vue'),
        meta: { requiresAuth: true, componentType: 'cluster' }
      },
      {
        path: 'pending-changes',
        name: 'PendingChanges',
        component: () => import('../components/PendingChanges.vue'),
        meta: { requiresAuth: true, componentType: 'pending-changes' }
      },
      {
        path: 'load-local-components',
        name: 'LoadLocalComponents',
        component: () => import('../components/LoadLocalComponents.vue'),
        meta: { requiresAuth: true, componentType: 'load-local-components' }
      },
      {
        path: 'error-logs',
        name: 'ErrorLogs',
        component: ErrorLogs,
        meta: { requiresAuth: true, componentType: 'error-logs' }
      }
    ]
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
      next();
    }
  } else if (to.name === 'Login' && loggedIn) {
    next({ path: '/app' });
  } else {
    next();
  }
});

export default router; 