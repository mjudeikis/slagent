# slagent — Agent Rules

## Project
Go library for streaming agent sessions to Slack threads, plus `slaude` CLI that wraps Claude Code.

## Structure
```
slagent.go, thread.go, turn.go   — Root library: Thread, Turn, NewSlackClient
compat.go, native.go             — Compat (postMessage/update) and native (chat.startStream) backends
mrkdwn.go, reply.go              — Markdown→mrkdwn, reply polling

credential/                      — Load, Save, Extract tokens from Slack desktop app
  credential.go                  — Credentials struct, Load, Save, Path
  extract.go, leveldb.go         — Extract() orchestrator, LevelDB token reading
  cookie.go, decrypt.go, paths.go

channel/                         — Resolve, List
  channel.go                     — Client, ResolveByName, ResolveUser, List

cmd/slaude/                      — CLI (auth, channels, share, status, start)
  main.go
cmd/slaude/internal/
  session/session.go             — Session orchestration
  claude/process.go, events.go   — Claude subprocess, stream-JSON parsing
  terminal/terminal.go           — Terminal UI
  perms/server.go, listener.go   — MCP permission server for Slack-based tool approval

cmd/slagent-demo/                — Demo CLI
  main.go

contrib/                         — Project assets
  logo.png                       — Project logo (update here, not root)
```

Module: `github.com/sttts/slagent`

## Tasks
- Use the task system (TaskCreate/TaskUpdate/TaskList) for everything the user asks.
- Mark tasks in_progress before starting, completed when done.

## Commit Rules
- Title convention: `area/subarea: short what has been done`
- Commit in sensible chunks. Don't mix topics.
- Add files individually (not `git add -A`).
- Do `git add` and `git commit` in one command.
- Don't push without being asked.
- Before committing, simplify the code. Look deeply at changes.

## Build
- ALWAYS build binaries into the repo root after ANY code change: `go build -o slaude ./cmd/slaude/ && go build -o slagent-demo ./cmd/slagent-demo/`
- The user runs `./slaude` from the repo root. If you don't rebuild, they test stale code.
- Don't just run `go build ./...` — that only checks compilation, it doesn't produce binaries.

## Testing
- Table-driven tests for event sequences in `event_sequence_test.go`.
- Each test case replays events through both Slack (mock) and terminal (captured io.Writer).
- Fields: `wantSlack`, `wantSlackPrefix`, `wantSlackSuffix`, `wantSlackActivity`, `wantTerminal`.
- Mock Slack server in `mock_test.go`.
- Session-level tests (interactivePrompt, formatTool, toolDetail) in `cmd/slaude/internal/session/session_test.go`.
- Never skip tests to make CI pass. Fix the actual issue.

## Slack Formatting
- Text messages: `🤖 <mrkdwn converted text>` (inline prefix, no code block).
- Activity messages: context block with thinking/tool/status lines (max 6 lines). Transient — deleted when text arrives.
- Free-text AskUserQuestion: prefix `<@owner>: ` prepended at finish time via `MarkQuestion(prefix)`.
  Claude streams text BEFORE calling AskUserQuestion, so prefix must be prepended after buffering.
- Trailing `?` replaced with ` ❓` on finish for question turns.
- Multi-choice AskUserQuestion: separate prompt message with numbered emoji reactions.
- ExitPlanMode/EnterPlanMode: prompt with ✅/❌ reactions.
- Thread parent: `🧵 <title>` (plain text for emoji shortcode rendering).
- Code diffs (Edit/Write): posted as separate messages with ``` blocks.
- Use `--debug` flag to see raw JSON events for troubleshooting.

## Documentation
- User-facing knowledge (commands, syntax, usage) goes in `README.md`.
- `AGENTS.md` references README for UX and adds implementation details only.
- `doc/designs/` contains detailed design docs — keep in sync with code changes.
- When changing behavior, update README.md, AGENTS.md, and relevant design docs together.

## Coding Style
- Comment style: one-line comment above small blocks of logically connected lines.
- Avoid duplicate code; prefer shared helpers.
- Keep blank line above comments unless comment starts a scope.
- Preserve existing formatting unless changing semantics.
- Never stutter in package APIs (e.g. `credential.Load()` not `credential.LoadCredentials()`).
- Use short canonical variable names like `ch`, `ts`, `cfg`. Long names are for packages.

## Architecture Notes
- Turn interface abstracts Slack backends (compat vs native).
- compat: throttled postMessage/update (1/sec), debounce timers for text and activity. Activity is transient (deleted when text arrives).
- native: chat.startStream/appendStream/stopStream (bot tokens only).
- `readTurn` in session.go maps Claude stream-JSON events to Turn method calls.
- Event order: text_delta* → tool_use → text_delta* → result (tool_use comes AFTER text).
- `interactivePrompt()` returns nil for non-interactive tools; handled in readTurn's switch.
- Claude args after `--` are passed through to the subprocess. slaude only owns its own flags.

## Permission Approval via MCP
- Claude Code's `--permission-prompt-tool` delegates permission decisions to an MCP tool.
- slaude runs a Unix socket listener; Claude starts `slaude _mcp-permissions` as MCP stdio server.
- Flow: Claude needs permission → MCP tool → Unix socket → slaude posts ✅❌ to Slack → polls owner reaction → returns allow/deny.
- Allow response MUST include `updatedInput` with original tool input (Claude validates as union type).
- Deny response MUST include `message`.
- Permission prompt message is deleted from Slack after approval/denial to keep thread clean.
- `--mcp-config` expects a file path, not inline JSON. Config written to temp file.
- Sandbox violations (outside working directory) are blocked by Claude Code before the permission prompt fires.

## Slack Thread Message Ordering (bottom of thread)
Messages at the end of the thread follow this order:
1. **Activity message** — `:claude: Tool` / `✓ Tool: detail` (animated, per-turn)
2. **Tasks message** — TODO list from Claude's TodoWrite/TaskCreate events (persistent, updated across turns, only shown when tasks exist)
3. **Question/prompt** — `❓` free-text or `🗳️` multi-choice with reaction emojis (optional)

Tasks message is delete+repost on each new turn to stay near the bottom.
The activity message is managed by Turn (compat backend). Tasks message is managed by Session.

## Task Tracking in Slack
- Intercept `TodoWrite` and `TaskCreate`/`TaskUpdate` tool_use events from Claude's stream.
- Maintain task state in Session across turns.
- Render as a persistent Slack message below activity, above questions.
- TodoWrite replaces entire list; TaskCreate/TaskUpdate modify individual items.
- Format: `📋 Tasks\n☐ pending\n⏳ in_progress\n✅ completed`

## Emoji-Prefix Instance Targeting
See README.md for user-facing syntax (`:shortcode::` prefix, `/open`, `/lock` commands, title format).

Implementation:
- `parseInstancePrefix()` in `thread.go`, used by `pollOnce()` in `reply.go`.
- Non-command messages delivered to ALL instances; commands are instance-exclusive.
- Unknown `/commands` forwarded to Claude via `Reply.Command`.
- `mistargeted()` detects wrong syntax (Unicode emoji or single colon) and posts a hint.

## Thread Access Control
See README.md for commands and title format.

Implementation:
- `handleCommand()` in `thread.go` processes `/open`, `/lock`, `/close`.
- `isAuthorized()` checks banned → openAccess → owner → allowedUsers.
- `formatTitle()` / `parseTitle()` encode/decode access state in thread parent.
- Title parsed on `Resume()` to recover state. Other slaude instances subject to same rules.
