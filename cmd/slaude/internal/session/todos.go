package session

import (
	"encoding/json"
	"fmt"
	"strings"
)

// updateTodos parses a TodoWrite tool_use input and updates the task list in Slack.
func (s *Session) updateTodos(rawInput string) {
	var input struct {
		Todos []todo `json:"todos"`
	}
	if err := json.Unmarshal([]byte(rawInput), &input); err != nil || len(input.Todos) == 0 {
		// Empty list clears todos
		if s.todosTS != "" && s.thread != nil {
			s.thread.DeleteMessage(s.todosTS)
			s.todosTS = ""
		}
		s.todos = nil
		return
	}
	s.todos = input.Todos

	if s.thread == nil {
		return
	}

	text := s.formatTodos()
	if s.todosTS != "" {
		s.thread.UpdateMessage(s.todosTS, text)
	} else {
		ts, err := s.thread.Post(text)
		if err == nil {
			s.todosTS = ts
		}
	}
}

// repostTodos deletes and reposts the tasks message to keep it near the bottom.
func (s *Session) repostTodos() {
	if s.thread == nil || len(s.todos) == 0 {
		return
	}

	if s.todosTS != "" {
		s.thread.DeleteMessage(s.todosTS)
		s.todosTS = ""
	}

	text := s.formatTodos()
	ts, err := s.thread.Post(text)
	if err == nil {
		s.todosTS = ts
	}
}

// formatTodos renders the task list as a Slack mrkdwn string.
func (s *Session) formatTodos() string {
	var b strings.Builder
	b.WriteString("📋 *Tasks*\n")
	for _, t := range s.todos {
		switch t.Status {
		case "completed":
			fmt.Fprintf(&b, "  ✅ ~%s~\n", t.Content)
		case "in_progress":
			fmt.Fprintf(&b, "  ⏳ %s\n", t.Content)
		default:
			fmt.Fprintf(&b, "  ☐ %s\n", t.Content)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
