// Package session orchestrates the pairplan planning session.
package session

import (
	"context"
	"fmt"
	"os/user"
	"strings"
	"sync"
	"time"

	"github.com/sttts/pairplan/pkg/claude"
	pslack "github.com/sttts/pairplan/pkg/slack"
	"github.com/sttts/pairplan/pkg/terminal"
)

// Config holds session configuration.
type Config struct {
	Topic          string
	Channel        string
	ChannelName    string // display name (e.g. "#general" or "@haarchri")
	PermissionMode string
	SystemPrompt   string
	ResumeSessionID string // Claude session ID to resume
	ResumeThreadTS  string // Slack thread timestamp to resume
}

// ResumeInfo is returned by Run so the caller can print a resume command.
type ResumeInfo struct {
	SessionID string
	Channel   string
	ThreadTS  string
}

// Session is a running pairplan planning session.
type Session struct {
	cfg   Config
	ui    *terminal.UI
	proc  *claude.Process
	slack *pslack.Client

	// Slack reply queue: replies collected between turns
	replyMu     sync.Mutex
	replies     []pslack.Reply
	replyNotify chan struct{} // signaled when new replies arrive
}

// Run starts and runs the planning session until the user quits.
// Returns ResumeInfo so the caller can print a resume command.
func Run(ctx context.Context, cfg Config) (*ResumeInfo, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ui := terminal.New()
	sess := &Session{
		cfg:         cfg,
		ui:          ui,
		replyNotify: make(chan struct{}, 1),
	}

	// Set up Slack if channel is specified
	if cfg.Channel != "" {
		slackClient, err := pslack.New(cfg.Channel)
		if err != nil {
			return nil, fmt.Errorf("slack: %w", err)
		}
		sess.slack = slackClient
	}

	// Build system prompt with team feedback framing
	systemPrompt := cfg.SystemPrompt
	if sess.slack != nil {
		extra := "\n\nYou are in a collaborative planning session. " +
			"Messages prefixed with [Team feedback from Slack] contain input from team members " +
			"in a Slack thread. Consider their feedback and incorporate it into the plan."
		systemPrompt += extra
	}

	// Start Claude (with resume if specified)
	opts := []claude.Option{
		claude.WithPermissionMode(cfg.PermissionMode),
	}
	if systemPrompt != "" {
		opts = append(opts, claude.WithSystemPrompt(systemPrompt))
	}
	if cfg.ResumeSessionID != "" {
		opts = append(opts, claude.WithResume(cfg.ResumeSessionID))
	}

	proc, err := claude.Start(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}
	sess.proc = proc
	defer proc.Stop()

	// Resume or start Slack thread
	var threadURL string
	if sess.slack != nil {
		if cfg.ResumeThreadTS != "" {
			sess.slack.ResumeThread(cfg.ResumeThreadTS)
			threadURL = "(resumed)"
		} else {
			url, err := sess.slack.StartThread(cfg.Topic)
			if err != nil {
				return nil, fmt.Errorf("start slack thread: %w", err)
			}
			threadURL = url
		}
	}

	// Hide cursor during session, restore on exit
	ui.HideCursor()
	defer ui.ShowCursor()

	// Print banner
	channelDisplay := cfg.ChannelName
	if channelDisplay == "" {
		channelDisplay = cfg.Channel
	}
	ui.Banner(cfg.Topic, channelDisplay, threadURL)

	// Send initial topic (skip on resume — Claude already has context)
	if cfg.ResumeSessionID == "" {
		if sess.slack != nil {
			username := currentUser()
			go sess.slack.PostUserMessage(username, cfg.Topic)
		}
		if err := proc.Send(cfg.Topic); err != nil {
			return nil, fmt.Errorf("send topic: %w", err)
		}
		if err := sess.readTurn(); err != nil {
			return nil, fmt.Errorf("reading initial response: %w", err)
		}
	}

	// Without Slack, we're done after the initial response
	if sess.slack == nil {
		return nil, nil
	}

	// Start Slack poller
	go sess.pollSlack(ctx)

	// Auto-inject Slack feedback loop
	for {
		ui.Info("⏳ Waiting for team feedback from Slack...")
		replies, ok := sess.waitForReplies(ctx)
		if !ok {
			break
		}

		// Show in terminal
		for _, r := range replies {
			ui.SlackMessage(r.User, r.Text)
		}

		// Format and send to Claude
		var sb strings.Builder
		sb.WriteString("[Team feedback from Slack thread]\n")
		for _, r := range replies {
			fmt.Fprintf(&sb, "@%s: %s\n", r.User, r.Text)
		}

		if err := proc.Send(sb.String()); err != nil {
			ui.Error(fmt.Sprintf("send to claude: %v", err))
			break
		}
		if err := sess.readTurn(); err != nil {
			ui.Error(fmt.Sprintf("reading response: %v", err))
			break
		}
	}

	ui.Info("👋 Session ended.")

	// Build resume info
	resume := &ResumeInfo{
		SessionID: proc.SessionID(),
		Channel:   cfg.Channel,
		ThreadTS:  sess.slack.ThreadTS(),
	}

	return resume, nil
}

