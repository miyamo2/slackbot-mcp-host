package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcphost/pkg/llm"
	"github.com/mark3labs/mcphost/pkg/llm/anthropic"
	"github.com/mark3labs/mcphost/pkg/llm/google"
	"github.com/mark3labs/mcphost/pkg/llm/openai"
	"github.com/miyamo2/slackbot-mcp-host/internal/app"
	"github.com/miyamo2/slackbot-mcp-host/internal/interfaces"
	"github.com/miyamo2/slackbot-mcp-host/internal/log"
	"github.com/pkg/errors"
	"github.com/slack-go/slack"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"time"
)

//go:embed config.json
var config string

type Config struct {
	MCPServers       map[string]MCPServerConfig `json:"mcpServers"`
	TimeoutNs        int64                      `json:"timeoutNs"`
	LLMProviderName  string                     `json:"llmProviderName"`
	LLMApiKey        string                     `json:"llmApiKey"`
	LLMBaseURL       string                     `json:"llmBaseUrl"`
	LLMModelName     string                     `json:"llmModelName"`
	SlackBotToken    string                     `json:"slackBotToken"`
	SackSinginSecret string                     `json:"slackSigninSecret"`
	AllowedUsers     []string                   `json:"allowedUsers"`
	Port             int                        `json:"port"`
	GCPProjectId     string                     `json:"gcpProjectId"`
	RateLimit        RateLimitConfig            `json:"rateLimit"`
}

type MCPServerConfig struct {
	Command string         `json:"command"`
	Args    []string       `json:"args"`
	Env     map[string]any `json:"env"`
}

type RateLimitConfig struct {
	Enable    bool    `json:"enable"`
	Limit     float64 `json:"limit"`
	Burst     int     `json:"burst"`
	ExpressIn int64   `json:"expiresIn"`
}

func main() {
	// Parse the config
	var cfg Config
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		slog.Error("failed to parse config", slog.String("error", err.Error()))
		os.Exit(1)
	}
	slog.SetDefault(slog.New(log.NewHandler(cfg.GCPProjectId)))

	duration := time.Duration(cfg.TimeoutNs)
	if duration == 0 {
		// Set default timeout
		duration = 10 * time.Second
	}
	if cfg.Port == 0 {
		// Set default port
		cfg.Port = 8080
	}

	clients, closer, err := func() (map[string]client.MCPClient, func() error, error) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		return mcpClientFromConfig(ctx, cfg)
	}()
	if err != nil {
		slog.Error("failed to create mcp clients", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer closer()

	var allTools []llm.Tool
	for name, client := range clients {
		ctx, cancel := context.WithCancel(context.Background())
		tools, err := func() ([]llm.Tool, error) {
			defer cancel()
			return llmToolsFromMCPClient(ctx, client, name)
		}()
		if err != nil {
			slog.Error("failed to list tools", slog.String("error", err.Error()))
			continue
		}
		allTools = slices.Concat(allTools, tools)
		slog.Info("added tools from mcp client", slog.String("server", name), slog.Any("tools", tools))
	}

	llmProvider, err := func() (llm.Provider, error) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		return llmProviderFromConfig(ctx, cfg)
	}()
	if err != nil {
		slog.Error("failed to create llm provider", slog.String("error", err.Error()))
		os.Exit(1)
	}

	bot := slack.New(cfg.SlackBotToken)
	if _, err := bot.AuthTest(); err != nil {
		slog.Error("failed to authenticate bot token", slog.String("error", err.Error()))
		os.Exit(1)
	}

	alowedUsers := make(map[string]bool)
	for _, user := range cfg.AllowedUsers {
		alowedUsers[user] = true
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	e := echo.New()
	e.GET("/health", func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	})

	middlewares := []echo.MiddlewareFunc{
		interfaces.NewSecretVerify(cfg.SackSinginSecret),
		interfaces.NewParseEvent(),
		interfaces.NewAuth(alowedUsers, bot),
		interfaces.NewSessionMiddleware(ctx),
	}
	if cfg.RateLimit.Enable {
		middlewares = append(middlewares,
			interfaces.NewRateLimiter(
				cfg.RateLimit.Limit, cfg.RateLimit.Burst, time.Duration(cfg.RateLimit.ExpressIn)*time.Second))
	}
	e.POST("/slack/events",
		interfaces.NewHandler(app.NewUseCase(duration, bot, llmProvider, allTools, clients)),
		middlewares...)
	e.HTTPErrorHandler = interfaces.NewErrorHandler(bot)

	errChan := make(chan error, 1)
	go func() {
		slog.Info("start server.", slog.Int("port", cfg.Port))
		if err := e.Start(fmt.Sprintf(":%d", cfg.Port)); err != nil {
			errChan <- err
			return
		}
	}()

	select {
	case err := <-errChan:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error(err.Error())
		}
	case <-ctx.Done():
		if err := ctx.Err(); err != nil {
			slog.Error(err.Error())
		}
		slog.Info("stopping  server...")
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		defer cancel()
		e.Shutdown(ctx)
	}
}

