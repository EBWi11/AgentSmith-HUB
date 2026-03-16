<template>
  <div class="h-full flex flex-col bg-white">
    <div class="flex items-center justify-between p-4 border-b border-gray-200">
      <div>
        <h1 class="text-xl font-semibold text-gray-900">Memory</h1>
        <p class="text-sm text-gray-500 mt-1">PNS-scoped agent memory generated from comment and revert analysis.</p>
      </div>
      <button
        @click="refresh"
        :disabled="loading"
        class="btn btn-secondary btn-sm"
      >
        Refresh
      </button>
    </div>

    <div class="p-4 border-b border-gray-200 bg-gray-50">
      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Search</label>
          <input v-model="filters.keyword" type="text" placeholder="PNS, project, agent" class="filter-input" />
        </div>
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Project</label>
          <input v-model="filters.projectId" type="text" placeholder="project id" class="filter-input" />
        </div>
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Agent</label>
          <input v-model="filters.agentId" type="text" placeholder="agent id" class="filter-input" />
        </div>
        <div class="flex items-end">
          <div class="text-sm text-gray-500">{{ filteredItems.length }} scope(s)</div>
        </div>
      </div>
    </div>

    <div class="flex-1 min-h-0 grid grid-cols-1 lg:grid-cols-[380px_minmax(0,1fr)]">
      <div class="border-r border-gray-200 overflow-y-auto">
        <div v-if="loading" class="p-4 text-sm text-gray-500">Loading memory scopes...</div>
        <div v-else-if="error" class="p-4 text-sm text-red-600">{{ error }}</div>
        <div v-else-if="filteredItems.length === 0" class="p-4 text-sm text-gray-500">No memory scopes found.</div>
        <button
          v-for="item in filteredItems"
          :key="item.project_node_sequence"
          @click="selectScope(item)"
          class="w-full text-left p-4 border-b border-gray-100 hover:bg-gray-50 transition-colors"
          :class="{ 'bg-blue-50': selectedScope?.project_node_sequence === item.project_node_sequence }"
        >
          <div class="flex items-center justify-between">
            <div class="font-medium text-gray-900 truncate">{{ item.project_node_sequence }}</div>
            <span class="text-xs px-2 py-1 rounded-full bg-gray-100 text-gray-700">v{{ item.version || 1 }}</span>
          </div>
          <div class="mt-2 text-sm text-gray-600 space-y-1">
            <div v-if="item.project_id">Project: {{ item.project_id }}</div>
            <div v-if="item.agent_id">Agent: {{ item.agent_id }}</div>
            <div v-if="item.input_ids?.length">Inputs: {{ item.input_ids.join(', ') }}</div>
            <div class="text-xs text-gray-500">
              {{ item.summary_count }} summaries, {{ item.recent_feedback_count }} feedback entries
            </div>
          </div>
        </button>
      </div>

      <div class="overflow-y-auto">
        <div v-if="detailLoading" class="p-6 text-sm text-gray-500">Loading memory detail...</div>
        <div v-else-if="detailError" class="p-6 text-sm text-red-600">{{ detailError }}</div>
        <div v-else-if="!detail" class="p-6 text-sm text-gray-500">Select a memory scope to view details.</div>
        <div v-else class="p-6 space-y-6">
          <div>
            <h2 class="text-lg font-semibold text-gray-900">{{ detail.scope.project_node_sequence }}</h2>
            <div class="mt-2 grid grid-cols-1 md:grid-cols-2 gap-3 text-sm text-gray-600">
              <div v-if="detail.scope.project_id">Project: {{ detail.scope.project_id }}</div>
              <div v-if="detail.scope.agent_id">Agent: {{ detail.scope.agent_id }}</div>
              <div v-if="detail.scope.input_ids?.length">Inputs: {{ detail.scope.input_ids.join(', ') }}</div>
              <div v-if="detail.path" class="break-all">Path: {{ detail.path }}</div>
            </div>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div class="border border-gray-200 rounded-lg p-4 bg-gray-50">
              <div class="text-sm text-gray-500">Summaries</div>
              <div class="mt-1 text-2xl font-semibold text-gray-900">{{ detail.data.summaries?.length || 0 }}</div>
            </div>
            <div class="border border-gray-200 rounded-lg p-4 bg-gray-50">
              <div class="text-sm text-gray-500">Avoid Patterns</div>
              <div class="mt-1 text-2xl font-semibold text-gray-900">{{ detail.data.avoid_patterns?.length || 0 }}</div>
            </div>
            <div class="border border-gray-200 rounded-lg p-4 bg-gray-50">
              <div class="text-sm text-gray-500">Recent Feedback</div>
              <div class="mt-1 text-2xl font-semibold text-gray-900">{{ detail.data.recent_feedback?.length || 0 }}</div>
            </div>
          </div>

          <div v-if="detail.data.summaries?.length">
            <h3 class="text-sm font-medium text-gray-900 mb-2">Summaries</h3>
            <div class="space-y-2">
              <div v-for="(item, index) in detail.data.summaries" :key="index" class="border border-gray-200 rounded-lg p-4">
                <div class="flex items-center justify-between">
                  <div class="font-medium text-gray-900">{{ item.category || 'general' }}</div>
                  <div class="text-xs text-gray-500" v-if="item.confidence">confidence {{ Number(item.confidence).toFixed(2) }}</div>
                </div>
                <div class="mt-2 text-sm text-gray-700 whitespace-pre-wrap">{{ item.summary }}</div>
                <div class="mt-2 text-xs text-gray-500 break-all" v-if="item.source_operation_id">
                  Source operation: {{ item.source_operation_id }}
                </div>
              </div>
            </div>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
            <div>
              <h3 class="text-sm font-medium text-gray-900 mb-2">Avoid Patterns</h3>
              <div v-if="detail.data.avoid_patterns?.length" class="space-y-2">
                <div v-for="(item, index) in detail.data.avoid_patterns" :key="index" class="px-3 py-2 rounded-lg bg-red-50 text-red-800 text-sm border border-red-100">
                  {{ item }}
                </div>
              </div>
              <div v-else class="text-sm text-gray-500">No avoid patterns recorded.</div>
            </div>
            <div>
              <h3 class="text-sm font-medium text-gray-900 mb-2">Preferred Patterns</h3>
              <div v-if="detail.data.preferred_patterns?.length" class="space-y-2">
                <div v-for="(item, index) in detail.data.preferred_patterns" :key="index" class="px-3 py-2 rounded-lg bg-green-50 text-green-800 text-sm border border-green-100">
                  {{ item }}
                </div>
              </div>
              <div v-else class="text-sm text-gray-500">No preferred patterns recorded.</div>
            </div>
          </div>

          <div v-if="detail.data.signals?.length">
            <h3 class="text-sm font-medium text-gray-900 mb-2">Signals</h3>
            <div class="space-y-2">
              <div v-for="(item, index) in detail.data.signals" :key="index" class="px-3 py-2 rounded-lg bg-amber-50 text-amber-800 text-sm border border-amber-100">
                {{ item }}
              </div>
            </div>
          </div>

          <div v-if="detail.data.recent_feedback?.length">
            <h3 class="text-sm font-medium text-gray-900 mb-2">Recent Feedback</h3>
            <div class="space-y-2">
              <div v-for="(item, index) in detail.data.recent_feedback" :key="index" class="border border-gray-200 rounded-lg p-4">
                <div class="flex items-center justify-between gap-2">
                  <div class="text-sm text-gray-900 break-all">Operation: {{ item.operation_id }}</div>
                  <span class="text-xs px-2 py-1 rounded-full bg-gray-100 text-gray-700">{{ item.feedback_type || 'feedback' }}</span>
                </div>
                <div v-if="item.revert_operation_id" class="text-sm text-gray-600 break-all mt-1">Revert: {{ item.revert_operation_id }}</div>
                <div v-if="item.source_operation_id" class="text-sm text-gray-600 break-all mt-1">Feedback Event: {{ item.source_operation_id }}</div>
                <div v-if="item.ruleset_id || item.rule_id" class="text-sm text-gray-600 mt-1">
                  {{ item.ruleset_id }} <span v-if="item.rule_id">/ {{ item.rule_id }}</span>
                </div>
                <div v-if="item.reason" class="text-sm text-gray-700 mt-2 whitespace-pre-wrap">{{ item.reason }}</div>
              </div>
            </div>
          </div>

          <div>
            <h3 class="text-sm font-medium text-gray-900 mb-2">Raw YAML</h3>
            <div class="bg-gray-100 rounded-md" style="height: 320px;">
              <MonacoEditor
                :value="detail.raw || ''"
                language="yaml"
                :read-only="true"
                :diff-mode="false"
                style="height: 100%; width: 100%;"
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { hubApi } from '../api'
import MonacoEditor from './MonacoEditor.vue'

