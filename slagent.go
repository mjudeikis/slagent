// Package slagent provides a unified streaming interface for Slack agent sessions.
//
// It supports two backends transparently:
//   - Native streaming (bot tokens, xoxb-*): uses chat.startStream/appendStream/stopStream
//   - Compat streaming (session/user tokens): uses chat.postMessage/chat.update
//
// The backend is selected automatically based on the token type.
package slagent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	slackapi "github.com/slack-go/slack"
)

// Tool status constants for use with Turn.Tool().
const (
	ToolRunning = "running"
	ToolDone    = "done"
	ToolError   = "error"
)

// Reply is a message from a thread participant.
type Reply struct {
	User   string // Display name
	UserID string // Slack user ID
	Text   string
}

// ThreadOption configures a Thread.
type ThreadOption func(*threadConfig)

type threadConfig struct {
	ownerID           string
	instanceID        string
	openAccess        bool
	pollInterval      time.Duration
	bufferSize        int
	markdownConverter func(string) string
	apiURL            string     // base URL for native streaming API calls (testing)
	slackLog          io.Writer  // if non-nil, log all Slack API calls here
}

// randomInstanceID generates a short random hex string for instance identification.
func randomInstanceID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func defaultConfig() threadConfig {
	return threadConfig{
		pollInterval:      3 * time.Second,
		bufferSize:        256,
		markdownConverter: MarkdownToMrkdwn,
	}
}

// WithOwner restricts the thread to only accept input from the given user ID.
func WithOwner(userID string) ThreadOption {
	return func(c *threadConfig) { c.ownerID = userID }
}

// WithOpenAccess allows all thread participants to send input.
func WithOpenAccess() ThreadOption {
	return func(c *threadConfig) { c.openAccess = true }
}

// WithPollInterval sets the polling interval for new replies.
func WithPollInterval(d time.Duration) ThreadOption {
	return func(c *threadConfig) { c.pollInterval = d }
}

// WithBufferSize sets the text buffer size before flushing (native backend).
func WithBufferSize(n int) ThreadOption {
	return func(c *threadConfig) { c.bufferSize = n }
}

// WithMarkdownConverter sets a custom markdown-to-mrkdwn converter.
func WithMarkdownConverter(fn func(string) string) ThreadOption {
	return func(c *threadConfig) { c.markdownConverter = fn }
}

// withAPIURL sets the base URL for native streaming API calls (testing only).
func withAPIURL(url string) ThreadOption {
	return func(c *threadConfig) { c.apiURL = url }
}

// WithInstanceID sets a specific instance ID for message tagging.
// If not set, a random one is generated. On resume, pass the original
// instance ID so the poller correctly identifies messages from this session.
func WithInstanceID(id string) ThreadOption {
	return func(c *threadConfig) { c.instanceID = id }
}

// WithSlackLog enables logging of all Slack API calls to w.
func WithSlackLog(w io.Writer) ThreadOption {
	return func(c *threadConfig) { c.slackLog = w }
}

// NewSlackClient creates a *slack.Client with optional cookie support.
func NewSlackClient(token, cookie string) *slackapi.Client {
	if cookie != "" {
		return slackapi.New(token, slackapi.OptionHTTPClient(
			&cookieHTTPClient{cookie: cookie},
		))
	}
	return slackapi.New(token)
}

// cookieHTTPClient wraps http.Client and injects the d= cookie on every request.
type cookieHTTPClient struct {
	inner  *http.Client
	cookie string
}

func (c *cookieHTTPClient) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Cookie", fmt.Sprintf("d=%s", c.cookie))
	if c.inner == nil {
		return http.DefaultClient.Do(req)
	}
	return c.inner.Do(req)
}
