<template>
  <div class="flex items-center justify-center min-h-screen">
    <div class="text-center">
      <p class="text-gray-600">Completing sign-in...</p>
    </div>
  </div>
</template>

<script>
import { getUserManager } from '../api/oidc';
import { hubApi } from '../api';
export default {
  name: 'OidcCallback',
  async created() {    
    console.log('OIDC callback');
    try {
      const user = await getUserManager().signinRedirectCallback();
      const token = user?.id_token;
      if (token) {
        hubApi.setBearer(token);
        this.$router.replace('/app');
      } else {
        this.$router.replace('/');
      }
    } catch (e) {
      console.error('OIDC callback error', e);
      this.$router.replace('/');
    }
  }
};
</script>


