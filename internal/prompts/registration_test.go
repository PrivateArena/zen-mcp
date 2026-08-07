package prompts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"zen-mcp/internal/mcpcfg"
)

func TestLoadSkillContentDirectLookup(t *testing.T) {
	origRoot := mcpcfg.ProjectRoot
	defer func() { mcpcfg.ProjectRoot = origRoot }()

	tmpDir := t.TempDir()
	mcpcfg.ProjectRoot = tmpDir
	skillsDir := filepath.Join(tmpDir, "resources", "skills")
	os.MkdirAll(skillsDir, 0o755)

	skillContent := "# Skill Test\n\nThis is test content."
	os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(skillContent), 0o644)

	loaded, err := LoadSkillContent("test-skill")
	if err != nil {
		t.Fatalf("LoadSkillContent failed: %v", err)
	}
	if !strings.Contains(loaded, "test-skill") {
		t.Errorf("loaded content missing skill title: %s", loaded)
	}
	if !strings.Contains(loaded, "This is test content.") {
		t.Errorf("loaded content missing body: %s", loaded)
	}
}

func TestLoadSkillContentDirectorySkill(t *testing.T) {
	origRoot := mcpcfg.ProjectRoot
	defer func() { mcpcfg.ProjectRoot = origRoot }()

	tmpDir := t.TempDir()
	mcpcfg.ProjectRoot = tmpDir
	skillsDir := filepath.Join(tmpDir, "resources", "skills")
	skillDir := filepath.Join(skillsDir, "dir-skill")
	os.MkdirAll(skillDir, 0o755)

	skillContent := "# Dir Skill\n\nDirectory skill content."
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644)

	loaded, err := LoadSkillContent("dir-skill")
	if err != nil {
		t.Fatalf("LoadSkillContent failed: %v", err)
	}
	if !strings.Contains(loaded, "Dir Skill") {
		t.Errorf("loaded content missing skill title: %s", loaded)
	}
	if !strings.Contains(loaded, "Directory skill content.") {
		t.Errorf("loaded content missing body: %s", loaded)
	}
}

func TestLoadSkillContentMissingSkill(t *testing.T) {
	origRoot := mcpcfg.ProjectRoot
	defer func() { mcpcfg.ProjectRoot = origRoot }()

	tmpDir := t.TempDir()
	mcpcfg.ProjectRoot = tmpDir
	skillsDir := filepath.Join(tmpDir, "resources", "skills")
	os.MkdirAll(skillsDir, 0o755)

	_, err := LoadSkillContent("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing skill")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPromptHandlerUsesCapturedDefinition(t *testing.T) {
	origRoot := mcpcfg.ProjectRoot
	defer func() { mcpcfg.ProjectRoot = origRoot }()

	tmpDir := t.TempDir()
	mcpcfg.ProjectRoot = tmpDir
	promptsDir := filepath.Join(tmpDir, "resources", "prompts")
	os.MkdirAll(promptsDir, 0o755)

	originalYAML := `
- name: test-prompt
  description: Original description
  arguments:
    - name: text
      description: Text argument
      required: false
  template: "Original template: {{text}}"
`
	os.WriteFile(filepath.Join(promptsDir, "test-prompt.yaml"), []byte(originalYAML), 0o644)

	srv := mcpserver.NewMCPServer("test", "1.0")
	RegisterPrompts(srv, tmpDir)

	ctx := context.Background()
	reqJSON := []byte(`{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"test-prompt","arguments":{"text":"hello"}}}`)
	resp := srv.HandleMessage(ctx, reqJSON)

	if errResp, ok := resp.(mcp.JSONRPCError); ok {
		t.Fatalf("prompts/get returned error: code=%d message=%s data=%v", errResp.Error.Code, errResp.Error.Message, errResp.Error.Data)
	}
	jsonResp, ok := resp.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("unexpected response type: %T", resp)
	}
	if jsonResp.JSONRPC != "2.0" {
		t.Fatalf("unexpected response: %v", resp)
	}
	result, ok := jsonResp.Result.(mcp.GetPromptResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", jsonResp.Result)
	}
	if len(result.Messages) == 0 {
		t.Fatalf("no messages in response")
	}
	firstMsg := result.Messages[0]
	content, ok := firstMsg.Content.(mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content type: %T", firstMsg.Content)
	}
	text := content.Text
	if !strings.Contains(text, "Original template: hello") {
		t.Errorf("handler did not use captured definition, got: %s", text)
	}

	modifiedYAML := `
- name: test-prompt
  description: Modified description
  arguments:
    - name: text
      description: Text argument
      required: false
  template: "Modified template: {{text}}"
`
	os.WriteFile(filepath.Join(promptsDir, "test-prompt.yaml"), []byte(modifiedYAML), 0o644)

	resp2 := srv.HandleMessage(ctx, reqJSON)
	if errResp, ok := resp2.(mcp.JSONRPCError); ok {
		t.Fatalf("second prompts/get returned error: code=%d message=%s data=%v", errResp.Error.Code, errResp.Error.Message, errResp.Error.Data)
	}
	jsonResp2, ok := resp2.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("unexpected second response type: %T", resp2)
	}
	result2, ok := jsonResp2.Result.(mcp.GetPromptResult)
	if !ok {
		t.Fatalf("unexpected second result type: %T", jsonResp2.Result)
	}
	if len(result2.Messages) == 0 {
		t.Fatalf("no messages in second response")
	}
	firstMsg2 := result2.Messages[0]
	content2, ok := firstMsg2.Content.(mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected second content type: %T", firstMsg2.Content)
	}
	text2 := content2.Text
	if !strings.Contains(text2, "Original template: hello") {
		t.Errorf("handler reloaded from disk after modification, got: %s", text2)
	}
	if strings.Contains(text2, "Modified template") {
		t.Errorf("handler should not use modified definition, got: %s", text2)
	}
}