// readTurn reads events from Claude until the turn ends (result event).
func (s *Session) readTurn() error {
	s.ui.StartResponse()
	var fullText strings.Builder
	var thinkingText strings.Builder
	thinkingShown := false

	// Set up live thinking indicator for Slack
	var lt *pslack.LiveThinking
	if s.slack != nil {
		lt = s.slack.NewLiveThinking()
	}

	for {
		evt, err := s.proc.ReadEvent()
		if err != nil {
			if lt != nil {
				lt.Done()
			}
			s.ui.EndResponse()
			return err
		}
		if evt == nil {
			if lt != nil {
				lt.Done()
			}
			s.ui.EndResponse()
			return fmt.Errorf("unexpected EOF from Claude")
		}

		switch evt.Type {
		case "text_delta":
			// End thinking phase on first text
			if lt != nil && thinkingShown {
				lt.Done()
			}
			s.ui.StreamText(evt.Text)
			fullText.WriteString(evt.Text)

		case "thinking":
			if !thinkingShown {
				s.ui.Thinking()
				thinkingShown = true
				if lt != nil {
					go lt.Start()
				}
			}
			thinkingText.WriteString(evt.Text)
			if lt != nil {
				go lt.Update(thinkingText.String())
			}

		case claude.TypeAssistant:
			// Complete message — we already streamed the text, but record it
			if fullText.Len() == 0 && evt.Text != "" {
				s.ui.StreamText(evt.Text)
				fullText.WriteString(evt.Text)
			}

		case "tool_use":
			// End thinking phase if still active
			if lt != nil && thinkingShown {
				lt.Done()
			}
			s.ui.ToolActivity(evt.ToolName, summarizeToolInput(evt.ToolInput))
			if s.slack != nil {
				go s.slack.PostToolActivity(fmt.Sprintf("%s: %s", evt.ToolName, summarizeToolInput(evt.ToolInput)))
			}

		case claude.TypeResult:
			if lt != nil {
				lt.Done()
			}
			s.ui.EndResponse()

			// Post complete response to Slack
			text := fullText.String()
			if s.slack != nil && text != "" {
				go s.slack.PostClaudeMessage(text)
			}
			return nil

		case claude.TypeSystem:
			// Ignore system events (emitted at start of each turn)
		}
	}
}

// waitForReplies blocks until Slack replies are available or context is cancelled.
func (s *Session) waitForReplies(ctx context.Context) ([]pslack.Reply, bool) {
	select {
	case <-ctx.Done():
		return nil, false
	case <-s.replyNotify:
		s.replyMu.Lock()
		replies := s.replies
		s.replies = nil
		s.replyMu.Unlock()
		return replies, true
	}
}

// pollSlack continuously polls for new Slack thread replies.
func (s *Session) pollSlack(ctx context.Context) {
	ticker := time.NewTicker(pslack.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			replies, err := s.slack.PollReplies()
			if err != nil {
				continue
			}
			if len(replies) > 0 {
				s.replyMu.Lock()
				s.replies = append(s.replies, replies...)
				s.replyMu.Unlock()

				// Signal that replies are available
				select {
				case s.replyNotify <- struct{}{}:
				default:
				}
			}
		}
	}
}

func currentUser() string {
	u, err := user.Current()
	if err != nil {
		return "developer"
	}
	return u.Username
}

func summarizeToolInput(input string) string {
	if len(input) > 80 {
		return input[:77] + "..."
	}
	return input
}
