package interfaces

import (
	"bytes"
	"context"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"golang.org/x/time/rate"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// NewSecretVerify is a middleware that verifies the Slack signing secret for incoming requests.
func NewSecretVerify(slackSinginSecret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			slog.InfoContext(c.Request().Context(), "Begin verifying slack signing secret")
			defer slog.InfoContext(c.Request().Context(), "End verifying slack signing secret")
			verifier, err := slack.NewSecretsVerifier(c.Request().Header, slackSinginSecret)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError)
			}
			teeReader := io.TeeReader(c.Request().Body, &verifier)
			body, err := io.ReadAll(teeReader)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError)
			}
			if err := verifier.Ensure(); err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized)
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
			slog.InfoContext(c.Request().Context(), "Begin parsing slack event")
			defer slog.InfoContext(c.Request().Context(), "End parsing slack event")
			var buf bytes.Buffer
			teeReader := io.TeeReader(c.Request().Body, &buf)
			body, err := io.ReadAll(teeReader)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError)
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
	var userCache sync.Map
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			slog.InfoContext(c.Request().Context(), "Begin auth middleware")
			defer slog.InfoContext(c.Request().Context(), "End auth middleware")
			innerEvent, ok := appMentionEventFromContext(c)
			if !ok {
				return next(c)
			}

			var user *slack.User
			if u, ok := userCache.Load(innerEvent.User); ok {
				switch u := u.(type) {
				case *slack.User:
					user = u
				}
			}
			if user == nil {
				var err error
				user, err = client.GetUserInfo(innerEvent.User)
				if err != nil || user == nil {
					return echo.NewHTTPError(http.StatusUnauthorized)
				}
				userCache.Store(innerEvent.User, user)
			}
			if user.IsBot || user.IsAppUser {
				return nil
			}
			c.Set("user", user)
			if len(allowedUsers) == 0 {
				return next(c)
			}
			if !allowedUsers[user.ID] {
				return echo.NewHTTPError(http.StatusForbidden)
			}
			return next(c)
		}
	}
}

// headerXSlackNoRetry is a header that indicates that the request should not be retried.
const headerXSlackNoRetry = "X-Slack-No-Retry"

// NewErrorHandler is a middleware that handles errors and sends a message to the Slack channel.
func NewErrorHandler(client *slack.Client) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		slog.InfoContext(c.Request().Context(), "Begin error handler")
		defer slog.InfoContext(c.Request().Context(), "End error handler")
		if err == nil {
			return
		}
		innerEvent, ok := appMentionEventFromContext(c)
		if !ok {
			return
		}
		if c.Response().Committed {
			return
		}
		switch err := err.(type) {
		case *echo.HTTPError:
			slog.WarnContext(c.Request().Context(), "occurred *echo.HTTPError", slog.String("error", err.Error()))
			switch err.Code {
			case http.StatusUnauthorized:
				client.PostMessageContext(
					c.Request().Context(),
					innerEvent.Channel,
					slack.MsgOptionTS(innerEvent.TimeStamp),
					slack.MsgOptionText(fmt.Sprintf("<@%s> \n🫵 Unauthorized", innerEvent.User), false))
			case http.StatusForbidden:
				client.PostMessageContext(
					c.Request().Context(),
					innerEvent.Channel,
					slack.MsgOptionTS(innerEvent.TimeStamp),
					slack.MsgOptionText(
						fmt.Sprintf("<@%s> \n⛔ You are not allowed to perform this operation. Please contact the bot administrator.", innerEvent.User),
						false))
			case http.StatusTooManyRequests:
				client.PostMessageContext(
					c.Request().Context(),
					innerEvent.Channel,
					slack.MsgOptionTS(innerEvent.TimeStamp),
					slack.MsgOptionText(fmt.Sprintf("<@%s> \n🙌 You have reached your rate limit. Please try again later.", innerEvent.User), false))
			default:
				client.PostMessageContext(
					c.Request().Context(),
					innerEvent.Channel,
					slack.MsgOptionTS(innerEvent.TimeStamp),
					slack.MsgOptionText(fmt.Sprintf("<@%s> \n⚠️ Occured unexpected error", innerEvent.User), false))
			}

			c.Response().Header().Set(headerXSlackNoRetry, "1")
			c.NoContent(err.Code)
			return
		}
		slog.WarnContext(c.Request().Context(), "occurred unexpected error", slog.String("error", err.Error()))
		c.NoContent(http.StatusInternalServerError)
	}
}

