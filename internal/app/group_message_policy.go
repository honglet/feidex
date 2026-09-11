package app

import (
	"strings"

	"feidex/internal/feishu"
)

type groupMessagePolicyConfigurer interface {
	SetGroupMessagePolicy(feishu.GroupMessagePolicy)
}

func configureGroupMessagePolicy(a *App) {
	if a == nil || a.feishu == nil {
		return
	}
	configurer, ok := a.feishu.(groupMessagePolicyConfigurer)
	if !ok {
		return
	}
	configurer.SetGroupMessagePolicy(func(input feishu.GroupMessagePolicyInput) bool {
		return shouldDeliverGroupMessageToApp(a, input)
	})
}

func shouldDeliverGroupMessageToApp(a *App, input feishu.GroupMessagePolicyInput) bool {
	mentionedAny := input.MentionedAny || len(input.MentionedOpenIDs) > 0
	mentionedSelf := messageMentionsCurrentBot(a, input.MentionedOpenIDs, input.MentionedSelf)
	if shouldAcceptGroupMessage(a, input.ChatID, input.RootMessageID, input.ParentMessageID, mentionedSelf, mentionedAny) {
		return true
	}
	return shouldProbeGroupPrimaryForMessage(a, input.ChatID, input.RootMessageID, input.ParentMessageID, mentionedSelf, mentionedAny)
}

func messageMentionsCurrentBot(a *App, mentionedOpenIDs []string, fallback bool) bool {
	selfOpenID := currentBotOpenID(a)
	if selfOpenID == "" {
		return fallback
	}
	if len(mentionedOpenIDs) == 0 {
		return fallback
	}
	for _, mentionedOpenID := range mentionedOpenIDs {
		if strings.TrimSpace(mentionedOpenID) == selfOpenID {
			return true
		}
	}
	return false
}

func shouldProbeGroupPrimaryForMessage(a *App, chatID, rootMessageID, parentMessageID string, mentionedSelf, mentionedAny bool) bool {
	if a == nil || hasGroupPrimaryState(a, "group", chatID) {
		return false
	}
	if mentionedSelf {
		return true
	}
	if mentionedAny {
		return false
	}
	return strings.TrimSpace(rootMessageID) == "" && strings.TrimSpace(parentMessageID) == ""
}

func shouldAcceptGroupMessage(a *App, chatID, rootMessageID, parentMessageID string, mentionedSelf, mentionedAny bool) bool {
	if a == nil {
		return false
	}
	if mentionedSelf {
		return true
	}
	// An explicit mention of another person or bot must not fall through to the
	// local primary frontend.
	if mentionedAny {
		return false
	}
	if rootMessageID != "" || parentMessageID != "" {
		// Message links are frontend-scoped, so a local link proves that this
		// bot owns the reply chain. A missing link is intentionally ignored.
		if hasLocalGroupMessageLink(a, rootMessageID, parentMessageID) {
			return true
		}
		return false
	}
	return isGroupPrimary(a, "group", chatID)
}

func hasLocalGroupMessageLink(a *App, messageIDs ...string) bool {
	if a == nil {
		return false
	}
	for _, messageID := range messageIDs {
		if strings.TrimSpace(messageID) == "" {
			continue
		}
		if link := a.State().MessageLink(messageID); link != nil {
			return true
		}
	}
	return false
}
