<template>
  <div ref="rootRef" class="agent-trace-viewer min-w-0">
    <div
      v-if="steps.length === 0"
      class="text-sm text-gray-500 py-2"
    >
      No trace data (e.g. timed out before any LLM call).
    </div>

    <template v-else>
      <div class="flex flex-wrap items-center gap-2 mb-3">
        <button
          type="button"
          class="text-xs px-2 py-1 rounded border border-gray-300 bg-white hover:bg-gray-50 text-gray-700"
          @click="setAllOpen(true)"
        >
          Expand all
        </button>
        <button
          type="button"
          class="text-xs px-2 py-1 rounded border border-gray-300 bg-white hover:bg-gray-50 text-gray-700"
          @click="setAllOpen(false)"
        >
          Collapse all
        </button>
      </div>

      <div class="space-y-2">
        <div
          v-for="(step, i) in steps"
          :key="i"
          class="trace-step rounded-lg border border-gray-200 bg-white overflow-hidden"
        >
          <details class="trace-step-details" :open="defaultOpen">
            <summary
              class="cursor-pointer list-none flex flex-wrap items-center gap-2 px-3 py-2.5 text-sm font-medium select-none
                     hover:bg-gray-50 border-b border-gray-100 [&::-webkit-details-marker]:hidden"
            >
              <svg
                class="trace-chevron w-4 h-4 text-gray-400 shrink-0 transition-transform duration-200"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
              </svg>
              <span class="text-xs font-semibold text-gray-500 tabular-nums">Round {{ step.round }}</span>
              <span v-if="formatStepTimestamp(step.at)" class="text-[10px] text-gray-500 tabular-nums">
                {{ formatStepTimestamp(step.at) }}
              </span>
              <span
                class="text-xs px-2 py-0.5 rounded-full font-medium"
                :class="kindBadgeClass(step)"
              >
                {{ stepLabel(step) }}
              </span>
              <span v-if="stepKind(step) === 'llm' && step.model" class="text-xs text-gray-600 font-mono truncate max-w-[min(100%,20rem)]">
                {{ step.model }}
              </span>
              <span v-if="stepKind(step) === 'tool' && step.tool_name" class="text-xs text-gray-700 font-mono truncate max-w-[min(100%,28rem)]">
                {{ step.tool_name }}
              </span>
              <span v-if="stepKind(step) === 'tool' && step.tool_kind" class="text-[10px] uppercase tracking-wide text-gray-500">
                {{ step.tool_kind }}
              </span>
              <span v-if="stepKind(step) === 'llm' && step.output_tool_calls?.length" class="text-xs text-gray-500">
                {{ step.output_tool_calls.length }} tool call(s)
              </span>
              <span v-if="stepKind(step) === 'llm' && step.error" class="text-xs text-red-600 truncate max-w-[min(100%,24rem)]">
                {{ step.error }}
              </span>
              <span v-if="stepKind(step) === 'legacy_assistant' && step.tool_calls?.length" class="text-xs text-gray-500">
                {{ step.tool_calls.length }} tool call(s)
              </span>
            </summary>

            <div class="px-3 py-3 space-y-3 border-l-4 border-gray-200 ml-3 mr-2 mb-2 bg-gray-50/80 rounded-r-md">
              <!-- New format: full LLM request/response -->
              <template v-if="stepKind(step) === 'llm'">
                <details class="trace-nested rounded border border-slate-200 bg-slate-50/50">
                  <summary class="cursor-pointer px-2 py-1.5 text-xs font-semibold text-slate-800 list-none [&::-webkit-details-marker]:hidden flex items-center gap-1">
                    <span class="text-gray-400">▸</span> LLM input (excluding system prompt)
                  </summary>
                  <pre class="px-2 pb-2 pt-0 text-xs text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed max-h-[32rem] overflow-y-auto">{{ formatJsonBlock(step.input_messages) }}</pre>
                </details>

                <details v-if="hasText(step.reasoning_content)" class="trace-nested rounded border border-violet-100 bg-violet-50/40">
                  <summary class="cursor-pointer px-2 py-1.5 text-xs font-semibold text-violet-900 list-none [&::-webkit-details-marker]:hidden flex items-center gap-1">
                    <span class="text-gray-400">▸</span> Reasoning / thinking
                  </summary>
                  <pre class="px-2 pb-2 text-xs text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed">{{ step.reasoning_content }}</pre>
                </details>

                <details v-if="step.thinking_blocks" class="trace-nested rounded border border-violet-100 bg-violet-50/30">
                  <summary class="cursor-pointer px-2 py-1.5 text-xs font-semibold text-violet-900 list-none [&::-webkit-details-marker]:hidden flex items-center gap-1">
                    <span class="text-gray-400">▸</span> Thinking blocks (raw)
                  </summary>
                  <pre class="px-2 pb-2 text-xs text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed">{{ formatJsonBlock(step.thinking_blocks) }}</pre>
                </details>

                <details v-if="hasText(step.output_content)" class="trace-nested rounded border border-blue-100 bg-blue-50/40">
                  <summary class="cursor-pointer px-2 py-1.5 text-xs font-semibold text-blue-800 list-none [&::-webkit-details-marker]:hidden flex items-center gap-1">
                    <span class="text-gray-400">▸</span> LLM output (assistant content)
                  </summary>
                  <pre class="px-2 pb-2 text-xs text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed">{{ formatJsonBlock(step.output_content) }}</pre>
                </details>

                <div v-if="step.output_tool_calls && step.output_tool_calls.length" class="space-y-2">
                  <div class="text-xs font-semibold text-gray-600 uppercase tracking-wide">Tool calls (from model)</div>
                  <div
                    v-for="(tc, j) in step.output_tool_calls"
                    :key="tc.id || j"
                    class="ml-1 border border-gray-200 rounded-md bg-white overflow-hidden"
                  >
                    <details class="trace-nested-details">
                      <summary
                        class="cursor-pointer list-none flex items-center gap-2 px-2 py-2 text-xs font-mono text-gray-900 bg-gray-100/80 hover:bg-gray-100 [&::-webkit-details-marker]:hidden"
                      >
                        <svg class="trace-chevron w-3 h-3 text-gray-400 shrink-0 transition-transform duration-200" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                        </svg>
                        <span class="font-semibold text-purple-800">{{ tc.name }}</span>
                        <span v-if="tc.id" class="text-gray-500 truncate">#{{ tc.id }}</span>
                      </summary>
                      <div class="px-2 py-2 border-t border-gray-100 ml-2 border-l-2 border-purple-200 pl-3">
                        <div class="text-[10px] font-semibold text-gray-500 mb-1">Arguments</div>
                        <pre class="text-xs text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed">{{ formatJsonBlock(tc.arguments) }}</pre>
                      </div>
                    </details>
                  </div>
                </div>
              </template>

              <!-- New format: tool execution -->
              <template v-else-if="stepKind(step) === 'tool'">
                <details v-if="hasText(step.arguments)" class="trace-nested rounded border border-emerald-100 bg-emerald-50/30">
                  <summary class="cursor-pointer px-2 py-1.5 text-xs font-semibold text-emerald-900 list-none [&::-webkit-details-marker]:hidden flex items-center gap-1">
                    <span class="text-gray-400">▸</span> Tool input (arguments)
                  </summary>
                  <pre class="px-2 pb-2 text-xs text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed">{{ formatJsonBlock(step.arguments) }}</pre>
                </details>
                <details class="trace-nested rounded border border-green-100 bg-green-50/30">
                  <summary class="cursor-pointer px-2 py-1.5 text-xs font-semibold text-green-900 list-none [&::-webkit-details-marker]:hidden flex items-center gap-1">
                    <span class="text-gray-400">▸</span> Tool output (result)
                  </summary>
                  <pre class="px-2 pb-2 text-xs text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed">{{ formatJsonBlock(step.result) }}</pre>
                </details>
                <div v-if="step.tool_call_id" class="text-[10px] text-gray-500 font-mono">tool_call_id: {{ step.tool_call_id }}</div>
              </template>

              <!-- Legacy: assistant -->
              <template v-else-if="stepKind(step) === 'legacy_assistant'">
                <details v-if="hasText(step.content)" class="trace-nested rounded border border-blue-100 bg-blue-50/40">
                  <summary class="cursor-pointer px-2 py-1.5 text-xs font-semibold text-blue-800 list-none [&::-webkit-details-marker]:hidden flex items-center gap-1">
                    <span class="text-gray-400">▸</span> Assistant message
                  </summary>
                  <pre class="px-2 pb-2 pt-0 text-xs text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed">{{ step.content }}</pre>
                </details>

                <div v-if="step.tool_calls && step.tool_calls.length" class="space-y-2">
                  <div class="text-xs font-semibold text-gray-600 uppercase tracking-wide">Tool calls</div>
                  <div
                    v-for="(tc, j) in step.tool_calls"
                    :key="tc.id || j"
                    class="ml-1 border border-gray-200 rounded-md bg-white overflow-hidden"
                  >
                    <details class="trace-nested-details">
                      <summary
                        class="cursor-pointer list-none flex items-center gap-2 px-2 py-2 text-xs font-mono text-gray-900 bg-gray-100/80 hover:bg-gray-100 [&::-webkit-details-marker]:hidden"
                      >
                        <svg class="trace-chevron w-3 h-3 text-gray-400 shrink-0 transition-transform duration-200" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                        </svg>
                        <span class="font-semibold text-purple-800">{{ tc.name }}</span>
                        <span v-if="tc.id" class="text-gray-500 truncate">#{{ tc.id }}</span>
                      </summary>
                      <div class="px-2 py-2 border-t border-gray-100 ml-2 border-l-2 border-purple-200 pl-3">
                        <div class="text-[10px] font-semibold text-gray-500 mb-1">Arguments</div>
                        <pre class="text-xs text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed">{{ formatJsonBlock(tc.arguments) }}</pre>
                      </div>
                    </details>
                  </div>
                </div>
              </template>

              <!-- Legacy: tool -->
              <template v-else-if="stepKind(step) === 'legacy_tool'">
                <details v-if="hasText(step.arguments)" class="trace-nested rounded border border-emerald-100 bg-emerald-50/30">
                  <summary class="cursor-pointer px-2 py-1.5 text-xs font-semibold text-emerald-900 list-none [&::-webkit-details-marker]:hidden flex items-center gap-1">
                    <span class="text-gray-400">▸</span> Arguments
                  </summary>
                  <pre class="px-2 pb-2 text-xs text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed">{{ formatJsonBlock(step.arguments) }}</pre>
                </details>
                <details class="trace-nested rounded border border-green-100 bg-green-50/30">
                  <summary class="cursor-pointer px-2 py-1.5 text-xs font-semibold text-green-900 list-none [&::-webkit-details-marker]:hidden flex items-center gap-1">
                    <span class="text-gray-400">▸</span> Result
                  </summary>
                  <pre class="px-2 pb-2 text-xs text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed">{{ formatJsonBlock(step.result) }}</pre>
                </details>
                <div v-if="step.tool_call_id" class="text-[10px] text-gray-500 font-mono">tool_call_id: {{ step.tool_call_id }}</div>
              </template>

              <template v-else>
                <pre class="text-xs whitespace-pre-wrap break-words font-mono">{{ formatJsonBlock(step) }}</pre>
              </template>
            </div>
          </details>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'

