package api

import (
	"AgentSmith-HUB/memory"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
)

type memoryListItem struct {
	AgentID             string   `json:"agent_id,omitempty"`
	ProjectID           string   `json:"project_id,omitempty"`
	ProjectNodeSequence string   `json:"project_node_sequence"`
	InputIDs            []string `json:"input_ids,omitempty"`
	UpdatedAt           string   `json:"updated_at,omitempty"`
	Version             int      `json:"version"`
	SummaryCount        int      `json:"summary_count"`
	RecentRevertCount   int      `json:"recent_revert_count"`
	Path                string   `json:"path"`
}

func getMemoryList(c echo.Context) error {
	dir := memory.PNSMemoryDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusOK, []memoryListItem{})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	items := make([]memoryListItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		cfg, err := memory.ParseConfig(string(raw))
		if err != nil || cfg == nil || strings.TrimSpace(cfg.Scope.ProjectNodeSequence) == "" {
			continue
		}
		items = append(items, memoryListItem{
			AgentID:             cfg.Scope.AgentID,
			ProjectID:           cfg.Scope.ProjectID,
			ProjectNodeSequence: cfg.Scope.ProjectNodeSequence,
			InputIDs:            cfg.Scope.InputIDs,
			UpdatedAt:           cfg.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Version:             cfg.Version,
			SummaryCount:        len(cfg.Summaries),
			RecentRevertCount:   len(cfg.RecentReverts),
			Path:                filepath.Join(dir, entry.Name()),
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].ProjectNodeSequence < items[j].ProjectNodeSequence
	})

	return c.JSON(http.StatusOK, items)
}

func getMemoryDetail(c echo.Context) error {
	pns := strings.TrimSpace(c.Param("pns"))
	if pns == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "pns is required"})
	}
	cfg, err := memory.LoadPNSMemory(pns)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if cfg == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "memory not found"})
	}
	raw, err := memory.MarshalConfig(cfg)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"scope": cfg.Scope,
		"data":  cfg,
		"raw":   raw,
		"path":  memory.PNSMemoryPath(cfg.Scope.ProjectNodeSequence),
	})
}
