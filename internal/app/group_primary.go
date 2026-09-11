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

type botNameProvider interface {
	BotName() string
}

type groupPrimaryAssignment struct {
	TargetBotName string
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
	ownerBotName := ""
	if botCount == 1 {
		ownerBotName = currentBotName(a)
		if ownerBotName == "" {
			return nil, fmt.Errorf("bot name is required to initialize group primary")
		}
	}
	// The group lookup can finish after another frontend has initialized the
	// group or processed /primary on. Check and insert under the shared store
	// lock so a late probe cannot reset that owner (including to an empty one).
	return a.State().EnsureGroupPrimary(&state.GroupPrimary{
		ChatID:       chatID,
		ChatType:     chatType,
		OwnerBotName: ownerBotName,
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
	owner := groupPrimaryOwnerName(a, chatType, chatID)
	self := currentBotName(a)
	return owner != "" && self != "" && owner == self
}

func groupPrimaryOwnerName(a *App, chatType, chatID string) string {
	primary := groupPrimaryForChat(a, chatType, chatID)
	if primary == nil {
		return ""
	}
	return strings.TrimSpace(primary.OwnerBotName)
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
	return currentBotName(a)
}

func groupPrimaryOwnerBotDisplayName(ownerBotName string) string {
	if name := strings.TrimSpace(ownerBotName); name != "" {
		return name
	}
	return "(未设置)"
}

func setGroupPrimaryOwner(a *App, chatType, chatID, ownerBotName string) (*state.GroupPrimary, error) {
	if a == nil {
		return nil, fmt.Errorf("app not initialized")
	}
	chatType = strings.ToLower(strings.TrimSpace(chatType))
	chatID = strings.TrimSpace(chatID)
	ownerBotName = strings.TrimSpace(ownerBotName)
	if chatType != "group" || chatID == "" {
		return nil, fmt.Errorf("group chat is required")
	}
	record := groupPrimaryForChat(a, chatType, chatID)
	if record == nil {
		record = &state.GroupPrimary{
			ID:       appstate.DefaultGroupPrimaryID(a.FrontendID(), chatType, chatID),
			ChatID:   chatID,
			ChatType: chatType,
		}
	}
	record.OwnerBotName = ownerBotName
	if err := a.State().SaveGroupPrimary(record); err != nil {
		return nil, err
	}
	updated := groupPrimaryForChat(a, chatType, chatID)
	if updated == nil {
		return nil, fmt.Errorf("group primary state for %s/%s not found after update", chatType, chatID)
	}
	return updated, nil
}

func setGroupPrimary(a *App, chatType, chatID string, enabled bool) (*state.GroupPrimary, error) {
	if !enabled {
		return nil, fmt.Errorf("clearing group primary is unsupported")
	}
	ownerBotName := currentBotName(a)
	if ownerBotName == "" {
		return nil, fmt.Errorf("bot name is required to set group primary")
	}
	return setGroupPrimaryOwner(a, chatType, chatID, ownerBotName)
}

func syncGroupPrimaryAssignment(a *App, msg *feishu.InboundMessage) (bool, error) {
	assignment, ok := groupPrimaryAssignmentFromMessage(msg)
	if !ok {
		return false, nil
	}
	selfName := currentBotName(a)
	targetsSelf := selfName != "" && selfName == assignment.TargetBotName
	if targetsSelf {
		// Primary selection is addressed by name even if transport mention-self
		// detection still has an outdated app-scoped identity.
		return true, newBindingService(a).setPrimaryOwnerForMessage(msg, selfName)
	}
	_, err := setGroupPrimaryOwner(a, msg.ChatType, msg.ChatID, assignment.TargetBotName)
	return true, err
}

func groupPrimaryAssignmentFromMessage(msg *feishu.InboundMessage) (groupPrimaryAssignment, bool) {
	if msg == nil || strings.TrimSpace(msg.ChatType) != "group" {
		return groupPrimaryAssignment{}, false
	}
	return groupPrimaryAssignmentFromTextAndMentions(msg.Text, msg.MentionedNames)
}

func groupPrimaryAssignmentFromPolicyInput(input feishu.GroupMessagePolicyInput) (groupPrimaryAssignment, bool) {
	return groupPrimaryAssignmentFromTextAndMentions(input.Text, input.MentionedNames)
}

func groupPrimaryAssignmentFromTextAndMentions(text string, mentionedNames []string) (groupPrimaryAssignment, bool) {
	targetBotName := singleMentionedName(mentionedNames)
	if targetBotName == "" {
		return groupPrimaryAssignment{}, false
	}
	if parsePrimaryOnCommandFromText(text) {
		return groupPrimaryAssignment{TargetBotName: targetBotName}, true
	}
	return groupPrimaryAssignment{}, false
}

func singleMentionedName(names []string) string {
	// Multiple targets are ambiguous; never silently choose the first mention.
	if len(names) != 1 {
		return ""
	}
	return strings.TrimSpace(names[0])
}

func parsePrimaryOnCommandFromText(text string) bool {
	fields := strings.Fields(strings.TrimSpace(text))
	for i, field := range fields {
		if strings.TrimSpace(field) != "/primary" {
			continue
		}
		if i+2 != len(fields) {
			return false
		}
		return strings.EqualFold(strings.TrimSpace(fields[i+1]), "on")
	}
	return false
}
