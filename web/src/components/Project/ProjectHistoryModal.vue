<template>
  <div v-if="show" class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50" @click.self="close">
    <div class="relative top-20 mx-auto p-5 border w-1/2 shadow-lg rounded-md bg-white">
      <div class="mt-3 text-center">
        <h3 class="text-lg leading-6 font-medium text-gray-900">Project History for {{ projectId }}</h3>
        <div class="mt-2 px-7 py-3">
          <div v-if="loading" class="text-center">
            <p>Loading history...</p>
          </div>
          <div v-else-if="error" class="text-red-500">
            <p>Error loading history: {{ error }}</p>
          </div>
          <div v-else-if="history.length === 0" class="text-gray-500">
            <p>No history found for this project.</p>
          </div>
          <div v-else>
            <ul class="divide-y divide-gray-200">
              <li v-for="event in history" :key="event.timestamp" class="py-4 flex">
                <div class="ml-3">
                  <p class="text-sm font-medium text-gray-900">{{ formatEventType(event.event_type) }}</p>
                  <p class="text-sm text-gray-500">{{ event.message }}</p>
                  <p class="text-xs text-gray-400">{{ new Date(event.timestamp).toLocaleString('en-US', {
                    year: 'numeric',
                    month: '2-digit',
                    day: '2-digit',
                    hour: '2-digit',
                    minute: '2-digit',
                    second: '2-digit',
                    hour12: false
                  }) }}</p>
                </div>
              </li>
            </ul>
          </div>
        </div>
        <div class="items-center px-4 py-3">
          <button @click="close" class="px-4 py-2 bg-gray-500 text-white text-base font-medium rounded-md w-full shadow-sm hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-gray-300">
            Close
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue';
import { hubApi as api } from '../../api';

const props = defineProps({
  show: Boolean,
  projectId: String,
});

const emit = defineEmits(['close']);

const history = ref([]);
const loading = ref(false);
const error = ref(null);

const fetchHistory = async () => {
  if (!props.projectId) return;
  loading.value = true;
  error.value = null;
  try {
    const response = await api.get(`/projects/${props.projectId}/history`);
    history.value = response.data;
  } catch (err) {
    error.value = err.response?.data?.error || 'An unknown error occurred';
    console.error('Failed to fetch project history:', err);
  } finally {
    loading.value = false;
  }
};

watch(() => props.show, (newVal) => {
  if (newVal) {
    fetchHistory();
  }
});

const formatEventType = (eventType) => {
  switch (eventType) {
    case 1:
      return 'Project Started';
    case 2:
      return 'Project Stopped';
    case 3:
      return 'Configuration Updated';
    default:
      return 'Unknown Event';
  }
};

const close = () => {
  emit('close');
};
</script>
