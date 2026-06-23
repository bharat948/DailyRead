package main

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func main() {
	client := anthropic.NewClient(
		option.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
	)

	sysBlock := anthropic.TextBlockParam{Text: "system prompt text"}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-haiku-20240307"),
		MaxTokens: 1024,
		System:    []anthropic.TextBlockParam{sysBlock},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
		},
	}

	fmt.Printf("%T\n", params)
	_ = client
	_ = context.Background()
}
