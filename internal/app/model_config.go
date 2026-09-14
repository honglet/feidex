package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"feidex/internal/app/modelconfig"
	"feidex/internal/codexrpc"
	"feidex/internal/config"
	"feidex/internal/feishu"
	"feidex/internal/state"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
)

type modelConfigService struct {
	app   *App
	inner modelconfig.ModelConfigService
}

func newModelConfigService(app *App) modelConfigService {
	return modelConfigService{
		app: app,
		inner: modelconfig.ModelConfigService{
			GetConfig:      func() *config.Config { return app.cfg },
			GetCodexConfig: func() *config.Config { return app.EffectiveCodexConfig() },
			GetCfgPath:     func() string { return app.cfgPath },
			GetConfigMu:    func() *sync.RWMutex { return &app.configMu },
			UpdateCodexConfig: func(mutate func(*config.CodexConfig), result codexrpc.ModelListResult) error {
				return updateFrontendCodexConfig(app, mutate, result)
			},
			ReplyText: func(ctx context.Context, msgID string, text string, replyInThread bool) error {
				return app.feishu.ReplyText(ctx, msgID, text, replyInThread)
			},
			ReplyCard: func(ctx context.Context, msgID string, card map[string]any, replyInThread bool) (string, error) {
				return app.feishu.ReplyCard(ctx, msgID, card, replyInThread)
			},
			UpdateClaudeConfig: func(cfg config.ClaudeConfig) {
				if app.claude != nil {
					app.claude.UpdateConfig(cfg)
				}
			},
			ClaudeSetModel: func(ctx context.Context, sessionKey, model string) (bool, error) {
				if app.claude == nil {
					return false, nil
				}
				return app.claude.SetModel(ctx, sessionKey, model)
			},
			ClaudeSetEffort: func(ctx context.Context, sessionKey, effort string) (bool, error) {
				if app.claude == nil {
					return false, nil
				}
				return app.claude.SetEffort(ctx, sessionKey, effort)
			},
			IsClaudeAvailable: func() bool {
				return app.claude != nil
			},
			RequireCodexClient: func() (modelconfig.CodexClient, error) {
				return requireCodexClient(app)
			},
			MakeSessionKey: func(msg *feishu.InboundMessage) string {
				return makeSessionKey(app, msg)
			},
			NormalizeSessionKey: func(sessionKey string) string {
				return normalizeSessionKey(app, sessionKey)
			},
			SessionBelongsToFrontend: func(sessionKey string) bool {
				return sessionBelongsToFrontend(app, sessionKey)
			},
			ReplyInThreadEnabled: func(chatType string) bool {
				return replyInThreadEnabled(app, chatType)
			},
			CompleteGlobalModelSet: func(action *feishu.CardAction, modelID string) (*callback.CardActionTriggerResponse, error) {
				return newBackendConfigurationService(app).completeGlobalModelSet(action, modelID)
			},
			CompleteGlobalReasoningEffortSet: func(action *feishu.CardAction, effort string) (*callback.CardActionTriggerResponse, error) {
				return newBackendConfigurationService(app).completeGlobalReasoningEffortSet(action, effort)
			},
			HandleBackendModelCommand: func(msg *feishu.InboundMessage, args []string) error {
				return newBackendConfigurationService(app).handleBackendModelCommand(msg, args)
			},
			FormatMenuBody: menuCardBody,
			FrontendIdleBlockedReason: func() string {
				return frontendIdleBlockedReason(app)
			},
			FrontendIdleBlockedReasonIgnoringCurrentMessage: func() string {
				return frontendIdleBlockedReasonIgnoringCurrentMessage(app)
			},
			ReplyCommandActionResponse: func(msg *feishu.InboundMessage, resp *callback.CardActionTriggerResponse) error {
				return replyCommandActionResponse(app, msg, resp)
			},
		},
	}
}

func updateFrontendCodexConfig(app *App, mutate func(*config.CodexConfig), result codexrpc.ModelListResult) error {
	if app == nil || app.cfg == nil {
		return fmt.Errorf("nil config")
	}
	effective := app.EffectiveCodexConfig()
	if effective == nil {
		return fmt.Errorf("nil config")
	}
	updated := effective.Codex
	if mutate != nil {
		mutate(&updated)
	}
	updated.Model = strings.TrimSpace(updated.Model)
	updated.ReasoningEffort = strings.TrimSpace(updated.ReasoningEffort)
	updated.PlanModel = strings.TrimSpace(updated.PlanModel)
	updated.PlanReasoningEffort = strings.TrimSpace(updated.PlanReasoningEffort)
	selectedModel := modelconfig.FindModelEntry(result, updated.Model)
	if !modelconfig.ModelSupportsEffort(selectedModel, updated.ReasoningEffort) {
		updated.ReasoningEffort = ""
	}
	selectedPlanModel, _ := modelconfig.EffectivePlanConfiguredModelAndEffort(&config.Config{Codex: updated}, result, nil)
	if !modelconfig.ModelSupportsEffort(selectedPlanModel, updated.PlanReasoningEffort) {
		updated.PlanReasoningEffort = ""
	}
	if strings.TrimSpace(app.CodexProfile()) == "" {
		app.configMu.Lock()
		defer app.configMu.Unlock()
		app.cfg.Codex = updated
		if err := app.cfg.Normalize(filepath.Dir(app.cfgPath)); err != nil {
			return err
		}
		return config.Save(app.cfgPath, app.cfg)
	}
	return config.UpdateCodexProfile(app.codexHome, app.codexProfile, func(profile *config.CodexConfig) {
		profile.Model = updated.Model
		profile.ReasoningEffort = updated.ReasoningEffort
		profile.PlanModel = updated.PlanModel
		profile.PlanReasoningEffort = updated.PlanReasoningEffort
	})
}

