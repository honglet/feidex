package appstate

import (
	"strings"

	"feidex/internal/state"
)

// GroupPrimary returns this Feidex instance's primary owner setting for one group.
func (s *Store) GroupPrimary(chatType, chatID string) *state.GroupPrimary {
	if s == nil || s.Store == nil {
		return nil
	}
	return s.Store.GetGroupPrimary(DefaultGroupPrimaryID(s.FrontendID, chatType, chatID))
}

// GroupPrimariesForChat returns this instance's primary records for one chat.
func (s *Store) GroupPrimariesForChat(chatType, chatID string) []*state.GroupPrimary {
	if s == nil || s.Store == nil {
		return nil
	}
	primary := s.GroupPrimary(chatType, chatID)
	if primary == nil {
		return nil
	}
	return []*state.GroupPrimary{primary}
}

// SaveGroupPrimary persists primary owner state for this Feidex instance.
func (s *Store) SaveGroupPrimary(primary *state.GroupPrimary) error {
	if s == nil || s.Store == nil {
		return nil
	}
	return s.Store.UpsertGroupPrimary(s.groupPrimaryForSave(primary))
}

// EnsureGroupPrimary initializes a group's shared owner without overwriting an
// existing record from another frontend or an explicit primary command.
func (s *Store) EnsureGroupPrimary(primary *state.GroupPrimary) (*state.GroupPrimary, error) {
	if s == nil || s.Store == nil {
		return nil, nil
	}
	return s.Store.EnsureGroupPrimary(s.groupPrimaryForSave(primary))
}

// DefaultGroupPrimaryID is the stable state key for one group chat.
func DefaultGroupPrimaryID(frontendID, chatType, chatID string) string {
	_ = frontendID
	return "primary_" + sanitizeGroupPrimaryIDPart(chatType) + "_" + sanitizeGroupPrimaryIDPart(chatID)
}

func (s *Store) groupPrimaryForSave(primary *state.GroupPrimary) *state.GroupPrimary {
	if primary == nil {
		return nil
	}
	cp := *primary
	if strings.TrimSpace(cp.ID) == "" {
		cp.ID = DefaultGroupPrimaryID(s.FrontendID, cp.ChatType, cp.ChatID)
	}
	return &cp
}

func sanitizeGroupPrimaryIDPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}
