package slagent

import (
	"strings"
	"testing"
)

func TestPostPromptAddsReactions(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "C_TEST", WithOwner("U_OWNER"))
	thread.Start("test")

	reactions := []string{"one", "two", "three", "x"}
	msgTS, err := thread.PostPrompt("pick one", reactions)
	if err != nil {
		t.Fatal(err)
	}

	// All reactions should be pre-added for session tokens
	for _, r := range reactions {
		mock.mu.Lock()
		users := mock.reactions[msgTS][r]
		mock.mu.Unlock()
		if !users["U_OWNER"] {
			t.Errorf("reaction %q should be pre-added, got users: %v", r, users)
		}
	}
}

func TestGetReactionsReturnsState(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "C_TEST", WithOwner("U_OWNER"))
	thread.Start("test")

	reactions := []string{"one", "two", "x"}
	msgTS, err := thread.PostPrompt("pick", reactions)
	if err != nil {
		t.Fatal(err)
	}

	items, err := thread.GetReactions(msgTS)
	if err != nil {
		t.Fatal(err)
	}

	// Should have all 3 reactions with owner
	found := make(map[string]bool)
	for _, item := range items {
		found[item.Name] = true
	}
	for _, r := range reactions {
		if !found[r] {
			t.Errorf("GetReactions missing %q", r)
		}
	}
}

func TestGetReactionsAfterOwnerClick(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "C_TEST", WithOwner("U_OWNER"))
	thread.Start("test")

	reactions := []string{"one", "two", "x"}
	msgTS, err := thread.PostPrompt("pick", reactions)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate owner clicking "two" (removes it)
	mock.simulateOwnerClick(msgTS, "two")

	items, err := thread.GetReactions(msgTS)
	if err != nil {
		t.Fatal(err)
	}

	// "two" should be gone (or empty users), "one" and "x" should still have owner
	for _, item := range items {
		if item.Name == "two" {
			for _, u := range item.Users {
				if u == "U_OWNER" {
					t.Error("owner should not be on 'two' after click")
				}
			}
		}
	}
}

func TestAddReactionRestoresAfterClick(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "C_TEST", WithOwner("U_OWNER"))
	thread.Start("test")

	reactions := []string{"one", "two"}
	msgTS, err := thread.PostPrompt("pick", reactions)
	if err != nil {
		t.Fatal(err)
	}

	// Owner clicks "one"
	mock.simulateOwnerClick(msgTS, "one")

	// Re-add it
	thread.AddReaction(msgTS, "one")

	// Owner should be back on "one"
	mock.mu.Lock()
	hasOwner := mock.reactions[msgTS]["one"]["U_OWNER"]
	mock.mu.Unlock()
	if !hasOwner {
		t.Error("AddReaction should restore owner on the reaction")
	}
}

func TestRemoveAllReactions(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "C_TEST", WithOwner("U_OWNER"))
	thread.Start("test")

	reactions := []string{"one", "two", "x"}
	msgTS, err := thread.PostPrompt("pick", reactions)
	if err != nil {
		t.Fatal(err)
	}

	thread.RemoveAllReactions(msgTS, reactions)

	// All reactions should be removed
	items, err := thread.GetReactions(msgTS)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		for _, u := range item.Users {
			if u == "U_OWNER" {
				t.Errorf("reaction %q should be removed, still has owner", item.Name)
			}
		}
	}
}

func TestUpdateMessageChangesText(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "C_TEST", WithOwner("U_OWNER"))
	thread.Start("test")

	msgTS, err := thread.PostPrompt("original text", []string{"one"})
	if err != nil {
		t.Fatal(err)
	}

	// Update the message
	thread.UpdateMessage(msgTS, "updated with 👉")

	// Verify the message was updated
	msgs := mock.activeMessages()
	for _, msg := range msgs {
		if msg.TS == msgTS {
			if !strings.Contains(msg.blockText(), "👉") {
				t.Errorf("message should be updated, got: %q", msg.blockText())
			}
			return
		}
	}
	t.Error("message not found after update")
}

func TestThinkingEmoji(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "C_TEST",
		WithOwner("U_OWNER"),
		WithThinkingEmoji(":claude-thinking:"),
	)
	if thread.ThinkingEmoji() != ":claude-thinking:" {
		t.Errorf("ThinkingEmoji = %q, want :claude-thinking:", thread.ThinkingEmoji())
	}
}

func TestThinkingEmojiDefault(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "C_TEST")
	if thread.ThinkingEmoji() != ":claude:" {
		t.Errorf("ThinkingEmoji = %q, want :claude:", thread.ThinkingEmoji())
	}
}