const loading = ref(false)
const error = ref(null)
const items = ref([])
const selectedScope = ref(null)
const detail = ref(null)
const detailLoading = ref(false)
const detailError = ref(null)

const filters = ref({
  keyword: '',
  projectId: '',
  agentId: ''
})

const filteredItems = computed(() => {
  const keyword = filters.value.keyword.trim().toLowerCase()
  const projectId = filters.value.projectId.trim().toLowerCase()
  const agentId = filters.value.agentId.trim().toLowerCase()

  return items.value.filter(item => {
    if (keyword) {
      const haystack = [
        item.project_node_sequence,
        item.project_id,
        item.agent_id,
        ...(item.input_ids || [])
      ].filter(Boolean).join(' ').toLowerCase()
      if (!haystack.includes(keyword)) return false
    }
    if (projectId && !(item.project_id || '').toLowerCase().includes(projectId)) return false
    if (agentId && !(item.agent_id || '').toLowerCase().includes(agentId)) return false
    return true
  })
})

async function refresh() {
  loading.value = true
  error.value = null
  try {
    const data = await hubApi.getMemoryList()
    items.value = Array.isArray(data) ? data : []
    if (selectedScope.value) {
      const matched = items.value.find(item => item.project_node_sequence === selectedScope.value.project_node_sequence)
      if (matched) {
        selectedScope.value = matched
        await loadDetail(matched.project_node_sequence)
      } else {
        selectedScope.value = null
        detail.value = null
      }
    } else if (items.value.length > 0) {
      await selectScope(items.value[0])
    }
  } catch (e) {
    error.value = e.response?.data?.error || e.message || 'Failed to load memory list'
  } finally {
    loading.value = false
  }
}

async function loadDetail(pns) {
  detailLoading.value = true
  detailError.value = null
  try {
    detail.value = await hubApi.getMemoryDetail(pns)
  } catch (e) {
    detailError.value = e.response?.data?.error || e.message || 'Failed to load memory detail'
  } finally {
    detailLoading.value = false
  }
}

async function selectScope(item) {
  selectedScope.value = item
  await loadDetail(item.project_node_sequence)
}

onMounted(() => {
  refresh()
})
</script>
