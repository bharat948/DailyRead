package llm

import (
	"context"
)

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
	RoleTool      MessageRole = "tool"
)

type Message struct {
	Role       MessageRole
	Content    string
	ToolCalls  []ToolCall
	ToolResult *ToolResult
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string // JSON string
}

type ToolResult struct {
	ToolCallID string
	Content    string
	IsError    bool
}

type Tool struct {
	Name        string
	Description string
	Schema      interface{} // JSON Schema struct representation
}

type ChatRequest struct {
	Model       string
	System      string
	Messages    []Message
	MaxTokens   int
	Temperature float64
}

type ChatResponse struct {
	Message    Message
	TokensIn   int
	TokensOut  int
	StopReason string
}

type StructuredRequest struct {
	Model       string
	System      string
	Messages    []Message
	MaxTokens   int
	Temperature float64
	Schema      interface{} // JSON schema struct for output
}

type LoopRequest struct {
	Model       string
	System      string
	Messages    []Message
	MaxTokens   int
	Temperature float64
}

// Client abstracts the underlying LLM provider (Anthropic, OpenAI)
type Client interface {
	// Chat performs a standard text generation or chat completion
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

	// Structured returns a validated structured output unmarshaled into dest
	Structured(ctx context.Context, req StructuredRequest, dest interface{}) error

	// ResearchLoop executes an agentic loop with tools up to maxRounds
	ResearchLoop(ctx context.Context, req LoopRequest, tools []Tool, maxRounds int, executor func(context.Context, string, string) (string, error)) (string, error)
}
