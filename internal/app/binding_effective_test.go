package app

import (
	"context"
	"testing"

	"feidex/internal/codexrpc"
	"feidex/internal/config"
	"feidex/internal/feishu"
	"feidex/internal/state"
)

func TestPrivateEffortCardAcceptsInputValueAndPersistsWorkspaceSetting(t *testing.T) {
	a, _, fc := newTestApp(t)
	a.frontendID = "xiaolongnv"
	workspaceID := a.cfg.Workspaces[0].ID
	sessionKey := "feishu:frontend:xiaolongnv:chat:private-effort"
	if err := a.State().SaveSession(&state.Session{Key: sessionKey, WorkspaceID: workspaceID, ChatID: "private-effort", ChatType: "p2p", OwnerUserID: "user-1"}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	fc.callHook = func(_ context.Context, method string, _ any, out any) error {
		if method == "model/list" {
			*out.(*codexrpc.ModelListResult) = codexrpc.ModelListResult{Data: []codexrpc.ModelListEntry{{ID: "model-a", IsDefault: true, SupportedReasoningEfforts: []codexrpc.ModelReasoningEffortEntry{{ReasoningEffort: "low"}, {ReasoningEffort: "high"}}}}}
		}
		return nil
	}
	_, err := newCardActionService(a).dispatch(&feishu.CardAction{
		ActionValue: map[string]any{"action": "model.config.select_effort", "session_key": sessionKey},
		InputValue:  "low",
		UserID:      "user-1",
		ChatID:      "private-effort",
	})
	if err != nil {
		t.Fatalf("dispatch() error = %v", err)
	}
	settings := a.BotWorkspaceSettings(workspaceID)
	if settings == nil || settings.ReasoningEffort != "low" {
		t.Fatalf("workspace reasoning effort = %#v, want low", settings)
	}
}

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

func TestEffectiveClaudeModelUsesFrontendModelBeforeGlobalClaudeModel(t *testing.T) {
	a, _, _ := newTestApp(t)
	a.frontendID = "claude-main"
	a.frontendModel = "deepseek-v4-pro"
	a.cfg.Claude.Model = "claude-opus-5"
	sess := &state.Session{WorkspaceID: "default"}
	ws := &config.Workspace{ID: "default", Cwd: t.TempDir()}
	if got := effectiveClaudeModel(a, sess, ws); got != "deepseek-v4-pro" {
		t.Fatalf("effective Claude model = %q, want frontend model", got)
	}
}

func TestP2PSessionScopeActiveForUnmaterializedCanonicalChat(t *testing.T) {
	a, _, _ := newTestApp(t)
	a.frontendID = "bot-a"
	key := "feishu:frontend:bot-a:chat:private-chat"
	if !p2pSessionScopeActive(a, key) {
		t.Fatal("expected an unmaterialized chat card to be treated as private scope")
	}
}

func TestEffectiveCodexPlanSettingsUsesWorkspaceOverrides(t *testing.T) {
	a, _, _ := newTestApp(t)
	a.frontendID = "bot-a"
	a.frontendPlanModel = "frontend-plan"
	a.frontendPlanReasoningEffort = "medium"
	a.cfg.Codex.PlanModel = "global-plan"
	a.cfg.Codex.PlanReasoningEffort = "low"
	if err := a.SaveBotWorkspaceSettings(&state.BotWorkspaceSettings{
		WorkspaceID:         "default",
		PlanModel:           "workspace-plan",
		PlanReasoningEffort: "high",
	}); err != nil {
		t.Fatal(err)
	}
	model, effort := a.EffectiveCodexPlanSettings(&state.Session{WorkspaceID: "default"})
	if model != "workspace-plan" || effort != "high" {
		t.Fatalf("plan settings = %q/%q, want workspace-plan/high", model, effort)
	}
}
