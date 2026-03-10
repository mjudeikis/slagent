// Package client provides an authenticated Slack client wrapper.
package client

import (
	"fmt"
	"net/http"
	"strings"

	slackapi "github.com/slack-go/slack"
)

// Client wraps *slack.Client with token metadata for raw API calls.
type Client struct {
	*slackapi.Client
	token      string
	cookie     string
	enterprise bool
}

// Token returns the raw Slack token.
func (c *Client) Token() string { return c.token }

// Cookie returns the session cookie (empty for bot/user tokens).
func (c *Client) Cookie() string { return c.cookie }

// Enterprise returns true for enterprise grid workspaces.
func (c *Client) Enterprise() bool { return c.enterprise }

// SetEnterprise marks this client as enterprise grid.
func (c *Client) SetEnterprise(v bool) { c.enterprise = v }

// ErrEnterprise is returned when a Slack API method is restricted on enterprise grid.
var ErrEnterprise = fmt.Errorf("enterprise grid workspace restricts this API (token would be invalidated)")

// enterpriseBlocked returns true if this is an enterprise grid workspace with
// a session token (xoxc-). User tokens (xoxp-) and bot tokens (xoxb-) from
// Slack apps work fine on enterprise and are not blocked.
func (c *Client) enterpriseBlocked() bool {
	return c.enterprise && strings.HasPrefix(c.token, "xoxc-")
}

// GetConversationInfo overrides the embedded method to block on enterprise grid
// with session tokens.
func (c *Client) GetConversationInfo(input *slackapi.GetConversationInfoInput) (*slackapi.Channel, error) {
	if c.enterpriseBlocked() {
		return nil, ErrEnterprise
	}
	return c.Client.GetConversationInfo(input)
}

// GetConversationsForUser overrides the embedded method to block on enterprise grid
// with session tokens.
func (c *Client) GetConversationsForUser(params *slackapi.GetConversationsForUserParameters) ([]slackapi.Channel, string, error) {
	if c.enterpriseBlocked() {
		return nil, "", ErrEnterprise
	}
	return c.Client.GetConversationsForUser(params)
}

// GetConversationHistory overrides the embedded method to block on enterprise grid
// with session tokens.
func (c *Client) GetConversationHistory(params *slackapi.GetConversationHistoryParameters) (*slackapi.GetConversationHistoryResponse, error) {
	if c.enterpriseBlocked() {
		return nil, ErrEnterprise
	}
	return c.Client.GetConversationHistory(params)
}

// HTTPDo executes a raw HTTP request with authentication headers set.
func (c *Client) HTTPDo(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	if c.cookie != "" {
		req.Header.Set("Cookie", fmt.Sprintf("d=%s", c.cookie))
	}
	return http.DefaultClient.Do(req)
}

// New creates an authenticated Slack client with optional cookie support.
// Extra slack.Option values (e.g. slackapi.OptionAPIURL for testing) are
// appended after the cookie transport option.
func New(token, cookie string, opts ...slackapi.Option) *Client {
	var allOpts []slackapi.Option
	if cookie != "" {
		allOpts = append(allOpts, slackapi.OptionHTTPClient(
			&cookieHTTPClient{cookie: cookie},
		))
	}
	allOpts = append(allOpts, opts...)
	return &Client{
		Client: slackapi.New(token, allOpts...),
		token:  token,
		cookie: cookie,
	}
}

// cookieHTTPClient injects the d= cookie on every request.
type cookieHTTPClient struct {
	cookie string
}

func (c *cookieHTTPClient) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Cookie", fmt.Sprintf("d=%s", c.cookie))
	return http.DefaultClient.Do(req)
}
