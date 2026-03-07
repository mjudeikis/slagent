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

	// After finish: streaming msg deleted, final msg posted
	active := mock.activeMessages()
	if len(active) == 0 {
		t.Fatal("no active messages after Finish")
	}

	// The final message should contain the converted text
	last := active[len(active)-1]
	if last.Text == "" {
		t.Error("final message has empty text")
	}
}

func TestCompatTurnThinking(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "xoxc-test", "C_TEST")
	thread.Resume("1700000001.000000")

	turn := thread.NewTurn()
	turn.Thinking("Let me think about this...")
	turn.Thinking("\nMore thoughts")
	turn.Finish()

	// Thinking message should be deleted after finish
	active := mock.activeMessages()
	for _, m := range active {
		if m.Text == "thinking..." {
			t.Error("thinking message should be deleted after finish")
		}
	}
}

func TestCompatTurnTools(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "xoxc-test", "C_TEST")
	thread.Resume("1700000001.000000")

	turn := thread.NewTurn()
	turn.Tool("t1", "Read", "running", "main.go")
	turn.Tool("t1", "Read", "done", "")
	turn.Tool("t2", "Write", "running", "out.go")
	turn.Finish()

	// Tool message should be deleted after finish
	active := mock.activeMessages()
	for _, m := range active {
		if strings.Contains(m.Text, "Tool") && strings.Contains(m.Text, "Read") {
			t.Error("tool message should be deleted after finish")
		}
	}
}

func TestCompatTurnStatus(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "xoxc-test", "C_TEST")
	thread.Resume("1700000001.000000")

	turn := thread.NewTurn()

	// Status replaces previous
	turn.Status("searching...")
	turn.Status("compiling...")
	turn.Finish()

	// Both should be cleaned up
	active := mock.activeMessages()
	for _, m := range active {
		if m.Text == "searching..." || m.Text == "compiling..." {
			t.Error("status messages should be deleted after finish")
		}
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

func TestCompatToolMaxHistory(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "xoxc-test", "C_TEST")
	thread.Resume("1700000001.000000")

	turn := thread.NewTurn()
	impl := turn.(*turnImpl)
	w := impl.w.(*compatTurn)

	// Add more than maxToolHistory tools
	for i := 0; i < 8; i++ {
		turn.Tool("t"+string(rune('0'+i)), "Tool"+string(rune('0'+i)), "done", "")
	}

	if len(w.tools) > maxToolHistory {
		t.Errorf("tools = %d, want <= %d", len(w.tools), maxToolHistory)
	}
}
