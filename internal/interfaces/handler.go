package interfaces

import (
	"context"
	"fmt"
	"github.com/goccy/go-json"
	"github.com/labstack/echo/v4"
	"github.com/slack-go/slack/slackevents"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"unicode"
)

// UseCase represents the use-case for handling Slack messages and LLM interactions.
type UseCase interface {
	// Execute handles LLM interactions and Slack message updates.
	//
	// 	- sessionCtx: context representing the session for the operation.
	// 	- channel: The Slack channel ID where the message will be posted.
	// 	- threadTs: The timestamp of the thread to reply to.
	// 	- prompt: The prompt to send to the LLM.
	Execute(sessionCtx context.Context, channel, threadTs, prompt string) error
}

// Session represents a session for handling Slack events.
type Session struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func sessionKey(channel, threadTs, user string) string {
	return fmt.Sprintf("%s#%s#%s", channel, threadTs, user)
}

// newSession creates a new session.
func newSession(ctx context.Context) *Session {
	ctx, cancel := context.WithCancel(ctx)
	return &Session{
		ctx:    ctx,
		cancel: cancel,
	}
}

var sessions sync.Map

// NewHandler returns handler for Slack events.
//
//   - ctx: The context representing the application's lifecycle.
//   - uc: The use-case for handling Slack messages and LLM interactions.
func NewHandler(
	ctx context.Context,
	uc UseCase,
) echo.HandlerFunc {
	return func(c echo.Context) error {
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		event := c.Get("event").(slackevents.EventsAPIEvent)
		switch event.Type {
		case slackevents.URLVerification:
			var res *slackevents.ChallengeResponse
			if err := json.Unmarshal(body, &res); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError)
			}
			return c.String(http.StatusOK, res.Challenge)
		case slackevents.CallbackEvent:
			switch innerEvent := event.InnerEvent.Data.(type) {
			case *slackevents.AppMentionEvent:
				prompt := promptFromMention(innerEvent)
				if prompt == "" {
					return c.NoContent(http.StatusBadRequest)
				}
				if _, ok := sessions.Load(sessionKey(innerEvent.Channel, innerEvent.ThreadTimeStamp, innerEvent.User)); ok {
					return echo.NewHTTPError(http.StatusConflict)
				}
				session := newSession(ctx)
				sessions.Store(sessionKey(innerEvent.Channel, innerEvent.ThreadTimeStamp, innerEvent.User), session)
				errCh := make(chan error)
				go func() {
					defer close(errCh)
					defer sessions.Delete(sessionKey(innerEvent.Channel, innerEvent.ThreadTimeStamp, innerEvent.User))
					defer session.cancel()
					select {
					case errCh <- uc.Execute(session.ctx, innerEvent.Channel, innerEvent.TimeStamp, prompt):
						if err := <-errCh; err != nil {
							slog.Error("failed to execute", slog.String("error", err.Error()))
						}
					case <-session.ctx.Done():
						slog.Info("session done")
						if err := session.ctx.Err(); err != nil {
							slog.Error(err.Error())
						}
					}
				}()
				return c.NoContent(http.StatusAccepted)
			}
		}
		return nil
	}
}

// promptFromMention extracts the prompt from the app mention event text.
func promptFromMention(event *slackevents.AppMentionEvent) string {
	index := strings.IndexFunc(event.Text, func(r rune) bool {
		return unicode.IsSpace(r)
	})
	if index == -1 || index+1 >= len(event.Text) {
		return ""
	}
	return event.Text[index+1:]
}