// mcpClientFromConfig creates MCP clients from the given configuration.
func mcpClientFromConfig(rootCtx context.Context, conf Config) (map[string]client.MCPClient, func() error, error) {
	clients := make(map[string]client.MCPClient)
	for name, server := range conf.MCPServers {
		slog.InfoContext(context.TODO(), "create mcp client", slog.String("name", name))
		var (
			c   client.MCPClient
			err error
		)
		switch server.Command {
		case "sse_server":
			if len(server.Args) < 1 {
				return nil, nil, errors.New("missing argument")
			}
			host := server.Args[0]
			var headers map[string]string
			for k, v := range server.Env {
				headers[k] = fmt.Sprintf("%v", v)
			}
			c, err = client.NewSSEMCPClient(host, client.WithHeaders(headers))
		default:
			env := make([]string, 0, len(server.Env))
			for k, v := range server.Env {
				env = append(env, fmt.Sprintf("%s=%s", k, v))
			}
			c, err = client.NewStdioMCPClient(server.Command, env)
		}
		if err != nil {
			slog.ErrorContext(
				context.TODO(),
				`failed to create mcp client`,
				slog.String("name", name),
				slog.String("command", server.Command),
				slog.Any("args", server.Args),
				slog.Any("env", server.Env),
				slog.String("error", err.Error()))
			os.Exit(1)
		}
		slog.InfoContext(context.TODO(), "created mcp client",
			slog.String("name", name),
			slog.String("command", server.Command),
			slog.Any("args", server.Args),
			slog.Any("env", server.Env))
		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    "mcphost",
			Version: "0.1.0",
		}
		initRequest.Params.Capabilities = mcp.ClientCapabilities{}

		slog.InfoContext(context.TODO(), "initialize mcp client", slog.String("name", name))
		ctx, cancel := context.WithTimeout(rootCtx, 1*time.Minute)
		_, err = c.Initialize(ctx, initRequest)
		if err != nil {
			c.Close()
			for _, v := range clients {
				v.Close()
			}
			return nil, nil, fmt.Errorf("failed to initialize mcp client: %w", err)
		}
		cancel()
		clients[name] = c
		slog.InfoContext(context.TODO(), "mcp client initialized",
			slog.String("name", name),
			slog.String("command", server.Command),
			slog.Any("args", server.Args),
			slog.Any("env", server.Env))
	}
	closer := func() error {
		for name, client := range clients {
			if err := client.Close(); err != nil {
				return errors.Wrap(err, fmt.Sprintf("failed to close mcp client: %s", name))
			}
		}
		return nil
	}
	return clients, closer, nil
}

// llmToolsFromMCPClient converts mcp.Tool to llm.Tool
func llmToolsFromMCPClient(ctx context.Context, mcpClient client.MCPClient, mcpServerName string) ([]llm.Tool, error) {
	toolsResult, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to list tools: %s", mcpServerName))
	}
	llmTools := make([]llm.Tool, 0, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		namespacedName := fmt.Sprintf("%s__%s", mcpServerName, tool.Name)
		llmTools = append(llmTools, llm.Tool{
			Name:        namespacedName,
			Description: tool.Description,
			InputSchema: llm.Schema{
				Type:       tool.InputSchema.Type,
				Properties: tool.InputSchema.Properties,
				Required:   tool.InputSchema.Required,
			},
		})
	}
	return llmTools, nil
}

const (
	llmProviderAnthropic = "anthropic"
	llmProviderOpenAI    = "openai"
	llmProviderGoogle    = "google"
)

// llmProviderFromConfig creates an LLM provider from the given configuration.
func llmProviderFromConfig(ctx context.Context, cfg Config) (llm.Provider, error) {
	switch cfg.LLMProviderName {
	case llmProviderAnthropic:
		return anthropic.NewProvider(cfg.LLMApiKey, cfg.LLMBaseURL, cfg.LLMModelName), nil
	case llmProviderOpenAI:
		return openai.NewProvider(cfg.LLMApiKey, cfg.LLMBaseURL, cfg.LLMModelName), nil
	case llmProviderGoogle:
		return google.NewProvider(ctx, cfg.LLMApiKey, cfg.LLMModelName)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", cfg.LLMProviderName)
	}
}
