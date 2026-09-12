package app

import (
	"testing"

	"feidex/internal/config"
	"feidex/internal/state"
)

func TestEffectiveModelUsesSessionBindingThenBotDefault(t *testing.T) {
	a, _, _ := newTestApp(t)
	a.frontendID = "bot-a"
	a.cfg.Codex.Model = "gpt-5-bot"
	a.cfg.Claude.Model = "sonnet-bot"
	ws := &config.Workspace{ID: "default", Cwd: t.TempDir()}
	sess := &state.Session{
		Key:         "feishu:frontend:bot-a:chat:chat-model-priority",
		BindingID:   "binding-model-priority",
		WorkspaceID: "default",
		ChatID:      "chat-model-priority",
		ChatType:    "group",
	}
	if err := a.State().SaveAgentBinding(&state.AgentBinding{
		ID:            "binding-model-priority",
		FrontendID:    "bot-a",
		ChatID:        "chat-model-priority",
		ChatType:      "group",
		WorkspaceID:   "default",
		ModelOverride: "gpt-5-binding",
		Status:        state.AgentBindingStatusActive.String(),
	}); err != nil {
		t.Fatalf("SaveAgentBinding() error = %v", err)
	}

	if got := effectiveCodexModel(a, sess, ws); got != "gpt-5-binding" {
		t.Fatalf("effectiveCodexModel() = %q, want binding override", got)
	}
	if got := effectiveClaudeModel(a, sess, ws); got != "gpt-5-binding" {
		t.Fatalf("effectiveClaudeModel() = %q, want binding override", got)
	}

	sess.ModelOverride = "gpt-5-session"
	if got := effectiveCodexModel(a, sess, ws); got != "gpt-5-session" {
		t.Fatalf("effectiveCodexModel() = %q, want session override", got)
	}
	if got := effectiveClaudeModel(a, sess, ws); got != "gpt-5-session" {
		t.Fatalf("effectiveClaudeModel() = %q, want session override", got)
	}

	sess.ModelOverride = ""
	if err := a.State().SaveAgentBinding(&state.AgentBinding{
		ID:          "binding-model-priority",
		FrontendID:  "bot-a",
		ChatID:      "chat-model-priority",
		ChatType:    "group",
		WorkspaceID: "default",
		Status:      state.AgentBindingStatusActive.String(),
	}); err != nil {
		t.Fatalf("SaveAgentBinding(clear) error = %v", err)
	}
	if got := effectiveCodexModel(a, sess, ws); got != "gpt-5-bot" {
		t.Fatalf("effectiveCodexModel() = %q, want bot default", got)
	}
	if got := effectiveClaudeModel(a, sess, ws); got != "sonnet-bot" {
		t.Fatalf("effectiveClaudeModel() = %q, want bot default", got)
	}
}

func TestEffectiveCodexModelUsesBotWorkspaceSettingBeforeProfile(t *testing.T) {
	a, _, _ := newTestApp(t)
	a.frontendID = "bot-a"
	a.cfg.Codex.Model = "global"
	ws := &config.Workspace{ID: "shared", Cwd: t.TempDir()}
	if err := a.SaveBotWorkspaceSettings(&state.BotWorkspaceSettings{WorkspaceID: ws.ID, Model: "workspace-model"}); err != nil {
		t.Fatal(err)
	}
	if err := a.State().SaveBotProfile(&state.BotProfile{FrontendID: "bot-a", Model: "profile-model"}); err != nil {
		t.Fatal(err)
	}
	sess := &state.Session{WorkspaceID: ws.ID}
	if got := effectiveCodexModel(a, sess, ws); got != "workspace-model" {
		t.Fatalf("effective model = %q, want workspace-model", got)
	}
	sess.ModelOverride = "session-model"
	if got := effectiveCodexModel(a, sess, ws); got != "session-model" {
		t.Fatalf("effective model = %q, want session-model", got)
	}
}
