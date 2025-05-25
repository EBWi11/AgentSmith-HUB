package project

import (
	"fmt"
	"io/ioutil"
	"strings"

	"gopkg.in/yaml.v3"
)

type ProjectNode interface {
	Start() error
	Stop() error
}

// Project represents the project configuration
type Project struct {
	Id      string `yaml:"id"`
	Name    string `yaml:"name"`
	Content string `yaml:"content"`
}

// LoadProject loads project configuration from a YAML file
func LoadProject(filePath string) (*Project, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read project file: %v", err)
	}

	var project Project
	if err := yaml.Unmarshal(data, &project); err != nil {
		return nil, fmt.Errorf("failed to parse project file: %v", err)
	}

	if err := project.Validate(); err != nil {
		return nil, fmt.Errorf("invalid project: %v", err)
	}

	return &project, nil
}

// Validate validates the project configuration
func (p *Project) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("project name is required")
	}

	if p.Id == "" {
		return fmt.Errorf("project id is required")
	}

	if p.Content == "" {
		return fmt.Errorf("project content is required")
	}

	if err := validateProjectContent(p.Content); err != nil {
		return fmt.Errorf("invalid project content: %v", err)
	}

	return nil
}

// validateProjectContent checks content for allowed node types and cycles
func validateProjectContent(content string) error {
	lines := strings.Split(content, "\n")
	allowedTypes := map[string]struct{}{
		"INPUT":   {},
		"OUTPUT":  {},
		"RULESET": {},
	}

	type edge struct{ from, to string }
	graph := make(map[string][]string)
	nodes := make(map[string]string) // nodeName -> type

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "->")
		if len(parts) != 2 {
			return fmt.Errorf("invalid line: %q", line)
		}
		from := strings.TrimSpace(parts[0])
		to := strings.TrimSpace(parts[1])

		fromType, _ := parseNode(from)
		toType, _ := parseNode(to)

		if _, ok := allowedTypes[fromType]; !ok {
			return fmt.Errorf("invalid node type: %s in %q", fromType, from)
		}
		if _, ok := allowedTypes[toType]; !ok {
			return fmt.Errorf("invalid node type: %s in %q", toType, to)
		}

		// New rules:
		// INPUT can only be from, never to
		if toType == "INPUT" {
			return fmt.Errorf("INPUT node %q cannot be a destination", to)
		}
		// OUTPUT can only be to, never from
		if fromType == "OUTPUT" {
			return fmt.Errorf("OUTPUT node %q cannot be a source", from)
		}

		nodes[from] = fromType
		nodes[to] = toType

		graph[from] = append(graph[from], to)
	}

	// Detect cycles using DFS
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	for node := range nodes {
		if hasCycle(node, graph, visited, recStack) {
			return fmt.Errorf("cycle detected in project content")
		}
	}

	return nil
}

// parseNode splits "TYPE.name" into ("TYPE", "name")
func parseNode(s string) (string, string) {
	parts := strings.SplitN(s, ".", 2)
	if len(parts) != 2 {
		return strings.ToUpper(strings.TrimSpace(s)), ""
	}
	return strings.ToUpper(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1])
}

// hasCycle detects cycles in the graph using DFS
func hasCycle(node string, graph map[string][]string, visited, recStack map[string]bool) bool {
	if recStack[node] {
		return true
	}
	if visited[node] {
		return false
	}
	visited[node] = true
	recStack[node] = true
	for _, neighbor := range graph[node] {
		if hasCycle(neighbor, graph, visited, recStack) {
			return true
		}
	}
	recStack[node] = false
	return false
}
