package slagent

import (
	"fmt"
	"strings"
	"sync"
	"time"

	slackapi "github.com/slack-go/slack"
)

const (
	maxBlockTextLen  = 3000
	maxDisplayLines  = 6
	compatThrottleMs = 1000
)

// compatTurn implements turnWriter using postMessage/update for session/user tokens.
// Thinking + tools + status share a single "activity" message (≤6 lines, updated in-place).
// Text streams in a separate message (last 6 lines while streaming, full text on finish).
// No messages are ever deleted.
type compatTurn struct {
	api      *slackapi.Client
	channel  string
	threadTS string
	convert  func(string) string
	posted   func(ts string)

	// Unified activity message (thinking + tools + status)
	thinkBuf   strings.Builder // accumulated thinking text
	activities []string        // discrete lines: tools, status
	activityTS string          // single message timestamp
	actUpdate  time.Time       // throttle

	// Text streaming
	textBuf    strings.Builder
	textTS     string
	textUpdate time.Time

	mu sync.Mutex
}

func newCompatTurn(api *slackapi.Client, channel, threadTS string, convert func(string) string, posted func(string)) *compatTurn {
	return &compatTurn{
		api:      api,
		channel:  channel,
		threadTS: threadTS,
		convert:  convert,
		posted:   posted,
	}
}

// renderActivity builds the activity message content from thinking + activity lines,
// keeping at most maxDisplayLines. Must be called with lock held.
func (c *compatTurn) renderActivity() string {
	var lines []string

	// Thinking lines
	if c.thinkBuf.Len() > 0 {
		lines = append(lines, "💭 _thinking..._")
		thinkText := c.thinkBuf.String()
		if len(thinkText) > 500 {
			thinkText = "…" + thinkText[len(thinkText)-499:]
		}
		for _, l := range strings.Split(thinkText, "\n") {
			lines = append(lines, "  "+l)
		}
	}

	// Activity lines (tools, status)
	lines = append(lines, c.activities...)

	// Keep last maxDisplayLines
	if len(lines) > maxDisplayLines {
		lines = lines[len(lines)-maxDisplayLines:]
	}

	return strings.Join(lines, "\n")
}

// flushActivity posts or updates the unified activity message. Must be called with lock held.
func (c *compatTurn) flushActivity() {
	// Throttle to 1/sec
	if c.activityTS != "" && time.Since(c.actUpdate) < time.Duration(compatThrottleMs)*time.Millisecond {
		return
	}

	display := c.renderActivity()
	if display == "" {
		return
	}

	ctx := slackapi.NewContextBlock("",
		slackapi.NewTextBlockObject("mrkdwn", display, false, false),
	)

	if c.activityTS == "" {
		_, ts, err := c.api.PostMessage(
			c.channel,
			slackapi.MsgOptionBlocks(ctx),
			slackapi.MsgOptionText("activity", false),
			slackapi.MsgOptionTS(c.threadTS),
		)
		if err == nil {
			c.activityTS = ts
			c.posted(ts)
		}
	} else {
		c.api.UpdateMessage(
			c.channel,
			c.activityTS,
			slackapi.MsgOptionBlocks(ctx),
			slackapi.MsgOptionText("activity", false),
		)
	}
	c.actUpdate = time.Now()
}

func (c *compatTurn) writeThinking(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.thinkBuf.WriteString(text)
	c.flushActivity()
}

func (c *compatTurn) writeTool(id, name, status, detail string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	summary := name
	if detail != "" {
		summary += ": " + detail
	}
	if status == ToolError {
		summary += " ❌"
	}

	// Pick an icon based on the tool name
	icon := "🔧"
	switch {
	case name == "Read":
		icon = "📄"
	case name == "Glob" || name == "Grep":
		icon = "🔍"
	case name == "Bash":
		icon = "💻"
	}

	c.activities = append(c.activities, fmt.Sprintf("%s %s", icon, summary))
	c.flushActivity()
}

func (c *compatTurn) writeStatus(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if text == "" {
		return
	}

	c.activities = append(c.activities, fmt.Sprintf("⏳ %s", text))
	c.flushActivity()
}

func (c *compatTurn) writeText(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.textBuf.WriteString(text)

	// Throttle updates to 1/sec
	if c.textTS != "" && time.Since(c.textUpdate) < time.Duration(compatThrottleMs)*time.Millisecond {
		return
	}

	// Show last maxDisplayLines lines
	full := c.textBuf.String()
	display := lastNLines(full, maxDisplayLines)
	display = c.convert(display)

	section := slackapi.NewSectionBlock(
		slackapi.NewTextBlockObject("mrkdwn", display, false, false),
		nil, nil,
	)

	if c.textTS == "" {
		_, ts, err := c.api.PostMessage(
			c.channel,
			slackapi.MsgOptionBlocks(section),
			slackapi.MsgOptionText(display, false),
			slackapi.MsgOptionTS(c.threadTS),
		)
		if err == nil {
			c.textTS = ts
			c.posted(ts)
		}
	} else {
		c.api.UpdateMessage(
			c.channel,
			c.textTS,
			slackapi.MsgOptionBlocks(section),
			slackapi.MsgOptionText(display, false),
		)
	}
	c.textUpdate = time.Now()
}

// finish freezes the activity message and updates the text message to the full final response.
// No messages are deleted.
func (c *compatTurn) finish() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Final flush of activity (frozen as-is, no deletion)
	if c.activityTS != "" {
		display := c.renderActivity()
		ctx := slackapi.NewContextBlock("",
			slackapi.NewTextBlockObject("mrkdwn", display, false, false),
		)
		c.api.UpdateMessage(
			c.channel,
			c.activityTS,
			slackapi.MsgOptionBlocks(ctx),
			slackapi.MsgOptionText("activity", false),
		)
	}

	// Update text message to full final response
	finalText := c.textBuf.String()
	if finalText == "" {
		return nil
	}

	mrkdwn := c.convert(finalText)
	chunks := splitAtLines(mrkdwn, maxBlockTextLen)

	// Update existing text message with first chunk
	if c.textTS != "" && len(chunks) > 0 {
		section := slackapi.NewSectionBlock(
			slackapi.NewTextBlockObject("mrkdwn", chunks[0], false, false),
			nil, nil,
		)
		c.api.UpdateMessage(
			c.channel,
			c.textTS,
			slackapi.MsgOptionBlocks(section),
			slackapi.MsgOptionText(chunks[0], false),
		)
		chunks = chunks[1:]
	}

	// Post remaining chunks as new messages
	for _, chunk := range chunks {
		section := slackapi.NewSectionBlock(
			slackapi.NewTextBlockObject("mrkdwn", chunk, false, false),
			nil, nil,
		)
		_, ts, err := c.api.PostMessage(
			c.channel,
			slackapi.MsgOptionBlocks(section),
			slackapi.MsgOptionText(chunk, false),
			slackapi.MsgOptionTS(c.threadTS),
		)
		if err != nil {
			return err
		}
		c.posted(ts)
	}

	return nil
}

// lastNLines returns the last n lines of text.
func lastNLines(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
