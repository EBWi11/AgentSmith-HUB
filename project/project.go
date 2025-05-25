package project

import (
	"fmt"
	"strings"
	"sync"
)

// NodeType represents the type of a node in the project graph.
type NodeType string

const (
	NodeTypeInput   NodeType = "INPUT"
	NodeTypeOutput  NodeType = "OUTPUT"
	NodeTypeRuleset NodeType = "RULESET"
)

// ProjectEngine manages the lifecycle and data flow of the project.
type ProjectEngine struct {
	Project   *Project
	Nodes     map[string]ProjectNode
	Channels  map[string]chan map[string]interface{}
	Edges     [][2]string // from, to
	startOnce sync.Once
	stopOnce  sync.Once
}

// NewProjectEngine parses the project content, builds nodes and channels, and returns a ProjectEngine.
func NewProjectEngine(project *Project, nodeFactory func(nodeType, nodeName string) (ProjectNode, error)) (*ProjectEngine, error) {
	lines := strings.Split(project.Content, "\n")
	nodes := make(map[string]ProjectNode)
	channels := make(map[string]chan map[string]interface{})
	edges := make([][2]string, 0)

	// Parse nodes and edges
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "->")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid line: %q", line)
		}
		from := strings.TrimSpace(parts[0])
		to := strings.TrimSpace(parts[1])

		fromType, fromName := parseNode(from)
		toType, toName := parseNode(to)

		// Create nodes if not exist
		if _, ok := nodes[from]; !ok {
			node, err := nodeFactory(fromType, fromName)
			if err != nil {
				return nil, fmt.Errorf("create node %s: %v", from, err)
			}
			nodes[from] = node
		}
		if _, ok := nodes[to]; !ok {
			node, err := nodeFactory(toType, toName)
			if err != nil {
				return nil, fmt.Errorf("create node %s: %v", to, err)
			}
			nodes[to] = node
		}

		// Create channel for this edge if not exist
		edgeKey := from + "->" + to
		if _, ok := channels[edgeKey]; !ok {
			channels[edgeKey] = make(chan map[string]interface{}, 1024)
		}
		edges = append(edges, [2]string{from, to})
	}

	return &ProjectEngine{
		Project:  project,
		Nodes:    nodes,
		Channels: channels,
		Edges:    edges,
	}, nil
}

// Start starts all nodes and connects them via channels according to the project content.
func (pe *ProjectEngine) Start() error {
	var err error
	pe.startOnce.Do(func() {
		// Connect nodes via channels
		for _, edge := range pe.Edges {
			from, to := edge[0], edge[1]
			ch := pe.Channels[from+"->"+to]
			// Set output for from node, input for to node
			if setter, ok := pe.Nodes[from].(interface {
				AddDownStream(chan map[string]interface{})
			}); ok {
				setter.AddDownStream(ch)
			}
			if setter, ok := pe.Nodes[to].(interface {
				AddUpStream(chan map[string]interface{})
			}); ok {
				setter.AddUpStream(ch)
			}
		}
		// Start all nodes
		for _, node := range pe.Nodes {
			if e := node.Start(); e != nil {
				err = e
				break
			}
		}
	})
	return err
}

// Stop stops all nodes in the project.
func (pe *ProjectEngine) Stop() error {
	var err error
	pe.stopOnce.Do(func() {
		for _, node := range pe.Nodes {
			if e := node.Stop(); e != nil {
				err = e
			}
		}
	})
	return err
}
