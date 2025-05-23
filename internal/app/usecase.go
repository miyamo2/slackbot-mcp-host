package app

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"github.com/goccy/go-json"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcphost/pkg/history"
	"github.com/mark3labs/mcphost/pkg/llm"
	"github.com/pkg/errors"
	"github.com/slack-go/slack"
)

var (
	ErrEmptyPrompt = errors.New("empty prompt")
)

// SlackClient is an interface that defines the methods for posting and updating messages in Slack.
type SlackClient interface {
	PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error)
	UpdateMessageContext(ctx context.Context, channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error)
	DeleteMessageContext(ctx context.Context, channel, messageTimestamp string) (string, string, error)
}

// UseCase represents the use-case for handling Slack messages and LLM interactions.
type UseCase struct {
	timeoutNs   time.Duration
	slackClient SlackClient
	llmProvider llm.Provider
	tools       []llm.Tool
	mcpClients  map[string]client.MCPClient
}

// NewUseCase returns a new instance of UseCase.
func NewUseCase(
	timeoutNs time.Duration,
	slackClient SlackClient,
	llmProvider llm.Provider,
	tools []llm.Tool,
	mcpClients map[string]client.MCPClient,
) *UseCase {
	return &UseCase{
		timeoutNs:   timeoutNs,
		slackClient: slackClient,
		llmProvider: llmProvider,
		tools:       tools,
		mcpClients:  mcpClients,
	}
}

// Execute handles LLM interactions and Slack message updates.
//
//   - sessionCtx: context representing the session for the operation.
//   - user: The Slack user ID who mentions the bot.
//   - channel: The Slack channel ID where the message will be posted.
//   - threadTs: The timestamp of the thread to reply to.
//   - prompt: The prompt to send to the LLM.
func (u *UseCase) Execute(sessionCtx context.Context, user, channel, threadTs, prompt string) error {
	slog.Info("BEGIN UseCase.Execute", slog.String("channel", channel), slog.String("threadTs", threadTs), slog.String("prompt", prompt))
	defer slog.Info("END UseCase.Execute", slog.String("channel", channel))

	if prompt == "" {
		return ErrEmptyPrompt
	}
	messages := []history.HistoryMessage{
		{
			Role: "user",
			Content: []history.ContentBlock{{
				Type: "text",
				Text: prompt,
			}},
		},
	}
	return u.execute(sessionCtx, user, channel, threadTs, prompt, messages)
}

var durationForLLMRateLimitExceeded = time.Minute + 30*time.Second

// execute handles the LLM interactions and Slack message updates.
// this method is called recursively to handle tool results.
func (u *UseCase) execute(sessionCtx context.Context, user, channel, threadTs, prompt string, messages []history.HistoryMessage) error {
	slog.Info("BEGIN UseCase.execute", slog.String("channel", channel), slog.String("threadTs", threadTs), slog.String("prompt", prompt))
	defer slog.Info("END UseCase.execute", slog.String("channel", channel))
	messageID, err := u.postMessage(sessionCtx, user, channel, "⌛ Thinking...", threadTs)
	if err != nil {
		return err
	}
	// Convert MessageParam to llm.Message for provider
	// Messages already implement llm.Message interface
	llmMessages := make([]llm.Message, 0, len(messages))
	for i := range messages {
		llmMessages = append(llmMessages, &(messages)[i])
	}

	var message llm.Message
	err = retry.Do(
		func() error {
			ctx, cancel := context.WithTimeout(sessionCtx, u.timeoutNs)
			defer cancel()
			message, err = u.llmProvider.CreateMessage(
				ctx,
				prompt,
				llmMessages,
				u.tools,
			)
			return err
		},
		retry.Attempts(5),
		retry.DelayType(retry.BackOffDelay),
		retry.RetryIf(func(err error) bool {
			return strings.Contains(err.Error(), "overloaded_error")
		}),
		retry.DelayType(func(n uint, err error, config *retry.Config) time.Duration {
			duration := retry.BackOffDelay(n, err, config)
			if err != nil && strings.Contains(err.Error(), "rate_limit_error") {
				messageID, _ = u.updateMessage(sessionCtx, user, channel, messageID, fmt.Sprintf("⌛ Rate limit exceeded. waiting for %d nanoseconds...", duration))
				if duration < durationForLLMRateLimitExceeded {
					return durationForLLMRateLimitExceeded
				}
			}
			return duration
		}),
	)
	if err != nil {
		slog.Error("failed to create message", slog.String("error", err.Error()))
		u.updateMessage(sessionCtx, user, channel, messageID, "😵‍💫‍")
		return err
	}

	var (
		messageContents []history.ContentBlock
		toolResults     []history.ContentBlock
	)

	// Add text content
	if message.GetContent() != "" {
		messageID, err = u.updateMessage(sessionCtx, user, channel, messageID, message.GetContent())
		messageContents = append(messageContents, history.ContentBlock{
			Type: "text",
			Text: message.GetContent(),
		})
	} else {
		// If the content is empty, delete the temporary message
		u.slackClient.DeleteMessageContext(sessionCtx, channel, messageID)
	}

	// Handle tool calls
	for _, toolCall := range message.GetToolCalls() {
		messageContent, toolResult := u.handleToolCall(sessionCtx, toolCall, message)
		if len(messageContent) > 0 {
			messageContents = slices.Concat(messageContents, messageContent)
		}
		if len(toolResult) > 0 {
			toolResults = slices.Concat(toolResults, toolResult)
		}
	}

	messages = append(messages, history.HistoryMessage{
		Role:    message.GetRole(),
		Content: messageContents,
	})
	if len(toolResults) > 0 {
		for _, toolResult := range toolResults {
			messages = append(messages, history.HistoryMessage{
				Role:    "tool",
				Content: []history.ContentBlock{toolResult},
			})
		}
		// Make another call to get Claude's response to the tool results
		return u.execute(sessionCtx, user, channel, threadTs, "", messages)
	}
	return nil
}

