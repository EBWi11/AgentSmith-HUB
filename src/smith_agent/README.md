# Smith Agent

Smith Agent is a module that performs **architect data analysis** on log/sample data using an LLM, following the Architect step from [AgentSmith](https://github.com/EBWi11/AgentSmith).

## Initialization

- **Only initialized when** `llm_api_key` is set in config (or synced from Redis in cluster) **and** a short LLM probe request succeeds.
- Call `smith_agent.InitIfLLMAvailable()` after `plugin.RegisterLLMCallIfConfigured()` in `main` (already wired).
- Use `smith_agent.Ready()` to check if the agent can run analysis.

## Data Analysis

**Architect** – Summarize log meaning, describe each field, classify (e.g. Network Traffic, System Audit).

## Usage

```go
if !smith_agent.Ready() {
    return // or return error
}
result, err := smith_agent.AnalyzeData(ctx, "my_log_source", sampleJSON)
if err != nil {
    return err
}
// result.ArchitectOutput
```

## Config

Uses the same LLM config as the `llmCall` plugin: `llm_api_key` and `llm_base_url` in `config.yaml` (or from Redis in cluster). No separate agent config.

## Input analysis loop (leader only)

When LLM is ready and the node is leader, a goroutine runs **daily**: for each input, it reads the **latest sample data** from the sampler, runs architect analysis, and stores the result in Redis.

- **Redis key**: `smith_agent:analysis:input:<input_id>`
- **TTL**: 30 days (value is overwritten on each run).
- **Value**: JSON of `AnalysisResult` (e.g. `{"ArchitectOutput":"..."}`).

Started from `main` after `loadLocalProjects()`. First run is 1 minute after start. If no input has sample data yet, the loop retries every **5 minutes** until at least one input has data and analysis is stored; then it switches to **every 24 hours**.
