package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"feidex/internal/app/appstate"
	"feidex/internal/feishu"
	"feidex/internal/state"
)

type botGroupAddedConfigurer interface {
	SetBotGroupAddedHandler(func(*feishu.BotGroupEvent))
}

type botOpenIDProvider interface {
	BotOpenID() string
}

type liveBotOpenIDProvider interface {
	RefreshBotOpenID() string
}

type botNameProvider interface {
	BotName() string
}

type groupPrimaryAssignment struct {
	TargetBotOpenID string
}

func configureGroupPrimaryEvents(a *App) {
	if a == nil || a.feishu == nil {
		return
	}
	configurer, ok := a.feishu.(botGroupAddedConfigurer)
	if !ok {
		return
	}
	configurer.SetBotGroupAddedHandler(func(event *feishu.BotGroupEvent) {
		handleBotGroupAdded(a, event)
	})
}

func handleBotGroupAdded(a *App, event *feishu.BotGroupEvent) {
	if a == nil || event == nil {
		return
	}
	chatID := strings.TrimSpace(event.ChatID)
	if chatID == "" {
		return
	}
	if _, err := ensureGroupPrimaryInitialized(context.Background(), a, "group", chatID); err != nil {
		slog.Warn("group primary auto init failed after bot added",
			"frontend_id", strings.TrimSpace(a.FrontendID()),
			"chat_id", chatID,
			"error", err,
		)
	}
	scheduleGroupAnnouncementStatusRefresh(a, chatID, "bot_added")
}

func ensureGroupPrimaryInitialized(ctx context.Context, a *App, chatType, chatID string) (*state.GroupPrimary, error) {
	chatType = strings.ToLower(strings.TrimSpace(chatType))
	chatID = strings.TrimSpace(chatID)
	if a == nil || chatType != "group" || chatID == "" {
		return nil, nil
	}
	if primary := groupPrimaryForChat(a, chatType, chatID); primary != nil {
		return primary, nil
	}
	if a.feishu == nil {
		return nil, fmt.Errorf("feishu client not initialized")
	}
	botCount, err := a.feishu.GetGroupBotCount(ctx, chatID)
	if err != nil {
		return nil, err
	}
	ownerBotOpenID := ""
	if botCount == 1 {
		ownerBotOpenID = currentLiveBotOpenID(a)
		if ownerBotOpenID == "" {
			return nil, fmt.Errorf("bot open_id is required to initialize group primary")
		}
	}
	// The group lookup can finish after another frontend has initialized the
	// group or processed /primary on. Check and insert under the shared store
	// lock so a late probe cannot reset that owner (including to an empty one).
	return a.State().EnsureGroupPrimary(&state.GroupPrimary{
		ChatID:         chatID,
		ChatType:       chatType,
		OwnerBotOpenID: ownerBotOpenID,
	})
}

func groupPrimaryForChat(a *App, chatType, chatID string) *state.GroupPrimary {
	if a == nil {
		return nil
	}
	return a.State().GroupPrimary(chatType, chatID)
}

func hasGroupPrimaryState(a *App, chatType, chatID string) bool {
	return groupPrimaryForChat(a, chatType, chatID) != nil
}

func isGroupPrimary(a *App, chatType, chatID string) bool {
	owner := groupPrimaryOwnerOpenID(a, chatType, chatID)
	self := currentBotOpenID(a)
	return owner != "" && self != "" && owner == self
}

func groupPrimaryOwnerOpenID(a *App, chatType, chatID string) string {
	primary := groupPrimaryForChat(a, chatType, chatID)
	if primary == nil {
		return ""
	}
	return strings.TrimSpace(primary.OwnerBotOpenID)
}

func currentBotOpenID(a *App) string {
	if a == nil || a.feishu == nil {
		return ""
	}
	provider, ok := a.feishu.(botOpenIDProvider)
	if !ok {
		return ""
	}
	return strings.TrimSpace(provider.BotOpenID())
}

func currentLiveBotOpenID(a *App) string {
	if a == nil || a.feishu == nil {
		return ""
	}
	if provider, ok := a.feishu.(liveBotOpenIDProvider); ok {
		if openID := strings.TrimSpace(provider.RefreshBotOpenID()); openID != "" {
			return openID
		}
	}
	return currentBotOpenID(a)
}

func currentBotName(a *App) string {
	if a == nil || a.feishu == nil {
		return ""
	}
	provider, ok := a.feishu.(botNameProvider)
	if !ok {
		return ""
	}
	return strings.TrimSpace(provider.BotName())
}

func currentBotDisplayName(a *App) string {
	if name := currentBotName(a); name != "" {
		return name
	}
	return currentBotOpenID(a)
}

