<template>
  <div class="flex items-center justify-center min-h-screen bg-gray-50">
    <div class="w-full max-w-md p-8 space-y-8 bg-white rounded-lg shadow-md">
      <div>
        <h2 class="text-3xl font-extrabold text-center text-gray-900">
          Sign in to AgentSmith
        </h2>
      </div>
      <form class="mt-8 space-y-6" @submit.prevent="login">
        <div class="rounded-md shadow-sm -space-y-px">
          <div>
            <label for="token" class="sr-only">Token</label>
            <input id="token" name="token" type="password" v-model="token" required
                   class="relative block w-full px-3 py-2 text-gray-900 placeholder-gray-500 border border-gray-300 rounded-md focus:outline-none focus:ring-blue-500 focus:border-blue-500 focus:z-10 sm:text-sm"
                   placeholder="Enter your authentication token">
          </div>
        </div>

        <div v-if="error" class="text-sm text-red-600">
          {{ error }}
        </div>

        <div>
          <button type="submit" :disabled="loading"
                  class="relative flex justify-center w-full px-4 py-2 text-sm font-medium text-white bg-blue-600 border border-transparent rounded-md group hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:bg-blue-300">
            <span v-if="loading" class="absolute inset-y-0 left-0 flex items-center pl-3">
              <svg class="w-5 h-5 text-white animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            </span>
            Sign In
          </button>
        </div>
        <div v-if="oidcEnabled">
          <button type="button" @click="loginOIDC" :disabled="loading"
                  class="relative flex justify-center w-full px-4 py-2 text-sm font-medium text-white bg-green-600 border border-transparent rounded-md group hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 disabled:bg-green-300">
            Use Single Sign-On
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script>
import { hubApi } from '../api';
import { getUserManager, isOidcEnabled } from '../api/oidc';

export default {
  name: 'Login',
  data() {
    return {
      token: '',
      loading: false,
      error: null,
      oidcEnabled: isOidcEnabled(),
    };
  },
  created() {
    // 清除可能存在的刷新状态
    localStorage.removeItem('crazyRefreshActive');
    localStorage.removeItem('refreshCount');
    localStorage.removeItem('totalRefreshes');
  },
  methods: {
    async login() {
      this.loading = true;
      this.error = null;
      try {
        hubApi.setToken(this.token);
        await hubApi.verifyToken();
        this.$router.push('/dashboard');
      } catch (err) {
        hubApi.clearToken();
        this.error = 'Login failed. Please check your token.';
      } finally {
        this.loading = false;
      }
    },
    async loginOIDC() {
      this.loading = true;
      try {
        await getUserManager().signinRedirect();
      } catch (e) {
        this.error = 'OIDC Login failed. Please check your settings.';
        console.error(e);
      }
      finally {
        this.loading = false;
      }
    }
  }
};
</script> 