const props = defineProps({
  /** JSON string or already-parsed array from API */
  trace: {
    type: [String, Array],
    default: ''
  },
  /** Top-level steps open by default */
  defaultOpen: {
    type: Boolean,
    default: false
  }
})

const rootRef = ref(null)

const steps = computed(() => {
  const t = props.trace
  if (t === null || t === undefined || t === '') return []
  if (Array.isArray(t)) return t.filter(Boolean)
  try {
    const parsed = JSON.parse(String(t))
    return Array.isArray(parsed) ? parsed.filter(Boolean) : []
  } catch {
    return []
  }
})

function stepKind(step) {
  if (!step || typeof step !== 'object') return 'unknown'
  if (step.type === 'llm') return 'llm'
  if (step.type === 'tool') return 'tool'
  if (step.role === 'assistant') return 'legacy_assistant'
  if (step.role === 'tool') return 'legacy_tool'
  return 'unknown'
}

function stepLabel(step) {
  const k = stepKind(step)
  if (k === 'llm') return 'llm'
  if (k === 'tool') return 'tool'
  if (k === 'legacy_assistant') return 'assistant (legacy)'
  if (k === 'legacy_tool') return 'tool (legacy)'
  return step.role || 'step'
}

function kindBadgeClass(step) {
  const k = stepKind(step)
  if (k === 'llm') return 'bg-indigo-100 text-indigo-800'
  if (k === 'tool' || k === 'legacy_tool') return 'bg-emerald-100 text-emerald-800'
  if (k === 'legacy_assistant') return 'bg-blue-100 text-blue-800'
  return 'bg-gray-100 text-gray-700'
}

