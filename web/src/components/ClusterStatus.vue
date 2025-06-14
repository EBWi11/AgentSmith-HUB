<template>
  <div class="p-6">
    <h2 class="text-xl font-bold mb-4">Cluster Status</h2>
    <div v-if="loading">Loading...</div>
    <div v-else-if="error" class="text-red-500">{{ error }}</div>
    <div v-else>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
        <div class="bg-blue-50 rounded p-4">
          <div class="text-xs text-gray-500">Node ID</div>
          <div class="font-semibold">{{ cluster.SelfID }}</div>
        </div>
        <div class="bg-blue-50 rounded p-4">
          <div class="text-xs text-gray-500">Node Address</div>
          <div class="font-semibold">{{ cluster.SelfAddress }}</div>
        </div>
        <div class="bg-green-50 rounded p-4">
          <div class="text-xs text-gray-500">Role</div>
          <div class="font-semibold">{{ cluster.Status === 'leader' ? 'Leader' : 'Follower' }}</div>
        </div>
        <div class="bg-blue-50 rounded p-4">
          <div class="text-xs text-gray-500">Leader ID</div>
          <div class="font-semibold">{{ cluster.LeaderID }}</div>
        </div>
        <div class="bg-blue-50 rounded p-4">
          <div class="text-xs text-gray-500">Leader Address</div>
          <div class="font-semibold">{{ cluster.LeaderAddress }}</div>
        </div>
      </div>
      <div class="mb-6">
        <h3 class="font-bold mb-2">Cluster Parameters</h3>
        <ul class="text-sm space-y-1">
          <li>Heartbeat Interval: {{ nsToSec(cluster.HeartbeatInterval) }} seconds</li>
          <li>Heartbeat Timeout: {{ nsToSec(cluster.HeartbeatTimeout) }} seconds</li>
          <li>Cleanup Interval: {{ nsToSec(cluster.CleanupInterval) }} seconds</li>
          <li>Max Miss Count: {{ cluster.MaxMissCount }}</li>
        </ul>
      </div>
      <div v-if="cluster.Nodes && Object.keys(cluster.Nodes).length > 0">
        <h3 class="font-bold mb-2">Node List</h3>
        <table class="min-w-full text-xs border">
          <thead>
            <tr class="bg-gray-100">
              <th class="p-2 border">Node ID</th>
              <th class="p-2 border">Address</th>
              <th class="p-2 border">Status</th>
              <th class="p-2 border">Last Heartbeat</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(node, id) in cluster.Nodes" :key="id">
              <td class="p-2 border">{{ id }}</td>
              <td class="p-2 border">{{ node.Address }}</td>
              <td class="p-2 border">{{ node.Status }}</td>
              <td class="p-2 border">{{ node.LastHeartbeat }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { hubApi } from '../api'

const cluster = ref({})
const loading = ref(true)
const error = ref(null)

onMounted(async () => {
  try {
    cluster.value = await hubApi.fetchClusterInfo()
  } catch (e) {
    error.value = 'Failed to get cluster information'
  } finally {
    loading.value = false
  }
})

function nsToSec(ns) {
  if (!ns) return '-'
  return Math.round(Number(ns) / 1e9)
}
</script> 