func (s modelConfigService) fetchModelList(ctx context.Context) (codexrpc.ModelListResult, error) {
	return s.inner.FetchModelList(ctx)
}

func (s modelConfigService) fetchPlanCollaborationModePreset(ctx context.Context) (*codexrpc.CollaborationModeMask, error) {
	return s.inner.FetchPlanCollaborationModePreset(ctx)
}

func (s modelConfigService) renderModelConfigCard(result codexrpc.ModelListResult, planPreset *codexrpc.CollaborationModeMask, sessionKey, menuAction string) map[string]any {
	return s.inner.RenderModelConfigCard(result, planPreset, sessionKey, menuAction)
}

func (s modelConfigService) updateGlobalModelConfig(mutate func(*config.CodexConfig), result codexrpc.ModelListResult) error {
	return s.inner.UpdateGlobalModelConfig(mutate, result)
}

func (s modelConfigService) completeCodexPlanModelSet(action *feishu.CardAction, modelID string) (*callback.CardActionTriggerResponse, error) {
	return s.inner.CompleteCodexPlanModelSet(action, modelID)
}

func (s modelConfigService) completeCodexPlanReasoningEffortSet(action *feishu.CardAction, reasoningEffort string) (*callback.CardActionTriggerResponse, error) {
	return s.inner.CompleteCodexPlanReasoningEffortSet(action, reasoningEffort)
}

func (s modelConfigService) completeBotWorkspacePlanModelSet(action *feishu.CardAction, modelID string) (*callback.CardActionTriggerResponse, error) {
	return completeBotWorkspacePlanModelSet(s.app, action, modelID)
}

func (s modelConfigService) completeBotWorkspacePlanReasoningEffortSet(action *feishu.CardAction, effort string) (*callback.CardActionTriggerResponse, error) {
	return completeBotWorkspacePlanReasoningEffortSet(s.app, action, effort)
}

func completeBotWorkspaceSession(a *App, action *feishu.CardAction, raw string) (string, *state.Session, *config.Workspace) {
	key := actionSessionKey(action)
	msg := commandMessageFromAction(a, action, key, raw)
	return currentWorkspaceForMessage(a, msg)
}

func completeBotWorkspacePlanModelSet(a *App, action *feishu.CardAction, modelID string) (*callback.CardActionTriggerResponse, error) {
	key, sess, ws := completeBotWorkspaceSession(a, action, "/model plan")
	if ws == nil && sess != nil {
		ws = config.FindWorkspace(a.cfg, sess.WorkspaceID)
	}
	if ws == nil {
		return nil, fmt.Errorf("当前 workspace 不可用")
	}
	modelID = clearableArg(modelID)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := newModelConfigService(a).fetchModelList(ctx)
	if err != nil {
		return nil, err
	}
	if modelID != "" && modelconfig.LookupModelEntry(result, modelID) == nil {
		return &callback.CardActionTriggerResponse{Toast: &callback.Toast{Type: "warning", Content: "未找到 model: " + modelID}}, nil
	}
	settings := a.BotWorkspaceSettings(ws.ID)
	if settings == nil {
		settings = &state.BotWorkspaceSettings{WorkspaceID: ws.ID}
	}
	settings.PlanModel = modelID
	if err := a.SaveBotWorkspaceSettings(settings); err != nil {
		return nil, err
	}
	return &callback.CardActionTriggerResponse{Toast: &callback.Toast{Type: "success", Content: "已更新当前 Bot 在该 workspace 的 Plan 模型"}, Card: rawCard(newModelConfigService(a).renderModelConfigCard(result, nil, key, actionStringValue(action, "menu_action")))}, nil
}

func completeBotWorkspacePlanReasoningEffortSet(a *App, action *feishu.CardAction, effort string) (*callback.CardActionTriggerResponse, error) {
	_, sess, ws := completeBotWorkspaceSession(a, action, "/model plan effort")
	if ws == nil && sess != nil {
		ws = config.FindWorkspace(a.cfg, sess.WorkspaceID)
	}
	if ws == nil {
		return nil, fmt.Errorf("当前 workspace 不可用")
	}
	effort = clearableArg(effort)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := newModelConfigService(a).fetchModelList(ctx)
	if err != nil {
		return nil, err
	}
	settings := a.BotWorkspaceSettings(ws.ID)
	if settings == nil {
		settings = &state.BotWorkspaceSettings{WorkspaceID: ws.ID}
	}
	planModel := strings.TrimSpace(settings.PlanModel)
	if planModel == "" {
		planModel = strings.TrimSpace(settings.Model)
	}
	if planModel == "" {
		if cfg := a.EffectiveCodexConfig(); cfg != nil {
			planModel = strings.TrimSpace(cfg.Codex.PlanModel)
		}
	}
	if planModel != "" && effort != "" {
		entry := modelconfig.LookupModelEntry(result, planModel)
		if !modelconfig.ModelSupportsEffort(entry, effort) {
			return &callback.CardActionTriggerResponse{Toast: &callback.Toast{Type: "warning", Content: "Plan 模式模型不支持这个推理强度"}}, nil
		}
	}
	settings.PlanReasoningEffort = effort
	if err := a.SaveBotWorkspaceSettings(settings); err != nil {
		return nil, err
	}
	return &callback.CardActionTriggerResponse{Toast: &callback.Toast{Type: "success", Content: "已更新当前 Bot 在该 workspace 的 Plan 推理强度"}}, nil
}

func (s modelConfigService) commandCodexModel(msg *feishu.InboundMessage, args []string) error {
	return s.inner.CommandCodexModel(msg, args)
}
