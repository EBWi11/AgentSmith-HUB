<template>
  <div class="agent-trace-viewer min-w-0">
    <div
      v-if="steps.length === 0"
      class="text-sm text-gray-500 py-2"
    >
      No trace data (e.g. timed out before any LLM call).
    </div>

    <template v-else>
      <div class="space-y-3">
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
              <span class="text-xs font-bold text-gray-800 tabular-nums">Step {{ i + 1 }}</span>
              <span
                v-if="step.round != null && step.round !== ''"
                class="text-[10px] text-gray-500 tabular-nums"
                title="ReAct turn index (same number can repeat: one LLM call + one or more tool runs per turn)"
              >
                Turn R{{ step.round }}
              </span>
              <span v-if="formatStepTimestamp(step.at)" class="text-[10px] text-gray-500 tabular-nums">
                {{ formatStepTimestamp(step.at) }}
              </span>
              <span
                class="text-xs px-2 py-0.5 rounded-full font-medium"
                :class="kindBadgeClass(step)"
              >
                {{ stepLabel(step) }}
              </span>
              <span v-if="stepKind(step) === 'llm' && step.model" class="text-xs text-gray-600 font-mono truncate max-w-[min(100%,18rem)]">
                {{ step.model }}
              </span>
              <span v-if="stepKind(step) === 'tool' && step.tool_name" class="text-xs text-gray-700 font-mono truncate max-w-[min(100%,24rem)]">
                {{ step.tool_name }}
              </span>
              <span v-if="stepKind(step) === 'tool' && step.tool_kind" class="text-[10px] uppercase tracking-wide text-gray-500">
                {{ step.tool_kind }}
              </span>
              <span v-if="stepKind(step) === 'llm' && step.output_tool_calls?.length" class="text-xs text-gray-500">
                {{ step.output_tool_calls.length }} planned tool(s)
              </span>
              <span v-if="stepKind(step) === 'llm' && step.error" class="text-xs text-red-600 truncate max-w-[min(100%,20rem)]">
                {{ step.error }}
              </span>
            </summary>

            <div class="px-3 py-3 space-y-3 border-l-4 border-gray-300 ml-3 mr-2 mb-2 bg-gray-50/60 rounded-r-md">
              <!-- LLM: INPUT / OUTPUT sections (collapsible, default open) -->
              <template v-if="stepKind(step) === 'llm'">
                <details class="trace-section rounded-md border border-slate-200 bg-white shadow-sm" open>
                  <summary class="cursor-pointer list-none px-3 py-2 text-xs font-bold uppercase tracking-wide text-slate-700 bg-slate-100/90 border-b border-slate-200 [&::-webkit-details-marker]:hidden flex items-center gap-2">
                    <span class="text-slate-400">▸</span> Input
                  </summary>
                  <div class="p-2 space-y-2">
                    <p
                      v-if="llmInputMetaByStepIndex[i]?.mode === 'placeholder'"
                      class="text-xs text-slate-600 leading-relaxed"
                    >
                      The LLM request is stored as <span class="font-mono font-semibold bg-slate-100 px-1 rounded">[omitted]</span> in logs (to reduce size and avoid duplication).
                      Full JSON is shown in <strong>Input</strong>.
                    </p>
                    <p
                      v-else-if="llmInputMetaByStepIndex[i]?.hint"
                      class="text-[11px] text-slate-600 leading-snug bg-amber-50/80 border border-amber-100 rounded px-2 py-1.5"
                    >
                      {{ llmInputMetaByStepIndex[i].hint }}
                    </p>
                    <template v-if="llmInputMetaByStepIndex[i]?.mode === 'empty'">
                      <p class="text-xs text-gray-500 px-1">No input messages recorded.</p>
                    </template>
                    <template
                      v-else-if="llmInputMetaByStepIndex[i] && llmInputMetaByStepIndex[i].mode !== 'placeholder'"
                    >
                      <div v-if="llmInputMetaByStepIndex[i].showDeltaBlock" class="space-y-1">
                        <div class="text-[10px] font-semibold text-slate-500 uppercase tracking-wide px-0.5">
                          {{ llmInputMetaByStepIndex[i].deltaTitle }}
                        </div>
                        <pre
                          class="text-xs text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed max-h-[28rem] overflow-y-auto bg-slate-50/80 rounded p-2 border border-slate-100"
                        >{{ formatJsonBlock(llmInputMetaByStepIndex[i].delta) }}</pre>
                      </div>
                    </template>
                    <details v-if="hasText(step.reasoning_content)" class="rounded border border-violet-100 bg-violet-50/50">
                      <summary class="cursor-pointer px-2 py-1.5 text-xs font-semibold text-violet-900 list-none [&::-webkit-details-marker]:hidden">Reasoning</summary>
                      <pre class="px-2 pb-2 text-xs text-gray-800 whitespace-pre-wrap break-words font-mono">{{ step.reasoning_content }}</pre>
                    </details>
                    <details v-if="step.thinking_blocks" class="rounded border border-violet-100 bg-violet-50/40">
                      <summary class="cursor-pointer px-2 py-1.5 text-xs font-semibold text-violet-900 list-none [&::-webkit-details-marker]:hidden">Thinking blocks</summary>
                      <pre class="px-2 pb-2 text-xs text-gray-800 whitespace-pre-wrap break-words font-mono">{{ formatJsonBlock(step.thinking_blocks) }}</pre>
                    </details>
                  </div>
                </details>

                <details class="trace-section rounded-md border border-blue-200 bg-white shadow-sm" open>
                  <summary class="cursor-pointer list-none px-3 py-2 text-xs font-bold uppercase tracking-wide text-blue-900 bg-blue-50/90 border-b border-blue-100 [&::-webkit-details-marker]:hidden flex items-center gap-2">
                    <span class="text-blue-400">▸</span> Output
                  </summary>
                  <div class="p-2 space-y-2">
                    <pre
                      v-if="hasText(step.output_content)"
                      class="text-xs text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed max-h-[20rem] overflow-y-auto bg-blue-50/30 rounded p-2 border border-blue-100"
                    >{{ formatJsonBlock(step.output_content) }}</pre>
                    <p v-else class="text-xs text-gray-500 px-1">No assistant text (tool-only response).</p>
                    <div v-if="step.output_tool_calls && step.output_tool_calls.length" class="space-y-1">
                      <div class="text-[10px] font-semibold text-gray-500 uppercase px-1">Planned tool calls</div>
                      <div
                        v-for="(tc, j) in step.output_tool_calls"
                        :key="tc.id || j"
                        class="border border-purple-100 rounded-md bg-white overflow-hidden"
                      >
                        <div class="px-2 py-1.5 text-xs font-mono bg-purple-50/80 flex items-center gap-2">
                          <span class="font-semibold text-purple-800">{{ tc.name }}</span>
                          <span v-if="tc.id" class="text-gray-500">#{{ tc.id }}</span>
                        </div>
                        <pre
                          v-if="shouldShowPlannedToolArgs(tc.arguments)"
                          class="px-2 pb-2 pt-2 border-t border-purple-50 text-xs font-mono whitespace-pre-wrap break-words bg-white"
                        >
                          {{ formatJsonBlock(tc.arguments) }}
                        </pre>
                      </div>
                    </div>
                  </div>
                </details>
              </template>

              <!-- Tool: INPUT / OUTPUT (collapsible, default open) -->
              <template v-else-if="stepKind(step) === 'tool'">
                <details v-if="hasText(step.arguments)" class="trace-section rounded-md border border-emerald-200 bg-white shadow-sm" open>
                  <summary class="cursor-pointer list-none px-3 py-2 text-xs font-bold uppercase tracking-wide text-emerald-900 bg-emerald-50/90 border-b border-emerald-100 [&::-webkit-details-marker]:hidden flex items-center gap-2">
                    <span class="text-emerald-500">▸</span> Input
                  </summary>
                  <pre class="p-2 text-xs text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed max-h-[24rem] overflow-y-auto">{{ formatJsonBlock(step.arguments) }}</pre>
                </details>
                <details class="trace-section rounded-md border border-green-200 bg-white shadow-sm" open>
                  <summary class="cursor-pointer list-none px-3 py-2 text-xs font-bold uppercase tracking-wide text-green-900 bg-green-50/90 border-b border-green-100 [&::-webkit-details-marker]:hidden flex items-center gap-2">
                    <span class="text-green-600">▸</span> Output
                  </summary>
                  <pre class="p-2 text-xs text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed max-h-[28rem] overflow-y-auto">{{ formatJsonBlock(step.result) }}</pre>
                </details>
              </template>

              <!-- Legacy assistant -->
              <template v-else-if="stepKind(step) === 'legacy_assistant'">
                <details v-if="hasText(step.content)" class="trace-section rounded-md border border-blue-200 bg-white shadow-sm" open>
                  <summary class="cursor-pointer px-3 py-2 text-xs font-bold uppercase tracking-wide text-blue-900 bg-blue-50/90 border-b border-blue-100 list-none [&::-webkit-details-marker]:hidden">Output</summary>
                  <pre class="p-2 text-xs text-gray-800 whitespace-pre-wrap break-words font-mono">{{ step.content }}</pre>
                </details>
                <div v-if="step.tool_calls && step.tool_calls.length" class="space-y-1">
                  <div class="text-[10px] font-semibold text-gray-500 uppercase px-1">Tool calls</div>
                  <div
                    v-for="(tc, j) in step.tool_calls"
                    :key="tc.id || j"
                    class="border border-gray-200 rounded-md bg-white overflow-hidden"
                  >
                    <details class="trace-nested-details">
                      <summary class="cursor-pointer list-none flex items-center gap-2 px-2 py-1.5 text-xs font-mono bg-gray-50 [&::-webkit-details-marker]:hidden">
                        <svg class="trace-chevron w-3 h-3 text-gray-400 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                        </svg>
                        <span class="font-semibold text-purple-800">{{ tc.name }}</span>
                      </summary>
                      <pre class="px-2 pb-2 text-xs font-mono border-t">{{ formatJsonBlock(tc.arguments) }}</pre>
                    </details>
                  </div>
                </div>
              </template>

              <!-- Legacy tool -->
              <template v-else-if="stepKind(step) === 'legacy_tool'">
                <details v-if="hasText(step.arguments)" class="trace-section rounded-md border border-emerald-200 bg-white shadow-sm" open>
                  <summary class="cursor-pointer px-3 py-2 text-xs font-bold uppercase tracking-wide text-emerald-900 bg-emerald-50/90 border-b border-emerald-100 list-none [&::-webkit-details-marker]:hidden">Input</summary>
                  <pre class="p-2 text-xs font-mono">{{ formatJsonBlock(step.arguments) }}</pre>
                </details>
                <details class="trace-section rounded-md border border-green-200 bg-white shadow-sm" open>
                  <summary class="cursor-pointer px-3 py-2 text-xs font-bold uppercase tracking-wide text-green-900 bg-green-50/90 border-b border-green-100 list-none [&::-webkit-details-marker]:hidden">Output</summary>
                  <pre class="p-2 text-xs font-mono">{{ formatJsonBlock(step.result) }}</pre>
                </details>
              </template>

              <template v-else>
                <pre class="text-xs whitespace-pre-wrap break-words font-mono p-2">{{ formatJsonBlock(step) }}</pre>
              </template>
            </div>
          </details>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  /** JSON string or already-parsed array from API */
  trace: {
    type: [String, Array],
    default: ''
  },
  /** Top-level step rows open by default */
  defaultOpen: {
    type: Boolean,
    default: false
  }
})

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

