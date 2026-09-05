package project

import "testing"

func TestGetPNSCanonicalizesLogicalNodes(t *testing.T) {
	tests := []struct {
		name  string
		nodes []FlowNode
	}{
		{
			name: "two inputs converge",
			nodes: []FlowNode{
				{FromType: "INPUT", FromID: "a", ToType: "RULESET", ToID: "shared"},
				{FromType: "INPUT", FromID: "b", ToType: "RULESET", ToID: "shared"},
				{FromType: "RULESET", FromID: "shared", ToType: "OUTPUT", ToID: "out"},
			},
		},
		{
			name: "outbound edge declared first",
			nodes: []FlowNode{
				{FromType: "RULESET", FromID: "shared", ToType: "OUTPUT", ToID: "out"},
				{FromType: "INPUT", FromID: "b", ToType: "RULESET", ToID: "shared"},
				{FromType: "INPUT", FromID: "a", ToType: "RULESET", ToID: "shared"},
			},
		},
		{
			name: "multi-level convergence",
			nodes: []FlowNode{
				{FromType: "INPUT", FromID: "a", ToType: "RULESET", ToID: "shared"},
				{FromType: "INPUT", FromID: "b", ToType: "RULESET", ToID: "shared"},
				{FromType: "RULESET", FromID: "shared", ToType: "AGENT", ToID: "review"},
				{FromType: "INPUT", FromID: "c", ToType: "AGENT", ToID: "review"},
				{FromType: "AGENT", FromID: "review", ToType: "OUTPUT", ToID: "out"},
			},
		},
		{
			name: "fan out keeps one source node",
			nodes: []FlowNode{
				{FromType: "INPUT", FromID: "a", ToType: "RULESET", ToID: "shared"},
				{FromType: "RULESET", FromID: "shared", ToType: "OUTPUT", ToID: "one"},
				{FromType: "RULESET", FromID: "shared", ToType: "OUTPUT", ToID: "two"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Project{FlowNodes: tt.nodes}
			p.getPNS()

			pnsByNode := make(map[string]string)
			for _, node := range p.FlowNodes {
				assertCanonicalNodePNS(t, pnsByNode, getNodeFromKey(node), node.FromPNS)
				assertCanonicalNodePNS(t, pnsByNode, getNodeToKey(node), node.ToPNS)
			}
		})
	}
}

func assertCanonicalNodePNS(t *testing.T, pnsByNode map[string]string, nodeKey, pns string) {
	t.Helper()
	if pns == "" {
		t.Fatalf("node %s has empty PNS", nodeKey)
	}
	if existing, exists := pnsByNode[nodeKey]; exists && existing != pns {
		t.Fatalf("node %s has inconsistent PNS values %q and %q", nodeKey, existing, pns)
	}
	pnsByNode[nodeKey] = pns
}
