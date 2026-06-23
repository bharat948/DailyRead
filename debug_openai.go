package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

type OpenAIClient struct {
	client *openai.Client
}

func NewOpenAIClient() *OpenAIClient {
	cli := openai.NewClient(
		option.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
	)
	return &OpenAIClient{
		client: cli,
	}
}

func (c *OpenAIClient) toOpenAIMessages(system string, messages []Message) []openai.ChatCompletionMessageParamUnion {
	var params []openai.ChatCompletionMessageParamUnion

	if system != "" {
		params = append(params, openai.SystemMessage(system))
	}

	for _, m := range messages {
		switch m.Role {
		case RoleUser:
			params = append(params, openai.UserMessage(m.Content))
		case RoleAssistant:
			var toolCalls []openai.ChatCompletionMessageToolCallParam
			for _, tc := range m.ToolCalls {
				toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
					ID: tc.ID,
					Function: openai.ChatCompletionMessageToolCallFunctionParam{
						Name:      openai.String(tc.Name),
						Arguments: openai.String(tc.Arguments),
					},
				})
			}
			msg := openai.ChatCompletionAssistantMessageParam{}
			if m.Content != "" {
				msg.Content = openai.String(m.Content)
			}
			if len(toolCalls) > 0 {
				msg.ToolCalls = openai.ChatCompletionMessageToolCalls(toolCalls)
			}
			params = append(params, msg)
		case RoleTool:
			if m.ToolResult != nil {
				params = append(params, openai.ToolMessage(m.ToolResult.ToolCallID, m.ToolResult.Content))
			}
		}
	}
	return params
}

func (c *OpenAIClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	params := openai.ChatCompletionNewParams{
		Messages: c.toOpenAIMessages(req.System, req.Messages),
		Model:    openai.ChatModel(req.Model),
	}

	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(req.MaxTokens))
	}

	res, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, err
	}

	return &ChatResponse{
		Message: Message{
			Role:    RoleAssistant,
			Content: res.Choices[0].Message.Content,
		},
		TokensIn:   int(res.Usage.PromptTokens),
		TokensOut:  int(res.Usage.CompletionTokens),
		StopReason: string(res.Choices[0].FinishReason),
	}, nil
}

func (c *OpenAIClient) Structured(ctx context.Context, req StructuredRequest, dest interface{}) error {
	schemaBytes, err := json.Marshal(req.Schema)
	if err != nil {
		return err
	}

	params := openai.ChatCompletionNewParams{
		Messages: c.toOpenAIMessages(req.System, req.Messages),
		Model:    openai.ChatModel(req.Model),
		ResponseFormat: openai.ResponseFormatJSONSchemaParam{
			JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:        openai.String("structured_output"),
				Description: openai.String("Structured output schema"),
				Schema:      openai.F(schemaBytes), // Wait, does this exist? I will use raw bytes/struct.
				Strict:      openai.Bool(true),
			},
		},
	}

	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(req.MaxTokens))
	}

	res, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(res.Choices[0].Message.Content), dest)
}

func (c *OpenAIClient) ResearchLoop(ctx context.Context, req LoopRequest, tools []Tool, maxRounds int, executor func(context.Context, string, string) (string, error)) (string, error) {
	var oTools []openai.ChatCompletionToolParam
	for _, t := range tools {
		schemaBytes, _ := json.Marshal(t.Schema)
		var param map[string]interface{}
		_ = json.Unmarshal(schemaBytes, &param)

		oTools = append(oTools, openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        openai.String(t.Name),
				Description: openai.String(t.Description),
				Parameters:  shared.FunctionParameters(param),
			},
		})
	}

	messages := c.toOpenAIMessages(req.System, req.Messages)
	
	var finalResponse string

	for round := 0; round < maxRounds; round++ {
		params := openai.ChatCompletionNewParams{
			Messages: messages,
			Model:    openai.ChatModel(req.Model),
			Tools:    openai.F(oTools),
		}

		if req.MaxTokens > 0 {
			params.MaxCompletionTokens = openai.Int(int64(req.MaxTokens))
		}

		if req.Model == "o1" || req.Model == "o3-mini" {
			params.ReasoningEffort = shared.ReasoningEffortHigh
		}

		res, err := c.client.Chat.Completions.New(ctx, params)
		if err != nil {
			return "", fmt.Errorf("API error: %w", err)
		}

		choice := res.Choices[0]
		
		msgParam := openai.ChatCompletionAssistantMessageParam{}
		if choice.Message.Content != "" {
			msgParam.Content = openai.String(choice.Message.Content)
		}
		if len(choice.Message.ToolCalls) > 0 {
			var tcParams []openai.ChatCompletionMessageToolCallParam
			for _, tc := range choice.Message.ToolCalls {
				tcParams = append(tcParams, tc.ToParam())
			}
			msgParam.ToolCalls = openai.ChatCompletionMessageToolCalls(tcParams)
		}
		messages = append(messages, msgParam)

		if choice.FinishReason != openai.ChatCompletionChoicesFinishReasonToolCalls {
			finalResponse = choice.Message.Content
			break
		}

		for _, tc := range choice.Message.ToolCalls {
			slog.Info("Executing tool", "tool", tc.Function.Name)
			result, err := executor(ctx, tc.Function.Name, tc.Function.Arguments)
			if err != nil {
				result = err.Error()
			}
			messages = append(messages, openai.ToolMessage(tc.ID, result))
		}
	}

	return finalResponse, nil
}
