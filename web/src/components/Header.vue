<template>
  <header class="h-16 border-b border-gray-200 flex items-center justify-between px-6">
    <!-- Logo and Title -->
    <div class="flex items-center space-x-3">
      <div class="w-8 h-8">
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" class="w-full h-full">
          <defs>
            <linearGradient id="mainGrad" cx="50%" cy="30%" r="70%">
              <stop offset="0%" style="stop-color:#6366f1"/>
              <stop offset="100%" style="stop-color:#4f46e5"/>
            </linearGradient>
            <linearGradient id="accentGrad" cx="50%" cy="50%" r="50%">
              <stop offset="0%" style="stop-color:#06b6d4"/>
              <stop offset="100%" style="stop-color:#0891b2"/>
            </linearGradient>
          </defs>
          
          <!-- Main Container -->
          <rect x="4" y="4" width="24" height="24" rx="6" 
                fill="url(#mainGrad)"/>
          
          <!-- Central Hub -->
          <circle cx="16" cy="16" r="5" fill="url(#accentGrad)" opacity="0.9"/>
          
          <!-- Hub Connections -->
          <g fill="url(#accentGrad)" opacity="0.7">
            <circle cx="16" cy="9" r="1.5"/>   <!-- Top -->
            <circle cx="23" cy="16" r="1.5"/>  <!-- Right -->
            <circle cx="16" cy="23" r="1.5"/>  <!-- Bottom -->
            <circle cx="9" cy="16" r="1.5"/>   <!-- Left -->
          </g>
          
          <!-- Connection Lines -->
          <g stroke="url(#accentGrad)" stroke-width="1" opacity="0.5" fill="none">
            <line x1="16" y1="11" x2="16" y2="10.5"/>  <!-- Top -->
            <line x1="21" y1="16" x2="21.5" y2="16"/>  <!-- Right -->
            <line x1="16" y1="21" x2="16" y2="21.5"/>  <!-- Bottom -->
            <line x1="11" y1="16" x2="10.5" y2="16"/>  <!-- Left -->
          </g>
          
          <!-- Central Intelligence Indicator -->
          <circle cx="16" cy="16" r="2" fill="#ffffff" opacity="0.9"/>
          <circle cx="16" cy="16" r="0.8" fill="url(#mainGrad)"/>
        </svg>
      </div>
      <h1 class="text-xl font-semibold text-gray-800">AgentSmith-HUB</h1>
    </div>
    
    <!-- Action Buttons -->
    <div class="flex items-center space-x-3">
      <button @click="openGitHub" class="btn btn-ghost-primary btn-sm flex items-center">
        <svg class="w-4 h-4 mr-2" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg">
          <path fill-rule="evenodd" d="M10 0C4.477 0 0 4.484 0 10.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0110 4.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.203 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.942.359.31.678.921.678 1.856 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0020 10.017C20 4.484 15.522 0 10 0z" clip-rule="evenodd"></path>
        </svg>
        GitHub
      </button>
      <button @click="logout" class="btn btn-ghost-primary btn-sm">
        Sign out
      </button>
    </div>
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
    },
    openGitHub() {
      window.open('https://github.com/EBWi11/AgentSmith-HUB', '_blank');
    }
  }
}
</script> 