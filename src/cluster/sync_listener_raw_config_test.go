package cluster

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/project"
	"testing"
)

func TestSyncListenerMaintainsSkillRawConfigOnIncrementalSync(t *testing.T) {
	skillID := "sync-skill"
	rawOne := "description: demo\ncontent: first\n"
	rawTwo := "description: demo\ncontent: second\n"

	prevSkill, skillExists := project.GetSkill(skillID)
	prevRaw, rawExists := common.GetRawConfig("skill", skillID)

	defer func() {
		if skillExists {
			project.SetSkill(skillID, prevSkill)
		} else {
			project.DeleteSkill(skillID)
		}
		if rawExists {
			common.SetRawConfig("skill", skillID, prevRaw)
		} else {
			common.DeleteRawConfig("skill", skillID)
		}
	}()

	project.DeleteSkill(skillID)
	common.DeleteRawConfig("skill", skillID)

	sl := &SyncListener{}

	if err := sl.createComponentInstance("skill", skillID, rawOne); err != nil {
		t.Fatalf("createComponentInstance failed: %v", err)
	}
	if got, ok := common.GetRawConfig("skill", skillID); !ok || got != rawOne {
		t.Fatalf("expected follower create to update raw config, got ok=%v raw=%q", ok, got)
	}

	if err := sl.updateComponentInstance("skill", skillID, rawTwo); err != nil {
		t.Fatalf("updateComponentInstance failed: %v", err)
	}
	if got, ok := common.GetRawConfig("skill", skillID); !ok || got != rawTwo {
		t.Fatalf("expected follower update to refresh raw config, got ok=%v raw=%q", ok, got)
	}

	if err := sl.deleteComponentInstance("skill", skillID); err != nil {
		t.Fatalf("deleteComponentInstance failed: %v", err)
	}
	if _, ok := common.GetRawConfig("skill", skillID); ok {
		t.Fatal("expected follower delete to clear raw config")
	}
}
