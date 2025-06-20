<template>
  <header class="h-16 border-b border-gray-200 flex items-center justify-end px-6">
    <button @click="logout" class="btn btn-ghost-primary btn-sm">
      Sign out
        </button>
  </header>
</template>

<script>
import { hubApi } from '../api';

export default {
  name: 'Header',
  methods: {
    logout() {
      hubApi.clearToken();
      this.$router.push('/');
    }
  }
}
</script> 