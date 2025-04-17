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
	Execute(sessionCtx context.Context, user, channel, threadTs, prompt string) error
}

// NewHandler returns handler for Slack events.
//
//   - ctx: The context representing the application's lifecycle.
//   - uc: The use-case for handling Slack messages and LLM interactions.
func NewHandler(
	uc UseCase,
) echo.HandlerFunc {
	return func(c echo.Context) error {
		event := c.Get("event").(slackevents.EventsAPIEvent)

		switch event.Type {
		case slackevents.URLVerification:
			var res *slackevents.ChallengeResponse
			body, err := io.ReadAll(c.Request().Body)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(body, &res); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError)
			}
			return c.String(http.StatusOK, res.Challenge)
		case slackevents.CallbackEvent:
			switch innerEvent := event.InnerEvent.Data.(type) {
			case *slackevents.AppMentionEvent:
				go func() {
					untypedSession, ok := sessions.Load(sessionKey(innerEvent.Channel, innerEvent.TimeStamp, innerEvent.User))
					if !ok {
						slog.Debug("missing session", slog.String("channel", innerEvent.Channel), slog.String("ts", innerEvent.TimeStamp), slog.String("user", innerEvent.User))
						return
					}
					session, _ := untypedSession.(*Session)
					errCh := make(chan error)

					user := session.user
					slog.Info("request received", slog.String("user_id", user.ID), slog.String("event", fmt.Sprintf("%+v", innerEvent)))

					prompt := promptFromMention(innerEvent)

					select {
					case errCh <- uc.Execute(session.ctx, innerEvent.User, innerEvent.Channel, innerEvent.TimeStamp, prompt):
						if err := <-errCh; err != nil {
							slog.Error("failed to execute", slog.String("error", err.Error()))
						}
						close(errCh)
						session.cancel()
						sessions.Delete(sessionKey(innerEvent.Channel, innerEvent.TimeStamp, innerEvent.User))
						return
					case <-session.ctx.Done():
						slog.Info("session done")
						if err := session.ctx.Err(); err != nil {
							slog.Error(err.Error())
						}
						close(errCh)
						sessions.Delete(sessionKey(innerEvent.Channel, innerEvent.TimeStamp, innerEvent.User))
						return
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
	prompt := event.Text[index+1:]
	if strings.TrimFunc(prompt, func(r rune) bool {
		return unicode.IsSpace(r)
	}) == "" {
		return ""
	}
	return event.Text[index+1:]
}