/** Stable compare for OpenAI-style chat messages in trace input arrays. */
function traceMessageFingerprint(m) {
  if (m == null) return 'null'
  if (typeof m !== 'object') {
    try {
      return JSON.stringify(m)
    } catch {
      return String(m)
    }
  }
  try {
    return JSON.stringify({
      role: String(m.role || '').toLowerCase(),
      content: m.content != null ? String(m.content) : '',
      tool_call_id: m.tool_call_id != null ? String(m.tool_call_id) : '',
      tool_calls: m.tool_calls != null ? m.tool_calls : undefined,
      reasoning_content: m.reasoning_content != null ? String(m.reasoning_content) : ''
    })
  } catch {
    return String(m)
  }
}

function llmInputCommonPrefixLen(prev, curr) {
  if (!Array.isArray(prev) || !Array.isArray(curr)) return 0
  const n = Math.min(prev.length, curr.length)
  let j = 0
  while (j < n && traceMessageFingerprint(prev[j]) === traceMessageFingerprint(curr[j])) {
    j++
  }
  return j
}

/**
 * Per-step LLM input UI: backend snapshots the full conversation (minus system) every call,
 * so later rounds repeat earlier messages — we show only the suffix vs the previous LLM step.
 */
function buildLlmInputMeta(step, stepIndex, allSteps) {
  const curr = step.input_messages
  // Persisted logs replace input_messages with "[omitted]" (see common.CompactAgentLogTraceJSON).
  if (typeof curr === 'string' && String(curr).trim() !== '') {
    return {
      mode: 'placeholder',
      hint: null,
      showDeltaBlock: false,
      delta: [],
      deltaTitle: '',
      showFullRequest: false,
      fullRequestOpen: false,
      totalMessages: 0
    }
  }
  if (!Array.isArray(curr) || curr.length === 0) {
    return {
      mode: 'empty',
      hint: null,
      showDeltaBlock: false,
      delta: [],
      deltaTitle: '',
      showFullRequest: false,
      fullRequestOpen: false,
      totalMessages: 0
    }
  }

  let prev = null
  for (let k = stepIndex - 1; k >= 0; k--) {
    const s = allSteps[k]
    if (stepKind(s) === 'llm' && Array.isArray(s.input_messages)) {
      prev = s.input_messages
      break
    }
  }

  if (!prev || prev.length === 0) {
    return {
      mode: 'first',
      hint: null,
      showDeltaBlock: true,
      delta: curr,
      deltaTitle: 'Messages (first call)',
      showFullRequest: false,
      fullRequestOpen: false,
      totalMessages: curr.length
    }
  }

  const prefixLen = llmInputCommonPrefixLen(prev, curr)
  if (prefixLen === 0) {
    return {
      mode: 'mismatch',
      hint:
        'Could not align this input with the previous LLM step (prefix mismatch). Showing the full message list.',
      showDeltaBlock: true,
      delta: curr,
      deltaTitle: 'Messages (this call)',
      showFullRequest: false,
      fullRequestOpen: false,
      totalMessages: curr.length
    }
  }

  const delta = curr.slice(prefixLen)
  if (delta.length === 0) {
    return {
      mode: 'nodelta',
      hint: 'No new messages compared to the previous LLM call. Full payload is expanded below.',
      showDeltaBlock: false,
      delta: [],
      deltaTitle: '',
      showFullRequest: true,
      fullRequestOpen: true,
      totalMessages: curr.length
    }
  }

  return {
    mode: 'delta',
    hint: `Each LLM call logs the full chat (system omitted). Below: only the ${delta.length} new message(s); the first ${prefixLen} repeat earlier turns.`,
    showDeltaBlock: true,
    delta,
    deltaTitle: `New in this call (${delta.length} message(s))`,
    showFullRequest: true,
    fullRequestOpen: false,
    totalMessages: curr.length
  }
}

