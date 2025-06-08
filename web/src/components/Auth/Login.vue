<template>
  <div class="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8">
    <div class="max-w-md w-full space-y-8">
      <div>
        <h2 class="mt-6 text-center text-3xl font-extrabold text-gray-900">
          AgentSmith-HUB
        </h2>
        <p class="mt-2 text-center text-sm text-gray-600">
          Please enter your token to continue
        </p>
      </div>
      <form class="mt-8 space-y-6" @submit.prevent="handleLogin">
        <div class="rounded-md shadow-sm -space-y-px">
          <div>
            <label for="token" class="sr-only">Token</label>
            <input
              id="token"
              v-model="token"
              name="token"
              type="password"
              required
              class="appearance-none rounded-md relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 focus:z-10 sm:text-sm"
              placeholder="Enter your token"
            />
          </div>
        </div>

        <div>
          <button
            type="submit"
            :disabled="loading"
            class="group relative w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
          >
            <span v-if="loading">Loading...</span>
            <span v-else>Sign in</span>
          </button>
        </div>

        <div v-if="error" class="text-red-500 text-sm text-center mt-2">
          {{ error }}
        </div>
      </form>
    </div>
  </div>
</template>

<script>
import { hubApi } from '../../services/api';

export default {
  name: 'Login',
  data() {
    return {
      token: '',
      loading: false,
      error: null
    };
  },
  methods: {
    async handleLogin() {
      this.loading = true;
      this.error = null;

      try {
        hubApi.setToken(this.token);
        await hubApi.verifyToken();
        this.$router.push('/dashboard');
      } catch (err) {
        console.error('Login failed:', err);
        this.error = 'Invalid token. Please try again.';
        hubApi.clearToken();
      } finally {
        this.loading = false;
      }
    }
  }
};
</script> 