package curriculum

import (
	"fmt"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
)

type PrerequisiteGraph map[common.ID][]common.ID

func (g PrerequisiteGraph) Validate() error {
	visiting := make(map[common.ID]bool, len(g))
	visited := make(map[common.ID]bool, len(g))
	var visit func(common.ID, []common.ID) error
	visit = func(node common.ID, path []common.ID) error {
		if visiting[node] {
			return fmt.Errorf("prerequisite cycle %v -> %s: %w", path, node, common.ErrConflict)
		}
		if visited[node] {
			return nil
		}
		visiting[node] = true
		path = append(path, node)
		for _, prerequisite := range g[node] {
			if !prerequisite.Valid() {
				return common.FieldError{Field: "prerequisite", Message: "contains an invalid id"}
			}
			if _, exists := g[prerequisite]; !exists {
				return fmt.Errorf("prerequisite %s is missing from graph: %w", prerequisite, common.ErrNotFound)
			}
			if err := visit(prerequisite, path); err != nil {
				return err
			}
		}
		visiting[node] = false
		visited[node] = true
		return nil
	}
	for node := range g {
		if err := visit(node, nil); err != nil {
			return err
		}
	}
	return nil
}

func (g PrerequisiteGraph) Missing(completed map[common.ID]bool, target common.ID) []common.ID {
	missing := make([]common.ID, 0)
	seen := make(map[common.ID]bool)
	var walk func(common.ID)
	walk = func(node common.ID) {
		for _, prerequisite := range g[node] {
			if seen[prerequisite] {
				continue
			}
			seen[prerequisite] = true
			if !completed[prerequisite] {
				missing = append(missing, prerequisite)
			}
			walk(prerequisite)
		}
	}
	walk(target)
	return missing
}
