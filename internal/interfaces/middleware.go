package interfaces

import (
	"bytes"
	"errors"
	"github.com/labstack/echo/v4"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"io"
	"net/http"
)

// NewSecretVerify is a middleware that verifies the Slack signing secret for incoming requests.
func NewSecretVerify(slackSinginSecret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			verifier, err := slack.NewSecretsVerifier(c.Request().Header, slackSinginSecret)
			if err != nil {
				return c.NoContent(http.StatusInternalServerError)
			}
			teeReader := io.TeeReader(c.Request().Body, &verifier)
			body, err := io.ReadAll(teeReader)
			if err != nil {
				return c.NoContent(http.StatusInternalServerError)
			}
			if err := verifier.Ensure(); err != nil {
				return c.NoContent(http.StatusUnauthorized)
			}
			c.Request().Body = io.NopCloser(bytes.NewBuffer(body))
			return next(c)
		}
	}
}

// NewParseEvent is a middleware that parses the incoming Slack event and sets it in the context.
func NewParseEvent() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			var buf bytes.Buffer
			teeReader := io.TeeReader(c.Request().Body, &buf)
			body, err := io.ReadAll(teeReader)
			if err != nil {
				return c.NoContent(http.StatusInternalServerError)
			}
			c.Request().Body = io.NopCloser(bytes.NewBuffer(body))

			// Parse the request
			event, err := slackevents.ParseEvent(body, slackevents.OptionNoVerifyToken())
			if err != nil {
				return err
			}
			c.Set("event", event)
			return next(c)
		}
	}
}

// NewAuth is a middleware that checks if the user is allowed to access the endpoint.
func NewAuth(allowedUsers map[string]bool, client *slack.Client) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// not to be nil
			slackEvent := c.Get("event").(slackevents.EventsAPIEvent)
			if slackEvent.Type != slackevents.CallbackEvent {
				return next(c)
			}
			switch innerEvent := slackEvent.InnerEvent.Data.(type) {
			case *slackevents.AppMentionEvent:
				user, err := client.GetUserInfo(innerEvent.User)
				if err != nil || user == nil {
					return c.NoContent(http.StatusUnauthorized)
				}
				if user.IsBot || user.IsAppUser {
					return nil
				}
				if len(allowedUsers) == 0 {
					return next(c)
				}
				if !allowedUsers[user.ID] {
					return c.NoContent(http.StatusForbidden)
				}
			}
			return next(c)
		}
	}
}

// NewErrorHandler is a middleware that handles errors and sends a message to the Slack channel.
func NewErrorHandler(client *slack.Client) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		event := c.Get("event")
		if event == nil {
			return
		}

		var (
			channel  string
			threadTs string
		)
		switch slackEvent := event.(type) {
		case *slackevents.AppMentionEvent:
			channel = slackEvent.Channel
			threadTs = slackEvent.TimeStamp
		default:
			return
		}
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			switch httpErr.Code {
			case http.StatusUnauthorized:
				client.PostMessage(channel, slack.MsgOptionText("Unauthorized", false), slack.MsgOptionTS(threadTs))
				return
			case http.StatusForbidden:
				client.PostMessage(channel, slack.MsgOptionText("You are not authorized to perform this operation.\nPlease contact the bot administrator.", false), slack.MsgOptionTS(threadTs))
				return
			}
		}
		return
	}
}
