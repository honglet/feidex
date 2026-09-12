package app

import (
	"strings"
	"sync"

	"feidex/internal/app/appcore"
	"feidex/internal/app/appstate"
	appbackend "feidex/internal/app/backend"
	appconvbackend "feidex/internal/app/convbackend"
	"feidex/internal/config"
	"feidex/internal/state"
)

// Feishu returns the Feishu client. Sub-packages should define narrow
// interfaces for the methods they need rather than depending on this type.
func (a *App) Feishu() FeishuClient {
	if a == nil {
		return nil
	}
	return a.feishu
}

// Config returns the application configuration.
func (a *App) Config() *config.Config {
	if a == nil {
		return nil
	}
	return a.cfg
}

// EffectiveCodexConfig returns the shared config with this frontend's Codex
// profile values overlaid. The returned config is a copy and is safe for
// display and request construction; mutations must be persisted explicitly.
func (a *App) EffectiveCodexConfig() *config.Config {
	if a == nil || a.cfg == nil {
		return nil
	}
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	copyCfg := *a.cfg
	copyCfg.Codex = a.cfg.Codex
	if strings.TrimSpace(a.frontendModel) != "" {
		copyCfg.Codex.Model = strings.TrimSpace(a.frontendModel)
	}
	if strings.TrimSpace(a.frontendReasoningEffort) != "" {
		copyCfg.Codex.ReasoningEffort = strings.TrimSpace(a.frontendReasoningEffort)
	}
	if strings.TrimSpace(a.frontendPlanModel) != "" {
		copyCfg.Codex.PlanModel = strings.TrimSpace(a.frontendPlanModel)
	}
	if strings.TrimSpace(a.frontendPlanReasoningEffort) != "" {
		copyCfg.Codex.PlanReasoningEffort = strings.TrimSpace(a.frontendPlanReasoningEffort)
	}
	if strings.TrimSpace(a.codexProfile) == "" {
		return &copyCfg
	}
	profile, err := config.LoadCodexProfile(a.codexHome, a.codexProfile)
	if err != nil {
		return &copyCfg
	}
	if strings.TrimSpace(a.frontendModel) == "" && strings.TrimSpace(profile.Model) != "" {
		copyCfg.Codex.Model = profile.Model
	}
	if strings.TrimSpace(a.frontendReasoningEffort) == "" && strings.TrimSpace(profile.ReasoningEffort) != "" {
		copyCfg.Codex.ReasoningEffort = profile.ReasoningEffort
	}
	if strings.TrimSpace(a.frontendPlanModel) == "" && strings.TrimSpace(profile.PlanModel) != "" {
		copyCfg.Codex.PlanModel = profile.PlanModel
	} else if strings.TrimSpace(a.frontendPlanModel) == "" && strings.TrimSpace(profile.Model) != "" {
		copyCfg.Codex.PlanModel = profile.Model
	}
	if strings.TrimSpace(a.frontendPlanReasoningEffort) == "" && strings.TrimSpace(profile.PlanReasoningEffort) != "" {
		copyCfg.Codex.PlanReasoningEffort = profile.PlanReasoningEffort
	}
	return &copyCfg
}

// CodexProfile returns the active frontend profile name.
func (a *App) CodexProfile() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(a.codexProfile)
}

// FrontendDefaultModel returns the frontend's configured model, including its
// Codex profile model when one is configured.
func (a *App) FrontendDefaultModel() string {
	if a == nil {
		return ""
	}
	if model := strings.TrimSpace(a.frontendModel); model != "" {
		return model
	}
	if profile, err := config.LoadCodexProfile(a.codexHome, a.codexProfile); err == nil && strings.TrimSpace(profile.Model) != "" {
		return strings.TrimSpace(profile.Model)
	}
	return strings.TrimSpace(a.frontendModel)
}

// Store returns the state store.
func (a *App) Store() *state.Store {
	if a == nil {
		return nil
	}
	return a.store
}

// Backend returns the name of the currently active backend.
func (a *App) Backend() string {
	if a == nil {
		return ""
	}
	return a.backend
}

// Claude returns the Claude core client.
func (a *App) Claude() ClaudeCore {
	if a == nil {
		return nil
	}
	return a.claude
}

// Codex returns the Codex client.
func (a *App) Codex() CodexClient {
	return getCodex(a)
}

// State returns the frontend-scoped app state store.
func (a *App) State() *appstate.Store {
	if a == nil {
		return nil
	}
	return appstate.New(a)
}

// BotProfileWorkspaceID returns the p2p BotProfile workspace, if configured.
// It is an optional appcore capability used by shared workspace resolution.
func (a *App) BotProfileWorkspaceID() string {
	if a == nil || a.State() == nil {
		return ""
	}
	if profile := a.State().BotProfile(); profile != nil {
		return strings.TrimSpace(profile.WorkspaceID)
	}
	return ""
}

