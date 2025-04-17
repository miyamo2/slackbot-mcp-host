package interfaces

import (
	"context"
	"fmt"
	"github.com/slack-go/slack"
	"sync"
)

// Session represents a session for handling Slack events.
type Session struct {
	ctx    context.Context
	cancel context.CancelFunc
	user   *slack.User
}

func sessionKey(channel, ts, user string) string {
	return fmt.Sprintf("%s#%s#%s", channel, ts, user)
}

// newSession creates a new session.
func newSession(ctx context.Context, user *slack.User) *Session {
	ctx, cancel := context.WithCancel(ctx)
	return &Session{
		ctx:    ctx,
		cancel: cancel,
		user:   user,
	}
}

var sessions sync.Map