const llmInputMetaByStepIndex = computed(() => {
  const list = steps.value
  return list.map((step, i) => (stepKind(step) === 'llm' ? buildLlmInputMeta(step, i, list) : null))
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

function shouldShowPlannedToolArgs(arg) {
  if (!hasText(arg)) return false
  const s = String(arg).trim()
  if (s === '{}' || s === '[]' || s === 'null') return false
  // If it's valid JSON and represents an empty object/array, hide it.
  try {
    const parsed = JSON.parse(s)
    if (parsed && typeof parsed === 'object') {
      if (Array.isArray(parsed)) return parsed.length > 0
      return Object.keys(parsed).length > 0
    }
    return true
  } catch {
    return true
  }
}

function formatStepTimestamp(raw) {
  if (!raw) return ''
  const d = new Date(raw)
  if (Number.isNaN(d.getTime())) return String(raw)
  return d.toLocaleString()
}

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
</script>

<style scoped>
.trace-step-details[open] > summary .trace-chevron,
.trace-nested-details[open] > summary .trace-chevron {
  transform: rotate(90deg);
}

.trace-section[open] > summary .text-slate-400,
.trace-section[open] > summary .text-blue-400,
.trace-section[open] > summary .text-emerald-500,
.trace-section[open] > summary .text-green-600 {
  display: inline-block;
  transform: rotate(90deg);
}

.trace-section > summary::-webkit-details-marker,
.trace-nested-details > summary::-webkit-details-marker {
  display: none;
}
</style>
