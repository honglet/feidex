package state

import (
	"path/filepath"
	"testing"
)

func TestGroupPrimaryPersistScopeAndClone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.UpsertGroupPrimary(&GroupPrimary{
		ID:             " primary-a ",
		ChatID:         " chat-1 ",
		ChatType:       " GROUP ",
		OwnerBotOpenID: " bot-a ",
	}); err != nil {
		t.Fatalf("UpsertGroupPrimary() error = %v", err)
	}
	gotA := store.GetGroupPrimary("primary-a")
	if gotA == nil || gotA.ChatID != "chat-1" || gotA.ChatType != "group" || gotA.OwnerBotOpenID != "bot-a" {
		t.Fatalf("frontend-a group primary = %+v", gotA)
	}
	gotA.OwnerBotOpenID = "mutated"
	if again := store.GetGroupPrimary("primary-a"); again == nil || again.OwnerBotOpenID != "bot-a" {
		t.Fatalf("GetScopedGroupPrimary returned shared state: %+v", again)
	}

	if err := store.UpsertGroupPrimary(&GroupPrimary{
		ID:             "primary-b",
		ChatID:         "chat-1",
		ChatType:       "group",
		OwnerBotOpenID: "bot-b",
	}); err == nil {
		t.Fatal("UpsertGroupPrimary accepted a second owner record for the same chat")
	}
	if got := store.GroupPrimariesByChat("ignored", "group", "chat-1"); len(got) != 1 || got[0].OwnerBotOpenID != "bot-a" {
		t.Fatalf("GroupPrimariesByChat() = %+v, want one owner", got)
	}
}

func TestEnsureGroupPrimaryPreservesExistingStateAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	initial := &GroupPrimary{ID: " primary-group ", ChatType: " GROUP ", ChatID: " chat-1 "}
	created, err := store.EnsureGroupPrimary(initial)
	if err != nil || created == nil || created.ID != "primary-group" || created.OwnerBotOpenID != "" {
		t.Fatalf("EnsureGroupPrimary(initial) = %+v, %v", created, err)
	}
	if initial.ID != " primary-group " || initial.ChatType != " GROUP " || initial.ChatID != " chat-1 " {
		t.Fatalf("EnsureGroupPrimary mutated its input: %+v", initial)
	}
	stale := *created
	stale.OwnerBotOpenID = "bot-a"
	if got, err := store.EnsureGroupPrimary(&stale); err != nil || got == nil || *got != *created {
		t.Fatalf("EnsureGroupPrimary overwrote initialized empty owner: %+v, %v", got, err)
	}
	manual := *created
	manual.OwnerBotOpenID = "bot-b"
	if err := store.UpsertGroupPrimary(&manual); err != nil {
		t.Fatal(err)
	}
	expected := store.GetGroupPrimary(created.ID)
	got, err := store.EnsureGroupPrimary(&stale)
	if err != nil || got == nil || *got != *expected {
		t.Fatalf("EnsureGroupPrimary overwrote explicit owner: %+v, %v; want %+v", got, err, expected)
	}
	got.OwnerBotOpenID = "mutated"
	if again := store.GetGroupPrimary(created.ID); again == nil || *again != *expected {
		t.Fatalf("EnsureGroupPrimary returned shared state: %+v", again)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if persisted := reopened.GetGroupPrimary(created.ID); persisted == nil || *persisted != *expected {
		t.Fatalf("persisted primary = %+v, want %+v", persisted, expected)
	}
}
