package llm

import (
	"fmt"
	"strings"

	"dailyread/internal/config"
)

type Role string

const (
	RolePlanner   Role = "planner"
	RoleResearch  Role = "research"
	RoleTriage    Role = "triage"
	RoleSummarize Role = "summarize"
	RoleCurate    Role = "curate"
	RoleCompact   Role = "compact"
)

type Router struct {
	cfg             config.ModelsConfig
	anthropicClient *AnthropicClient
	openaiClient    *OpenAIClient
}

func NewRouter(cfg config.ModelsConfig) *Router {
	return &Router{
		cfg:             cfg,
		anthropicClient: NewAnthropicClient(),
		openaiClient:    NewOpenAIClient(),
	}
}

// ClientFor returns the appropriate client instance and the specific model string to use for that role
func (r *Router) ClientFor(role Role) (Client, string, error) {
	provider := strings.ToLower(r.cfg.Provider)
	if provider == "" {
		provider = "anthropic" // Default fallback
	}

	var model string
	switch role {
	case RoleResearch, RoleCurate:
		model = r.cfg.Research
		if model == "" {
			if provider == "openai" {
				model = "o3-mini"
			} else {
				model = "claude-opus-4-8" // Fallback
			}
		}
	default: // Triage, Planner, Summarize, Compact
		model = r.cfg.Triage
		if model == "" {
			if provider == "openai" {
				model = "gpt-4o-mini"
			} else {
				model = "claude-haiku-4-5" // Fallback
			}
		}
	}

	switch provider {
	case "anthropic":
		return r.anthropicClient, model, nil
	case "openai":
		return r.openaiClient, model, nil
	default:
		return nil, "", fmt.Errorf("unsupported LLM provider: %s", provider)
	}
}
