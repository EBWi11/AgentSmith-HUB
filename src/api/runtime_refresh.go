package api

import (
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/project"
)

func refreshAffectedProjectsForComponentChange(componentType, componentID string, affectedProjects []string, source string, recordOperation bool) {
	for _, projectID := range affectedProjects {
		p, ok := project.GetProject(projectID)
		if !ok {
			continue
		}

		switch componentType {
		case "ruleset":
			err := p.HotReloadRuleset(componentID, source)
			if err != nil {
				logger.Error("Failed to hot reload ruleset after component change, falling back to restart",
					"project_id", projectID,
					"ruleset_id", componentID,
					"error", err)
				if restartErr := p.Restart(recordOperation, source+"_fallback"); restartErr != nil {
					logger.Error("Fallback project restart failed after ruleset hot reload error",
						"project_id", projectID,
						"ruleset_id", componentID,
						"error", restartErr)
				}
			}
		case "agent":
			err := p.HotReloadAgent(componentID, source)
			if err != nil {
				logger.Error("Failed to hot reload agent after component change, falling back to restart",
					"project_id", projectID,
					"agent_id", componentID,
					"error", err)
				if restartErr := p.Restart(recordOperation, source+"_fallback"); restartErr != nil {
					logger.Error("Fallback project restart failed after agent hot reload error",
						"project_id", projectID,
						"agent_id", componentID,
						"error", restartErr)
				}
			}
		default:
			err := p.Restart(recordOperation, source)
			if err != nil {
				logger.Error("Failed to restart project after component change",
					"project_id", projectID,
					"component_type", componentType,
					"component_id", componentID,
					"error", err)
			}
		}
	}
}