func groupPrimaryOwnerBotDisplayName(a *App, ownerBotOpenID string) string {
	ownerBotOpenID = strings.TrimSpace(ownerBotOpenID)
	if ownerBotOpenID == "" {
		return "(未设置)"
	}
	if self := currentBotOpenID(a); self != "" && self == ownerBotOpenID {
		if name := currentBotName(a); name != "" {
			return name
		}
	}
	return ownerBotOpenID
}

func currentOrMentionedBotOpenID(a *App, msg *feishu.InboundMessage) string {
	_ = msg
	return currentLiveBotOpenID(a)
}

func setGroupPrimaryOwner(a *App, chatType, chatID, ownerBotOpenID string) (*state.GroupPrimary, error) {
	if a == nil {
		return nil, fmt.Errorf("app not initialized")
	}
	chatType = strings.ToLower(strings.TrimSpace(chatType))
	chatID = strings.TrimSpace(chatID)
	ownerBotOpenID = strings.TrimSpace(ownerBotOpenID)
	if chatType != "group" || chatID == "" {
		return nil, fmt.Errorf("group chat is required")
	}
	record := groupPrimaryForChat(a, chatType, chatID)
	previousOwner := ""
	if record != nil {
		previousOwner = strings.TrimSpace(record.OwnerBotOpenID)
	}
	if record == nil {
		record = &state.GroupPrimary{
			ID:       appstate.DefaultGroupPrimaryID(a.FrontendID(), chatType, chatID),
			ChatID:   chatID,
			ChatType: chatType,
		}
	}
	record.OwnerBotOpenID = ownerBotOpenID
	if err := a.State().SaveGroupPrimary(record); err != nil {
		return nil, err
	}
	updated := groupPrimaryForChat(a, chatType, chatID)
	if updated == nil {
		return nil, fmt.Errorf("group primary state for %s/%s not found after update", chatType, chatID)
	}
	slog.Info("group primary owner written",
		"frontend_id", strings.TrimSpace(a.FrontendID()),
		"chat_id", chatID,
		"previous_owner_bot_open_id", previousOwner,
		"owner_bot_open_id", strings.TrimSpace(updated.OwnerBotOpenID),
		"current_bot_open_id", currentBotOpenID(a),
	)
	return updated, nil
}

func setGroupPrimary(a *App, chatType, chatID string, enabled bool) (*state.GroupPrimary, error) {
	if !enabled {
		return nil, fmt.Errorf("clearing group primary is unsupported")
	}
	ownerBotOpenID := ""
	ownerBotOpenID = currentLiveBotOpenID(a)
	if ownerBotOpenID == "" {
		return nil, fmt.Errorf("bot open_id is required to set group primary")
	}
	return setGroupPrimaryOwner(a, chatType, chatID, ownerBotOpenID)
}

func syncGroupPrimaryAssignment(a *App, msg *feishu.InboundMessage) (bool, error) {
	assignment, ok := groupPrimaryAssignmentFromMessage(msg)
	if !ok {
		return false, nil
	}
	selfOpenID := currentLiveBotOpenID(a)
	if selfOpenID != "" && selfOpenID == assignment.TargetBotOpenID {
		return false, nil
	}
	// Every frontend receives the same group event. Only the frontend whose
	// live bot OpenID is mentioned may execute /primary on. Other frontends
	// consume the assignment silently and must never persist the mentioned ID:
	// that ID may be stale, belong to another app, or be a non-bot mention.
	return true, nil
}

func groupPrimaryAssignmentFromMessage(msg *feishu.InboundMessage) (groupPrimaryAssignment, bool) {
	if msg == nil || strings.TrimSpace(msg.ChatType) != "group" {
		return groupPrimaryAssignment{}, false
	}
	return groupPrimaryAssignmentFromTextAndMentions(msg.Text, msg.MentionedOpenIDs)
}

func groupPrimaryAssignmentFromTextAndMentions(text string, mentionedOpenIDs []string) (groupPrimaryAssignment, bool) {
	targetBotOpenID := firstMentionedOpenID(mentionedOpenIDs)
	if targetBotOpenID == "" {
		return groupPrimaryAssignment{}, false
	}
	if parsePrimaryOnCommandFromText(text) {
		return groupPrimaryAssignment{TargetBotOpenID: targetBotOpenID}, true
	}
	return groupPrimaryAssignment{}, false
}

func firstMentionedOpenID(mentionedOpenIDs []string) string {
	for _, openID := range mentionedOpenIDs {
		if value := strings.TrimSpace(openID); value != "" {
			return value
		}
	}
	return ""
}

func parsePrimaryOnCommandFromText(text string) bool {
	fields := strings.Fields(strings.TrimSpace(text))
	for i, field := range fields {
		if strings.TrimSpace(field) != "/primary" {
			continue
		}
		if i+1 >= len(fields) {
			return false
		}
		return strings.EqualFold(strings.TrimSpace(fields[i+1]), "on")
	}
	return false
}
