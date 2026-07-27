package api

import (
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
	"sort"
	"strings"
)

type projectUsageIndex struct {
	rulesets map[string][]string
	inputs   map[string][]string
	outputs  map[string][]string
}

func buildProjectUsageIndex() projectUsageIndex {
	index := projectUsageIndex{
		rulesets: make(map[string][]string),
		inputs:   make(map[string][]string),
		outputs:  make(map[string][]string),
	}

	project.ForEachProject(func(projectID string, proj *project.Project) bool {
		for _, rulesetComponent := range proj.Rulesets {
			if rulesetComponent != nil && rulesetComponent.RulesetID != "" {
				index.rulesets[rulesetComponent.RulesetID] = appendUniqueUsage(index.rulesets[rulesetComponent.RulesetID], proj.Id)
			}
		}
		for _, inputComponent := range proj.Inputs {
			if inputComponent != nil && inputComponent.Id != "" {
				index.inputs[inputComponent.Id] = appendUniqueUsage(index.inputs[inputComponent.Id], proj.Id)
			}
		}
		for _, outputComponent := range proj.Outputs {
			if outputComponent != nil && outputComponent.Id != "" {
				index.outputs[outputComponent.Id] = appendUniqueUsage(index.outputs[outputComponent.Id], proj.Id)
			}
		}
		return true
	})

	sortUsageIndex(index.rulesets)
	sortUsageIndex(index.inputs)
	sortUsageIndex(index.outputs)
	return index
}

func buildPluginUsageIndex(includeUsage bool) map[string][]string {
	if !includeUsage {
		return map[string][]string{}
	}

	index := make(map[string][]string)
	knownPlugins := getKnownPluginNames()
	project.ForEachRuleset(func(rulesetID string, r *rules_engine.Ruleset) bool {
		for pluginName := range collectRulesetPluginUsage(r, knownPlugins) {
			index[pluginName] = appendUniqueUsage(index[pluginName], r.RulesetID)
		}
		return true
	})
	sortUsageIndex(index)
	return index
}

func collectRulesetPluginUsage(r *rules_engine.Ruleset, knownPlugins map[string]struct{}) map[string]struct{} {
	usedPlugins := make(map[string]struct{})
	if r == nil {
		return usedPlugins
	}

	for _, rule := range r.Rules {
		for _, checklist := range rule.ChecklistMap {
			for _, node := range checklist.CheckNodes {
				recordPluginValue(usedPlugins, knownPlugins, node.Value)
			}
		}
		for _, node := range rule.CheckMap {
			recordPluginValue(usedPlugins, knownPlugins, node.Value)
		}
		for _, appendElem := range rule.AppendsMap {
			recordPluginValue(usedPlugins, knownPlugins, appendElem.Value)
		}
		for _, pluginElem := range rule.PluginMap {
			recordPluginValue(usedPlugins, knownPlugins, pluginElem.Value)
		}
	}

	return usedPlugins
}

func recordPluginValue(usedPlugins map[string]struct{}, knownPlugins map[string]struct{}, value string) {
	if value == "" {
		return
	}
	for _, pluginName := range extractPluginNamesFromValue(value, knownPlugins) {
		usedPlugins[pluginName] = struct{}{}
	}
}

func extractPluginNamesFromValue(value string, knownPlugins map[string]struct{}) []string {
	names := make([]string, 0)
	segments := strings.Split(value, "(")
	for i := 0; i < len(segments)-1; i++ {
		candidate := strings.TrimSpace(segments[i])
		if candidate == "" {
			continue
		}
		lastToken := strings.Fields(candidate)
		if len(lastToken) == 0 {
			continue
		}
		name := strings.TrimSpace(lastToken[len(lastToken)-1])
		name = strings.Trim(name, `"'`)
		if name == "" {
			continue
		}
		if strings.ContainsAny(name, " \t\n\r<>=,") {
			continue
		}
		if _, exists := knownPlugins[name]; !exists {
			continue
		}
		names = appendUniqueUsage(names, name)
	}
	return names
}

func getKnownPluginNames() map[string]struct{} {
	names := make(map[string]struct{})
	for name := range plugin.GetAllPlugins() {
		names[name] = struct{}{}
	}
	for name := range plugin.GetAllPluginsNew() {
		names[name] = struct{}{}
	}
	return names
}

func appendUniqueUsage(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func sortUsageIndex(index map[string][]string) {
	for key := range index {
		sort.Strings(index[key])
	}
}
