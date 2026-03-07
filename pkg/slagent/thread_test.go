package slagent

import (
	"testing"
)

func TestIsNativeToken(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		{"xoxb-123-456", true},
		{"xoxc-123-456", false},
		{"xoxp-123-456", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isNativeToken(tt.token); got != tt.want {
			t.Errorf("isNativeToken(%q) = %v, want %v", tt.token, got, tt.want)
		}
	}
}

func TestThreadPermissions(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	// Thread with owner restriction
	thread := NewThread(mock.client(), "xoxc-test", "C_TEST", WithOwner("U_OWNER"))

	// Owner is authorized
	if !thread.isAuthorized("U_OWNER") {
		t.Error("owner should be authorized")
	}

	// Other user is not
	if thread.isAuthorized("U_OTHER") {
		t.Error("other user should not be authorized")
	}

	// !open command from owner
	if !thread.handleCommand("U_OWNER", "!open") {
		t.Error("!open from owner should be handled")
	}

	// Now other user is authorized
	if !thread.isAuthorized("U_OTHER") {
		t.Error("other user should be authorized after !open")
	}

	// !close from owner
	if !thread.handleCommand("U_OWNER", "!close") {
		t.Error("!close from owner should be handled")
	}
	if thread.isAuthorized("U_OTHER") {
		t.Error("other user should not be authorized after !close")
	}

	// !open from non-owner is ignored
	if thread.handleCommand("U_OTHER", "!open") {
		t.Error("!open from non-owner should not be handled")
	}
}

func TestThreadOpenAccess(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "xoxc-test", "C_TEST", WithOpenAccess())

	if !thread.isAuthorized("U_ANYONE") {
		t.Error("anyone should be authorized with open access")
	}
}

func TestThreadNoOwner(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	// No owner restriction
	thread := NewThread(mock.client(), "xoxc-test", "C_TEST")

	if !thread.isAuthorized("U_ANYONE") {
		t.Error("anyone should be authorized with no owner set")
	}
}

func TestThreadStartAndResume(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "xoxc-test", "C_TEST")

	// Start
	url, err := thread.Start("Test Plan")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if url == "" {
		t.Error("Start returned empty URL")
	}
	if thread.ThreadTS() == "" {
		t.Error("ThreadTS is empty after Start")
	}

	// Resume
	thread2 := NewThread(mock.client(), "xoxc-test", "C_TEST")
	thread2.Resume("1700000001.000000")
	if thread2.ThreadTS() != "1700000001.000000" {
		t.Errorf("ThreadTS = %q after Resume, want 1700000001.000000", thread2.ThreadTS())
	}
}

func TestThreadPost(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	thread := NewThread(mock.client(), "xoxc-test", "C_TEST")
	thread.Start("Test")

	err := thread.Post("hello from bot")
	if err != nil {
		t.Fatalf("Post: %v", err)
	}

	msgs := mock.postedMessages()
	found := false
	for _, m := range msgs {
		if m.Text == "hello from bot" {
			found = true
		}
	}
	if !found {
		t.Error("posted message not found in mock")
	}
}

func TestNewTurnSelectsBackend(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	// Compat backend for session token
	compat := NewThread(mock.client(), "xoxc-test", "C_TEST")
	compat.Resume("1700000001.000000")
	turn := compat.NewTurn()
	impl := turn.(*turnImpl)
	if _, ok := impl.w.(*compatTurn); !ok {
		t.Error("expected compatTurn for xoxc token")
	}

	// Note: native backend detection is tested via isNativeToken;
	// actual nativeTurn creation needs the mock URL override which
	// the raw HTTP calls in nativeTurn don't use (they hit slack.com).
}
