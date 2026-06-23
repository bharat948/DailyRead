package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type AnthropicClient struct {
	client *anthropic.Client
}

func NewAnthropicClient() *AnthropicClient {
	cli := anthropic.NewClient(
		option.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
	)
	return &AnthropicClient{
		client: &cli,
	}
}

func (c *AnthropicClient) toAnthropicMessages(messages []Message) []anthropic.MessageParam {
	var params []anthropic.MessageParam
	for _, m := range messages {
		switch m.Role {
		case RoleUser:
			params = append(params, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		case RoleAssistant:
			var blocks []anthropic.ContentBlockParamUnion
			if m.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Content))
			}
			for _, tc := range m.ToolCalls {
				var input map[string]interface{}
				_ = json.Unmarshal([]byte(tc.Arguments), &input)
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, input, tc.Name))
			}
			params = append(params, anthropic.NewAssistantMessage(blocks...))
		case RoleTool:
			if m.ToolResult != nil {
				blocks := []anthropic.ContentBlockParamUnion{
					anthropic.NewToolResultBlock(m.ToolResult.ToolCallID, m.ToolResult.Content, m.ToolResult.IsError),
				}
				params = append(params, anthropic.NewUserMessage(blocks...))
			}
		}
	}
	return params
}

func (c *AnthropicClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: int64(req.MaxTokens),
		Messages:  c.toAnthropicMessages(req.Messages),
	}

	if req.System != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.System},
		}
	}

	res, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return nil, err
	}

	return &ChatResponse{
		Message: Message{
			Role:    RoleAssistant,
			Content: res.Content[0].Text,
		},
		TokensIn:   int(res.Usage.InputTokens),
		TokensOut:  int(res.Usage.OutputTokens),
		StopReason: string(res.StopReason),
	}, nil
}

func (c *AnthropicClient) Structured(ctx context.Context, req StructuredRequest, dest interface{}) error {
	var schemaParam anthropic.ToolInputSchemaParam
	bytes, _ := json.Marshal(req.Schema)
	_ = json.Unmarshal(bytes, &schemaParam)

	toolName := "return_structured_data"

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: int64(req.MaxTokens),
		Messages:  c.toAnthropicMessages(req.Messages),
		Tools:     []anthropic.ToolUnionParam{anthropic.ToolUnionParamOfTool(schemaParam, toolName)},
		ToolChoice: anthropic.ToolChoiceParamOfTool(toolName),
	}

	if req.System != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.System},
		}
	}

	res, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return err
	}

	for _, block := range res.Content {
		if string(block.Type) == "tool_use" {
			bytes, _ := json.Marshal(block.Input)
			return json.Unmarshal(bytes, dest)
		}
	}

	return fmt.Errorf("model did not return tool use block")
}

func (c *AnthropicClient) ResearchLoop(ctx context.Context, req LoopRequest, tools []Tool, maxRounds int, executor func(context.Context, string, string) (string, error)) (string, error) {
	var antTools []anthropic.ToolUnionParam
	for _, t := range tools {
		var schemaParam anthropic.ToolInputSchemaParam
		bytes, _ := json.Marshal(t.Schema)
		_ = json.Unmarshal(bytes, &schemaParam)

		antTools = append(antTools, anthropic.ToolUnionParamOfTool(schemaParam, t.Name))
	}

	messages := c.toAnthropicMessages(req.Messages)
	
	var finalResponse string

	for round := 0; round < maxRounds; round++ {
		params := anthropic.MessageNewParams{
			Model:     anthropic.Model(req.Model),
			MaxTokens: int64(req.MaxTokens),
			Messages:  messages,
			Tools:     antTools,
		}

		if req.System != "" {
			params.System = []anthropic.TextBlockParam{
				{Text: req.System},
			}
		}

		res, err := c.client.Messages.New(ctx, params)
		if err != nil {
			return "", fmt.Errorf("API error: %w", err)
		}

		messages = append(messages, res.ToParam())

		if res.StopReason != anthropic.StopReasonToolUse {
			if len(res.Content) > 0 {
				finalResponse = res.Content[0].Text
			}
			break
		}

		var toolResults []anthropic.ContentBlockParamUnion
		for _, block := range res.Content {
			if string(block.Type) == "tool_use" {
				argsBytes, _ := json.Marshal(block.Input)
				slog.Info("Executing tool", "tool", block.Name)
				result, err := executor(ctx, block.Name, string(argsBytes))
				isErr := err != nil
				if isErr {
					result = err.Error()
				}
				toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, result, isErr))
			}
		}

		if len(toolResults) > 0 {
			messages = append(messages, anthropic.NewUserMessage(toolResults...))
		}
	}

	return finalResponse, nil
}
