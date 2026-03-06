package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/skill"
	"fmt"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
)

const NewSkillData = `description: |
  Describe what expertise or reference material this skill provides.

content: |
  Put the knowledge / reference content here.
  The agent can retrieve this via the get_reference function.
`

func getSkills(c echo.Context) error {
	skills := make([]map[string]interface{}, 0)
	processedIDs := make(map[string]bool)

	project.ForEachSkill(func(id string, s *skill.Skill) bool {
		tempRaw, hasTemp := project.GetSkillNew(id)

		rawConfig := s.RawConfig
		if hasTemp {
			rawConfig = tempRaw
		}

		skillData := map[string]interface{}{
			"id":      id,
			"hasTemp": hasTemp,
			"raw":     rawConfig,
			"status":  string(s.Status),
		}
		if s.Status == common.StatusError && s.Err != nil {
			skillData["errorMessage"] = s.Err.Error()
		}
		if s.Path != "" {
			skillData["path"] = s.Path
		}

		skills = append(skills, skillData)
		processedIDs[id] = true
		return true
	})

	allSkillsNew := project.GetAllSkillsNew()
	for id, tempRaw := range allSkillsNew {
		if !processedIDs[id] {
			skillData := map[string]interface{}{
				"id":      id,
				"hasTemp": true,
				"raw":     tempRaw,
			}
			skills = append(skills, skillData)
		}
	}

	return c.JSON(http.StatusOK, skills)
}

func getSkillDetail(c echo.Context) error {
	id := c.Param("id")

	if raw, ok := project.GetSkillNew(id); ok {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":      id,
			"raw":     raw,
			"hasTemp": true,
		})
	}

	s, exists := project.GetSkill(id)
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "skill not found"})
	}

	rawConfig, _ := common.GetRawConfig("skill", id)
	if rawConfig == "" {
		rawConfig = s.RawConfig
	}

	resp := map[string]interface{}{
		"id":      id,
		"raw":     rawConfig,
		"hasTemp": false,
		"status":  string(s.Status),
	}
	return c.JSON(http.StatusOK, resp)
}

func createSkill(c echo.Context) error {
	return createComponent("skill", c)
}

func updateSkill(c echo.Context) error {
	return updateComponent("skill", c)
}

func deleteSkillHandler(c echo.Context) error {
	return deleteComponent("skill", c)
}

func cancelSkillUpgrade(c echo.Context) error {
	id := c.Param("id")
	project.DeleteSkillNew(id)

	tempPath, tempExists := GetComponentPath("skill", id, true)
	if tempExists {
		_ = os.Remove(tempPath)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Skill upgrade cancelled for %s", id),
	})
}
