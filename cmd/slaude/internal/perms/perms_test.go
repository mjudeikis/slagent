package perms

import (
	"encoding/json"
	"os"
	"testing"
)

func TestListenerRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		handler  Handler
		toolName string
		input    string
		wantBeh  string
		wantMsg  string
	}{
		{
			name:     "allow",
			handler:  func(req *PermissionRequest) *PermissionResponse { return &PermissionResponse{Behavior: "allow"} },
			toolName: "Bash",
			input:    `{"command":"ls"}`,
			wantBeh:  "allow",
		},
		{
			name: "deny with message",
			handler: func(req *PermissionRequest) *PermissionResponse {
				return &PermissionResponse{Behavior: "deny", Message: "not allowed"}
			},
			toolName: "Bash",
			input:    `{"command":"rm -rf /"}`,
			wantBeh:  "deny",
			wantMsg:  "not allowed",
		},
		{
			name: "handler sees tool name and input",
			handler: func(req *PermissionRequest) *PermissionResponse {
				if req.ToolName != "Read" {
					return &PermissionResponse{Behavior: "deny", Message: "wrong tool: " + req.ToolName}
				}
				var input map[string]interface{}
				json.Unmarshal(req.Input, &input)
				if input["file_path"] != "/etc/passwd" {
					return &PermissionResponse{Behavior: "deny", Message: "wrong input"}
				}
				return &PermissionResponse{Behavior: "allow"}
			},
			toolName: "Read",
			input:    `{"file_path":"/etc/passwd"}`,
			wantBeh:  "allow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ln, err := NewListener(tt.handler)
			if err != nil {
				t.Fatal(err)
			}
			ln.Start()
			defer ln.Stop()

			params := &toolCallParams{
				Name: "permission_prompt",
				Arguments: toolCallInput{
					ToolName: tt.toolName,
					Input:    json.RawMessage(tt.input),
				},
			}
			result, err := forwardToParent(ln.SocketPath(), params, 1)
			if err != nil {
				t.Fatal(err)
			}

			var got map[string]interface{}
			if err := json.Unmarshal([]byte(result), &got); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}

			if got["behavior"] != tt.wantBeh {
				t.Errorf("behavior = %q, want %q", got["behavior"], tt.wantBeh)
			}

			if tt.wantBeh == "allow" {
				if got["updatedInput"] == nil {
					t.Error("allow response missing updatedInput")
				}
			}

			if tt.wantMsg != "" {
				if got["message"] != tt.wantMsg {
					t.Errorf("message = %q, want %q", got["message"], tt.wantMsg)
				}
			}
		})
	}
}

func TestAllowResponseIncludesOriginalInput(t *testing.T) {
	ln, err := NewListener(func(req *PermissionRequest) *PermissionResponse {
		return &PermissionResponse{Behavior: "allow"}
	})
	if err != nil {
		t.Fatal(err)
	}
	ln.Start()
	defer ln.Stop()

	input := `{"command":"echo hello","description":"Print hello"}`
	params := &toolCallParams{
		Name: "permission_prompt",
		Arguments: toolCallInput{
			ToolName: "Bash",
			Input:    json.RawMessage(input),
		},
	}
	result, err := forwardToParent(ln.SocketPath(), params, 1)
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]interface{}
	json.Unmarshal([]byte(result), &got)

	// updatedInput must contain the original input fields
	updated, ok := got["updatedInput"].(map[string]interface{})
	if !ok {
		t.Fatal("updatedInput is not an object")
	}
	if updated["command"] != "echo hello" {
		t.Errorf("updatedInput.command = %q, want %q", updated["command"], "echo hello")
	}
	if updated["description"] != "Print hello" {
		t.Errorf("updatedInput.description = %q, want %q", updated["description"], "Print hello")
	}
}

func TestDenyResponseRequiresMessage(t *testing.T) {
	ln, err := NewListener(func(req *PermissionRequest) *PermissionResponse {
		return &PermissionResponse{Behavior: "deny"}
	})
	if err != nil {
		t.Fatal(err)
	}
	ln.Start()
	defer ln.Stop()

	params := &toolCallParams{
		Name: "permission_prompt",
		Arguments: toolCallInput{
			ToolName: "Bash",
			Input:    json.RawMessage(`{"command":"rm -rf /"}`),
		},
	}
	result, err := forwardToParent(ln.SocketPath(), params, 1)
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]interface{}
	json.Unmarshal([]byte(result), &got)

	if got["message"] == nil || got["message"] == "" {
		t.Error("deny response must include non-empty message")
	}
}

func TestPermissionToolRef(t *testing.T) {
	ref := PermissionToolRef()
	want := "mcp__slaude_perms__permission_prompt"
	if ref != want {
		t.Errorf("PermissionToolRef() = %q, want %q", ref, want)
	}
}

func TestMCPConfigFile(t *testing.T) {
	ln, err := NewListener(func(req *PermissionRequest) *PermissionResponse {
		return &PermissionResponse{Behavior: "allow"}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Stop()

	path, err := ln.MCPConfigFile("/usr/local/bin/slaude")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	var cfg map[string]interface{}
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON in config file: %v", err)
	}

	servers, ok := cfg["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatal("mcpServers missing")
	}
	server, ok := servers["slaude_perms"].(map[string]interface{})
	if !ok {
		t.Fatal("slaude_perms server missing")
	}
	if server["type"] != "stdio" {
		t.Errorf("type = %q, want stdio", server["type"])
	}
	if server["command"] != "/usr/local/bin/slaude" {
		t.Errorf("command = %q, want /usr/local/bin/slaude", server["command"])
	}
}