// postMessage posts a message to the Slack channel and returns the message ID.
func (u *UseCase) postMessage(ctx context.Context, user, channel, message, threadTs string) (string, error) {
	slog.Info("BEGIN UseCase.postMessage", slog.String("channel", channel), slog.String("message", message), slog.String("threadTs", threadTs))
	defer slog.Info("END UseCase.postMessage", slog.String("channel", channel))
	ctx, cancel := context.WithTimeout(ctx, u.timeoutNs)
	defer cancel()
	_, v, err := u.slackClient.PostMessageContext(
		ctx,
		channel,
		slack.MsgOptionText(fmt.Sprintf("<@%s> \n%s", user, message), false),
		slack.MsgOptionTS(threadTs))
	return v, err
}

// updateMessage updates a message in the Slack channel and returns the message ID.
func (u *UseCase) updateMessage(ctx context.Context, user, channel, messageID, message string) (string, error) {
	slog.Info("BEGIN UseCase.updateMessage", slog.String("channel", channel), slog.String("messageID", messageID), slog.String("message", message))
	defer slog.Info("END UseCase.updateMessage", slog.String("channel", channel))
	ctx, cancel := context.WithTimeout(ctx, u.timeoutNs)
	defer cancel()
	_, v, _, err := u.slackClient.UpdateMessageContext(
		ctx,
		channel,
		messageID,
		slack.MsgOptionText(fmt.Sprintf("<@%s> \n%s", user, message), false))
	return v, err
}

// handleToolCall handles the tool call and returns the message content and tool results.
func (u *UseCase) handleToolCall(sessionCtx context.Context, toolCall llm.ToolCall, message llm.Message) (messageContent []history.ContentBlock, toolResults []history.ContentBlock) {
	slog.Info("Using tool", slog.String("tool_name", toolCall.GetName()))

	input, err := json.Marshal(toolCall.GetArguments())
	if err != nil {
		slog.Warn("failed to marshal tool arguments", slog.String("error", err.Error()))
		return
	}
	messageContent = append(messageContent, history.ContentBlock{
		Type:  "tool_use",
		ID:    toolCall.GetID(),
		Name:  toolCall.GetName(),
		Input: input,
	})

	// Log usage statistics if available
	inputTokens, outputTokens := message.GetUsage()
	if inputTokens > 0 || outputTokens > 0 {
		slog.Info("Usage statistics",
			slog.Int("input_tokens", inputTokens),
			slog.Int("output_tokens", outputTokens),
			slog.Int("total_tokens", inputTokens+outputTokens))
	}

	parts := strings.Split(toolCall.GetName(), "__")
	if len(parts) != 2 {
		slog.Warn("invalid tool name format", slog.String("tool_name", toolCall.GetName()))
		return
	}

	serverName, toolName := parts[0], parts[1]
	mcpClient, ok := u.mcpClients[serverName]
	if !ok {
		slog.Warn("server not found", slog.String("server_name", serverName))
		return
	}

	var toolArgs map[string]any
	if err := json.Unmarshal(input, &toolArgs); err != nil {
		slog.Warn("failed to unmarshal tool arguments", slog.String("error", err.Error()))
		return
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = toolName
	req.Params.Arguments = toolArgs

	toolResult, err := func() (*mcp.CallToolResult, error) {
		ctx, cancel := context.WithTimeout(sessionCtx, u.timeoutNs)
		defer cancel()
		return mcpClient.CallTool(
			ctx,
			req,
		)
	}()

	if err != nil {
		errMsg := fmt.Sprintf(
			"Error calling tool %s: %v",
			toolName,
			err,
		)
		// Add error message as tool result
		toolResults = append(toolResults, history.ContentBlock{
			Type:      "tool_result",
			ToolUseID: toolCall.GetID(),
			Content: []history.ContentBlock{{
				Type: "text",
				Text: errMsg,
			}},
		})
		return
	}

	if len(toolResult.Content) != 0 {
		// Create the tool result block
		resultBlock := history.ContentBlock{
			Type:      "tool_result",
			ToolUseID: toolCall.GetID(),
			Content:   toolResult.Content,
		}

		// Extract text content
		var resultText string
		// Handle array content directly since we know it's []interface{}
		for _, item := range toolResult.Content {
			if contentMap, ok := any(item).(map[string]any); ok {
				if text, ok := contentMap["text"]; ok {
					resultText = fmt.Sprintf("%s%v ", resultText, text)
				}
			}
		}
		resultBlock.Text = strings.TrimSpace(resultText)
		toolResults = append(toolResults, resultBlock)
	}
	return
}