function hasText(s) {
  return s !== null && s !== undefined && String(s).trim() !== ''
}

function formatStepTimestamp(raw) {
  if (!raw) return ''
  const d = new Date(raw)
  if (Number.isNaN(d.getTime())) return String(raw)
  return d.toLocaleString()
}

/**
 * Pretty-print if value is JSON string or object; otherwise return as string.
 */
function formatJsonBlock(raw) {
  if (raw === null || raw === undefined) return ''
  if (typeof raw === 'object') {
    try {
      return JSON.stringify(raw, null, 2)
    } catch {
      return String(raw)
    }
  }
  const text = String(raw)
  try {
    const parsed = JSON.parse(text)
    if (typeof parsed === 'string') {
      try {
        return JSON.stringify(JSON.parse(parsed), null, 2)
      } catch {
        return parsed
      }
    }
    return JSON.stringify(parsed, null, 2)
  } catch {
    return text
  }
}

function setAllOpen(open) {
  const root = rootRef.value
  if (!root) return
  root.querySelectorAll('details').forEach((el) => {
    el.open = open
  })
}
</script>

<style scoped>
.trace-step-details[open] > summary .trace-chevron,
.trace-nested-details[open] > summary .trace-chevron {
  transform: rotate(90deg);
}

.trace-nested > summary::-webkit-details-marker {
  display: none;
}
</style>
