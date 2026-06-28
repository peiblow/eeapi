package schema

import (
	"github.com/peiblow/eeapi/internal/swp"
)

type AgentDefinition struct {
	Hash         string           `json:"hash"`          // contract_agents._hash
	Name         string           `json:"name"`          // contract_agents.name
	Version      string           `json:"version"`       // contract_agents.version
	Purpose      string           `json:"purpose"`       // contract_agents.purpose
	SystemPrompt string           `json:"system_prompt"` // contract_agents.system_prompt (.md embarcado)
	Model        swp.ModelStmt    `json:"model"`         // contract_agents.model (JSONB)
	Behavior     swp.BehaviorStmt `json:"behavior"`      // contract_agents.behavior (JSONB)
	Tools        []swp.ToolStmt   `json:"tools"`         // agent_tools
	Skills       []swp.SkillStmt  `json:"skills"`        // agent_skills
}
