# pairplan — Agent Rules

## Project
Go CLI that mirrors Claude Code planning sessions to Slack threads.

## Structure
```
cmd/pairplan/main.go       — CLI entry point (start, auth, share, status)
pkg/claude/events.go       — Stream-JSON event parsing
pkg/claude/process.go      — Claude subprocess management
pkg/terminal/terminal.go   — Terminal output (writes to io.Writer for testability)
pkg/slack/client.go        — Slack API + credential management
pkg/slagent/thread.go      — Thread lifecycle, reply polling, prompt posting
pkg/slagent/turn.go        — Turn interface (Thinking/Tool/Text/MarkQuestion/Finish)
pkg/slagent/compat.go      — Compat backend (postMessage/update for session/user tokens)
pkg/slagent/native.go      — Native backend (chat.startStream for bot tokens)
pkg/slagent/mrkdwn.go      — Markdown → Slack mrkdwn conversion
pkg/session/session.go     — Session orchestration (wires claude+terminal+slack)
```

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

## Testing
- Table-driven tests for event sequences in `pkg/slagent/event_sequence_test.go`.
- Each test case replays events through both Slack (mock) and terminal (captured io.Writer).
- Fields: `wantSlack`, `wantSlackPrefix`, `wantSlackSuffix`, `wantSlackActivity`, `wantTerminal`.
- Mock Slack server in `pkg/slagent/mock_test.go`.
- Session-level tests (interactivePrompt, formatTool, toolDetail) in `pkg/session/session_test.go`.
- Never skip tests to make CI pass. Fix the actual issue.

## Slack Formatting
- Text messages: `🤖 <mrkdwn converted text>` (inline prefix, no code block).
- Activity messages: context block with thinking/tool/status lines (max 6 lines).
- Free-text AskUserQuestion: prefix `<@owner>: ` prepended at finish time via `MarkQuestion(prefix)`.
  Claude streams text BEFORE calling AskUserQuestion, so prefix must be prepended after buffering.
- Trailing `?` replaced with ` ❓` on finish for question turns.
- Multi-choice AskUserQuestion: separate prompt message with numbered emoji reactions.
- ExitPlanMode/EnterPlanMode: prompt with ✅/❌ reactions.
- Thread parent: `:thread: :claude: <title>` (plain text for emoji shortcode rendering).
- Code diffs (Edit/Write): posted as separate messages with ``` blocks.
- Use `--debug` flag to see raw JSON events for troubleshooting.

## Coding Style
- Comment style: one-line comment above small blocks of logically connected lines.
- Avoid duplicate code; prefer shared helpers.
- Keep blank line above comments unless comment starts a scope.
- Preserve existing formatting unless changing semantics.

## Architecture Notes
- Turn interface abstracts Slack backends (compat vs native).
- compat: throttled postMessage/update (1/sec), debounce timers for text and activity.
- native: chat.startStream/appendStream/stopStream (bot tokens only).
- `readTurn` in session.go maps Claude stream-JSON events to Turn method calls.
- Event order: text_delta* → tool_use → text_delta* → result (tool_use comes AFTER text).
- `interactivePrompt()` returns nil for non-interactive tools; handled in readTurn's switch.