// NewRateLimiter is a middleware that limits the rate of requests to the server.
func NewRateLimiter(limit float64, burst int, expressIn time.Duration) echo.MiddlewareFunc {
	if limit <= 0 {
		limit = 1
	}
	config := middleware.RateLimiterConfig{
		Skipper: func(c echo.Context) bool {
			event := c.Get("event").(slackevents.EventsAPIEvent)
			if event.Type == "" {
				return true
			}
			if event.Type == slackevents.URLVerification {
				return true
			}
			if c.Request().Header.Get("X-Slack-Retry-Num") != "" {
				return true
			}
			return middleware.DefaultSkipper(c)
		},
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(
			middleware.RateLimiterMemoryStoreConfig{Rate: rate.Limit(limit), Burst: burst, ExpiresIn: expressIn},
		),
		IdentifierExtractor: func(c echo.Context) (string, error) {
			user, err := userFromContext(c)
			if err != nil {
				return "", err
			}
			return user.ID, nil
		},
		ErrorHandler: func(_ echo.Context, err error) error {
			return err
		},
		DenyHandler: func(_ echo.Context, _ string, _ error) error {
			return echo.NewHTTPError(http.StatusTooManyRequests)
		},
	}
	f := middleware.RateLimiterWithConfig(config)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			slog.InfoContext(c.Request().Context(), "Begin rate limiter")
			defer slog.InfoContext(c.Request().Context(), "End rate limiter")
			return f(next)(c)
		}
	}
}

// NewSessionMiddleware is a middleware that checks if the session already exists.
func NewSessionMiddleware(rootCtx context.Context) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			slog.InfoContext(c.Request().Context(), "Begin session middleware")
			defer slog.InfoContext(c.Request().Context(), "End session middleware")
			innerEvent, ok := appMentionEventFromContext(c)
			if !ok {
				return next(c)
			}

			user, err := userFromContext(c)
			if err != nil {
				return err
			}
			session := newSession(rootCtx, user)
			_, load := sessions.LoadOrStore(sessionKey(innerEvent.Channel, innerEvent.TimeStamp, innerEvent.User), session)
			if load {
				// avoid duplicate requests
				slog.DebugContext(c.Request().Context(), "request was duplicated", slog.String("channel", innerEvent.Channel), slog.String("threadTs", innerEvent.ThreadTimeStamp), slog.String("user", innerEvent.User))
				return c.NoContent(http.StatusAccepted)
			}
			return next(c)
		}
	}
}

// userFromContext retrieves the user from the context.
func userFromContext(c echo.Context) (*slack.User, error) {
	user := c.Get("user")
	if user == nil {
		return nil, echo.NewHTTPError(http.StatusUnauthorized)
	}
	switch user := user.(type) {
	case *slack.User:
		return user, nil
	}
	return nil, echo.NewHTTPError(http.StatusInternalServerError)
}

// appMentionEventFromContext retrieves the AppMentionEvent from the context.
func appMentionEventFromContext(c echo.Context) (*slackevents.AppMentionEvent, bool) {
	event := c.Get("event")
	if event == nil {
		return nil, false
	}
	switch slackEvent := event.(type) {
	case slackevents.EventsAPIEvent:
		if slackEvent.Type != slackevents.CallbackEvent {
			return nil, false
		}
		appMentionEvent, ok := slackEvent.InnerEvent.Data.(*slackevents.AppMentionEvent)
		return appMentionEvent, ok
	}
	return nil, false
}
