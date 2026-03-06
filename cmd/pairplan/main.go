package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/sttts/pairplan/pkg/session"
	pslack "github.com/sttts/pairplan/pkg/slack"
	"github.com/sttts/pairplan/pkg/slack/extract"
)

var cli struct {
	Start    StartCmd    `cmd:"" help:"Start a planning session mirrored to Slack."`
	Auth     AuthCmd     `cmd:"" help:"Set up Slack credentials."`
	Channels ChannelsCmd `cmd:"" help:"List Slack channels and group DMs."`
	Share    ShareCmd    `cmd:"" help:"Post a plan file to Slack for review."`
	Status   StatusCmd   `cmd:"" help:"Show current configuration."`
}

// StartCmd starts an interactive planning session with Claude Code.
type StartCmd struct {
	Channel        string   `short:"c" help:"Slack channel name or ID." placeholder:"CHANNEL"`
	User           string   `short:"u" help:"Slack user for DM (name or @name)." placeholder:"USER"`
	Topic          []string `arg:"" optional:"" help:"Planning topic."`
	PermissionMode string   `help:"Claude permission mode." default:"plan"`
}

func (cmd *StartCmd) Run() error {
	cfg := session.Config{
		PermissionMode: cmd.PermissionMode,
		Topic:          strings.Join(cmd.Topic, " "),
		Channel:        cmd.Channel,
	}

	// Resolve --channel name or --user to a channel ID
	if cmd.User != "" || (cfg.Channel != "" && !isSlackID(cfg.Channel)) {
		client, err := pslack.New("")
		if err != nil {
			return err
		}
		if cmd.User != "" {
			chID, err := client.ResolveUserChannel(cmd.User, userProgress)
			fmt.Fprint(os.Stderr, "\r\033[K")
			if err != nil {
				return fmt.Errorf("resolving user: %w", err)
			}
			cfg.Channel = chID
		} else {
			chID, err := client.ResolveChannelByName(cfg.Channel)
			if err != nil {
				return fmt.Errorf("resolving channel: %w", err)
			}
			cfg.Channel = chID
		}
	}

	// If no channel given, prompt with channel list when credentials exist
	if cfg.Channel == "" {
		if _, err := pslack.LoadCredentials(); err == nil {
			cfg.Channel = promptChannel()
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	return session.Run(ctx, cfg)
}

// AuthCmd sets up Slack credentials.
type AuthCmd struct {
	Extract bool `help:"Extract credentials from local Slack desktop app."`
}

func (cmd *AuthCmd) Run() error {
	if cmd.Extract {
		return runAuthExtract()
	}
	return runAuthManual()
}

// ChannelsCmd lists accessible Slack channels.
type ChannelsCmd struct{}

func (cmd *ChannelsCmd) Run() error {
	client, err := pslack.New("")
	if err != nil {
		return err
	}

	channels, err := client.ListChannels(slackProgress)
	fmt.Fprint(os.Stderr, "\r\033[K")
	if err != nil {
		return fmt.Errorf("listing channels: %w", err)
	}

	for i, ch := range channels {
		name := ch.Name
		if ch.Type == "channel" || ch.Type == "group" {
			name = "#" + name
		}
		fmt.Printf("  %2d) %s\n", i+1, name)
	}
	return nil
}

// ShareCmd posts a plan file to Slack.
type ShareCmd struct {
	File    string `arg:"" help:"Plan file to share."`
	Channel string `short:"c" required:"" help:"Slack channel name or ID." placeholder:"CHANNEL"`
}

func (cmd *ShareCmd) Run() error {
	content, err := os.ReadFile(cmd.File)
	if err != nil {
		return fmt.Errorf("reading %s: %w", cmd.File, err)
	}

	channel := cmd.Channel
	if !isSlackID(channel) {
		client, err := pslack.New("")
		if err != nil {
			return err
		}
		channel, err = client.ResolveChannelByName(channel)
		if err != nil {
			return fmt.Errorf("resolving channel: %w", err)
		}
	}

	client, err := pslack.New(channel)
	if err != nil {
		return err
	}

	topic := fmt.Sprintf("Plan review: %s", cmd.File)
	url, err := client.StartThread(topic)
	if err != nil {
		return err
	}

	if err := client.PostClaudeMessage(string(content)); err != nil {
		return fmt.Errorf("posting plan: %w", err)
	}

	fmt.Printf("Plan shared: %s\n", url)
	return nil
}

// StatusCmd shows current configuration.
type StatusCmd struct{}

func (cmd *StatusCmd) Run() error {
	fmt.Println("No active session.")
	creds, err := pslack.LoadCredentials()
	if err != nil {
		fmt.Println("Slack: not configured (run 'pairplan auth')")
	} else {
		token := creds.EffectiveToken()
		if len(token) > 10 {
			token = token[:10]
		}
		fmt.Printf("Slack: configured (%s token: %s...)\n", creds.EffectiveType(), token)
	}
	return nil
}

func main() {
	ctx := kong.Parse(&cli,
		kong.Name("pairplan"),
		kong.Description("Mirror Claude Code planning sessions to Slack threads."),
		kong.UsageOnError(),
	)
	if err := ctx.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// promptChannel lists channels and lets the user pick one, or type @username for a DM.
func promptChannel() string {
	client, err := pslack.New("")
	if err != nil {
		return ""
	}

	channels, err := client.ListChannels(slackProgress)
	fmt.Fprint(os.Stderr, "\r\033[K")
	if err != nil || len(channels) == 0 {
		return ""
	}

	fmt.Println("Pick a channel (or type @username for a DM):")
	for i, ch := range channels {
		name := ch.Name
		if ch.Type == "channel" || ch.Type == "group" {
			name = "#" + name
		}
		fmt.Printf("  %2d) %s\n", i+1, name)
	}
	fmt.Print("\nChannel: ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	// @username → resolve to DM channel
	if strings.HasPrefix(line, "@") {
		chID, err := client.ResolveUserChannel(line, userProgress)
		fmt.Fprint(os.Stderr, "\r\033[K")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return ""
		}
		return chID
	}

	idx := 0
	fmt.Sscanf(line, "%d", &idx)
	idx--
	if idx < 0 || idx >= len(channels) {
		fmt.Fprintf(os.Stderr, "Invalid choice\n")
		return ""
	}
	return channels[idx].ID
}

func runAuthManual() error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Slack Token Setup")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()
	fmt.Println("1. Go to https://api.slack.com/apps")
	fmt.Println("2. Create a new app (or select existing)")
	fmt.Println("3. Go to 'OAuth & Permissions'")
	fmt.Println("4. Add scopes (Bot or User Token Scopes):")
	fmt.Println("   - chat:write")
	fmt.Println("   - channels:history")
	fmt.Println("   - groups:history")
	fmt.Println("   - channels:read")
	fmt.Println("   - groups:read")
	fmt.Println("   - users:read")
	fmt.Println("5. Install the app to your workspace")
	fmt.Println("6. Copy the token (xoxb-... for bot, xoxp-... for user)")
	fmt.Println()

	fmt.Print("Paste your token: ")
	token, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	token = strings.TrimSpace(token)

	// Determine token type
	var tokenType string
	switch {
	case strings.HasPrefix(token, "xoxb-"):
		tokenType = "bot"
	case strings.HasPrefix(token, "xoxp-"):
		tokenType = "user"
	default:
		fmt.Fprintf(os.Stderr, "Warning: token doesn't start with 'xoxb-' or 'xoxp-'.\n")
		tokenType = "bot"
	}

	creds := &pslack.Credentials{Token: token, Type: tokenType}
	if err := pslack.SaveCredentials(creds); err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}

	fmt.Printf("\nCredentials saved to %s (%s token)\n", pslack.CredentialsPath(), tokenType)
	if tokenType == "bot" {
		fmt.Println("Don't forget to invite the bot to your channel: /invite @your-bot-name")
	}
	return nil
}

func runAuthExtract() error {
	fmt.Println("Extracting Slack credentials from desktop app...")
	fmt.Println("(you may see a macOS keychain access prompt — please allow access)")
	fmt.Println()

	result, err := extract.Extract()
	if err != nil {
		return err
	}

	// Choose workspace
	var ws extract.Workspace
	if len(result.Workspaces) == 1 {
		ws = result.Workspaces[0]
		fmt.Printf("Found workspace: %s (%s)\n", ws.Name, ws.URL)
	} else {
		fmt.Println("Found workspaces:")
		for i, w := range result.Workspaces {
			fmt.Printf("  %d) %s (%s)\n", i+1, w.Name, w.URL)
		}
		fmt.Print("\nChoose workspace [1]: ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		idx := 0
		if line != "" {
			fmt.Sscanf(line, "%d", &idx)
			idx--
		}
		if idx < 0 || idx >= len(result.Workspaces) {
			idx = 0
		}
		ws = result.Workspaces[idx]
	}

	creds := &pslack.Credentials{
		Token:  ws.Token,
		Type:   "session",
		Cookie: result.Cookie,
	}
	if err := pslack.SaveCredentials(creds); err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}

	tokenPreview := ws.Token
	if len(tokenPreview) > 14 {
		tokenPreview = tokenPreview[:14]
	}
	fmt.Printf("\nCredentials saved for %s (token: %s...)\n", ws.Name, tokenPreview)
	fmt.Printf("Credentials file: %s\n", pslack.CredentialsPath())
	return nil
}

// isSlackID returns true if s looks like a Slack channel/user ID (e.g. C01234, G01234, D01234).
func isSlackID(s string) bool {
	if len(s) < 2 {
		return false
	}
	prefix := s[0]
	return (prefix == 'C' || prefix == 'G' || prefix == 'D') && s[1] >= '0' && s[1] <= '9'
}

func userProgress(checked int) {
	fmt.Fprintf(os.Stderr, "\rsearching users... %d", checked)
}

func slackProgress(p pslack.ListProgress) {
	switch p.Phase {
	case "checking":
		fmt.Fprintf(os.Stderr, "\rchecking recent activity... %d/%d", p.Done, p.Total)
	default:
		fmt.Fprintf(os.Stderr, "\rfetching channels... %d", p.Done)
	}
}