// BotProfile returns the current frontend's persisted default profile.
func (a *App) BotProfile() *state.BotProfile {
	if a == nil || a.State() == nil {
		return nil
	}
	return a.State().BotProfile()
}

// SetBotProfileWorkspaceID updates the p2p BotProfile workspace. Group
// workspace selection is intentionally handled by ConversationBinding.
func (a *App) SetBotProfileWorkspaceID(workspaceID string) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if a == nil || workspaceID == "" {
		return nil
	}
	_, err := updateBotProfile(a, func(profile *state.BotProfile) { profile.WorkspaceID = workspaceID })
	return err
}

// AgentBindingsForChat returns local binding configuration for one logical
// chat. It is an optional appcore capability used by binding-aware helpers.
func (a *App) AgentBindingsForChat(chatType, chatID string) []*state.AgentBinding {
	if a == nil {
		return nil
	}
	st := a.State()
	if st == nil {
		return nil
	}
	return st.AgentBindingsForChat(chatType, chatID)
}

// BotWorkspaceSettings returns persisted settings for this frontend/workspace.
func (a *App) BotWorkspaceSettings(workspaceID string) *state.BotWorkspaceSettings {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.GetBotWorkspaceSettings(a.frontendID, workspaceID)
}

// SaveBotWorkspaceSettings persists settings for this frontend/workspace.
func (a *App) SaveBotWorkspaceSettings(settings *state.BotWorkspaceSettings) error {
	if a == nil || a.store == nil || settings == nil {
		return nil
	}
	settings.FrontendID = a.frontendID
	return a.store.UpsertBotWorkspaceSettings(settings)
}

// ConfigPath returns the filesystem path to the configuration file.
func (a *App) ConfigPath() string {
	if a == nil {
		return ""
	}
	return a.cfgPath
}

// FrontendID returns the configured frontend identifier.
func (a *App) FrontendID() string {
	if a == nil {
		return ""
	}
	return a.frontendID
}

// BackendRuntime returns the runtime facade for the currently configured backend.
func (a *App) BackendRuntime() backendRuntimeFacade {
	return backendRuntime(a)
}

// ConversationBackend returns the conversation backend facade for the active backend.
func (a *App) ConversationBackend() appconvbackend.ConversationBackendFacade {
	return conversationBackend(a)
}

// Trackers returns the per-service runtime tracker bundle.
func (a *App) Trackers() *appTrackers {
	if a == nil {
		return nil
	}
	return &a.trackers
}

// ConfigMu returns the config read-write mutex.
func (a *App) ConfigMu() *sync.RWMutex {
	if a == nil {
		return nil
	}
	return a.configMutex()
}

// FrontendConfigIndex returns the active frontend configuration index.
func (a *App) FrontendConfigIndex() int {
	if a == nil {
		return -1
	}
	return a.frontendConfigIndex
}

// SetBackend sets the runtime backend override.
func (a *App) SetBackend(backend string) {
	if a == nil {
		return
	}
	a.configMutex().Lock()
	defer a.configMutex().Unlock()
	a.backend = appcore.NormalizeRuntimeBackend(backend)
}

// BackendStateMu returns the backend state mutex.
func (a *App) BackendStateMu() *sync.Mutex {
	if a == nil {
		return nil
	}
	return &a.backendStateMu
}

// BackendSwitchMu returns the backend switch mutex.
func (a *App) BackendSwitchMu() *sync.Mutex {
	if a == nil {
		return nil
	}
	return &a.backendSwitchMu
}

// BackendSwitching returns whether a backend switch is in progress.
func (a *App) BackendSwitching() bool {
	if a == nil {
		return false
	}
	return a.backendSwitching
}

// SetBackendSwitching sets the backend switching flag.
func (a *App) SetBackendSwitching(v bool) {
	if a == nil {
		return
	}
	a.backendSwitching = v
}

// BackendSwitchTarget returns the target backend during a switch.
func (a *App) BackendSwitchTarget() string {
	if a == nil {
		return ""
	}
	return a.backendSwitchTarget
}

// SetBackendSwitchTarget sets the target backend during a switch.
func (a *App) SetBackendSwitchTarget(v string) {
	if a == nil {
		return
	}
	a.backendSwitchTarget = v
}

// DefaultWorkspaceID returns the default workspace ID.
func (a *App) DefaultWorkspaceID() string {
	return appcore.DefaultWorkspaceID(a)
}

// MaintenanceTrackers returns the maintenance tracker map, lazily initializing it.
func (a *App) MaintenanceTrackers() appbackend.TrackerMap {
	if a == nil {
		return nil
	}
	if a.trackers.maintenanceTrackers == nil {
		a.trackers.maintenanceTrackers = make(appbackend.TrackerMap)
	}
	return a.trackers.maintenanceTrackers
}
