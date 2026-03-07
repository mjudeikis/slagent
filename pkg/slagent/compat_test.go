package slagent

import (
	"strings"
	"testing"
)

func TestCompatTurnTextStreaming(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "xoxc-test", "C_TEST")
	thread.Resume("1700000001.000000")

	turn := thread.NewTurn()
	turn.Text("Hello ")
	turn.Text("world!")
	err := turn.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// After finish: text message updated to full content, not deleted
	active := mock.activeMessages()
	if len(active) == 0 {
		t.Fatal("no active messages after Finish")
	}

	// No messages should be deleted
	for _, m := range mock.postedMessages() {
		if m.Deleted {
			t.Error("no messages should be deleted")
		}
	}
}

func TestCompatTurnThinkingNotDeleted(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "xoxc-test", "C_TEST")
	thread.Resume("1700000001.000000")

	turn := thread.NewTurn()
	turn.Thinking("Let me think about this...")
	turn.Thinking("\nMore thoughts")
	turn.Finish()

	// Activity message should still exist (not deleted)
	active := mock.activeMessages()
	found := false
	for _, m := range active {
		if m.Text == "activity" {
			found = true
		}
	}
	if !found {
		t.Error("activity message should persist after finish")
	}

	// No deletions
	for _, m := range mock.postedMessages() {
		if m.Deleted {
			t.Error("no messages should be deleted")
		}
	}
}

func TestCompatTurnUnifiedActivity(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "xoxc-test", "C_TEST")
	thread.Resume("1700000001.000000")

	turn := thread.NewTurn()
	turn.Thinking("analyzing code")
	turn.Tool("t1", "Read", "running", "main.go")
	turn.Tool("t2", "Grep", "running", "pattern")
	turn.Status("compiling...")
	turn.Finish()

	// All activity should be in ONE message (same TS)
	active := mock.activeMessages()
	activityCount := 0
	for _, m := range active {
		if m.Text == "activity" {
			activityCount++
		}
	}
	if activityCount != 1 {
		t.Errorf("expected 1 activity message, got %d", activityCount)
	}

	// No deletions
	for _, m := range mock.postedMessages() {
		if m.Deleted {
			t.Error("no messages should be deleted")
		}
	}
}

func TestCompatTurnToolIcons(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "xoxc-test", "C_TEST")
	thread.Resume("1700000001.000000")

	turn := thread.NewTurn()
	impl := turn.(*turnImpl)
	w := impl.w.(*compatTurn)

	turn.Tool("t1", "Read", "running", "main.go")

	w.mu.Lock()
	display := w.renderActivity()
	w.mu.Unlock()

	if !strings.Contains(display, "📄") {
		t.Error("Read tool should use 📄 icon")
	}
}

func TestCompatTurnToolError(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "xoxc-test", "C_TEST")
	thread.Resume("1700000001.000000")

	turn := thread.NewTurn()
	impl := turn.(*turnImpl)
	w := impl.w.(*compatTurn)

	turn.Tool("t1", "Bash", ToolError, "go build")

	w.mu.Lock()
	display := w.renderActivity()
	w.mu.Unlock()

	if !strings.Contains(display, "❌") {
		t.Error("error tool should show ❌")
	}
}

func TestCompatTurnEmptyFinish(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "xoxc-test", "C_TEST")
	thread.Resume("1700000001.000000")

	// Finish with no content should not error
	turn := thread.NewTurn()
	err := turn.Finish()
	if err != nil {
		t.Fatalf("empty Finish: %v", err)
	}
}

func TestCompatActivityMaxLines(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "xoxc-test", "C_TEST")
	thread.Resume("1700000001.000000")

	turn := thread.NewTurn()
	impl := turn.(*turnImpl)
	w := impl.w.(*compatTurn)

	// Add more than maxDisplayLines activities
	for i := 0; i < 10; i++ {
		turn.Tool("t"+string(rune('0'+i)), "Tool", "done", "")
	}

	w.mu.Lock()
	display := w.renderActivity()
	w.mu.Unlock()

	lines := strings.Split(display, "\n")
	if len(lines) > maxDisplayLines {
		t.Errorf("activity lines = %d, want <= %d", len(lines), maxDisplayLines)
	}
}

func TestCompatTextLastNLines(t *testing.T) {
	result := lastNLines("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8", 6)
	lines := strings.Split(result, "\n")
	if len(lines) != 6 {
		t.Errorf("lastNLines returned %d lines, want 6", len(lines))
	}
	if lines[0] != "line3" {
		t.Errorf("first line = %q, want %q", lines[0], "line3")
	}
}

func TestCompatFinishUpdatesTextMessage(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "xoxc-test", "C_TEST")
	thread.Resume("1700000001.000000")

	turn := thread.NewTurn()
	turn.Text("line 1\n")
	turn.Text("line 2\n")
	turn.Text("line 3\n")
	err := turn.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// The text message should exist and be updated (not deleted + re-posted)
	active := mock.activeMessages()
	textMsgFound := false
	for _, m := range active {
		if m.Text != "activity" && m.Text != "" && m.IsUpdate {
			textMsgFound = true
		}
	}
	if !textMsgFound {
		t.Error("text message should be updated on finish")
	}
}
