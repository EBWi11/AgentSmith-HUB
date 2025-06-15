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
  // 简化路由守卫逻辑，防止无限刷新
  // 移除了token验证逻辑，因为它可能导致在token无效时页面不断刷新
  // 现在只检查token是否存在，而不验证其有效性
  // 如果token无效，将在API请求时处理
  const loggedIn = !!localStorage.getItem('auth_token');

  if (to.matched.some(record => record.meta.requiresAuth)) {
    // 需要认证的路由
    if (!loggedIn) {
      next({ name: 'Login' });
    } else {
      // 有token就直接通过，不进行验证
      next();
    }
  } else if (to.name === 'Login' && loggedIn) {
    // 已登录用户访问登录页，重定向到应用页面
    next({ path: '/app' });
  } else {
    // 其他情况正常通过
    next();
  }
});

export default router; 