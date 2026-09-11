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
		ID:         " primary-a ",
		FrontendID: " frontend-a ",
		ChatID:     " chat-1 ",
		ChatType:   " GROUP ",
		Enabled:    true,
	}); err != nil {
		t.Fatalf("UpsertGroupPrimary() error = %v", err)
	}
	gotA := store.GetScopedGroupPrimary("frontend-a", "primary-a")
	if gotA == nil || gotA.FrontendID != "frontend-a" || gotA.ChatID != "chat-1" || gotA.ChatType != "group" || !gotA.Enabled {
		t.Fatalf("frontend-a group primary = %+v", gotA)
	}
	gotA.Enabled = false
	if again := store.GetScopedGroupPrimary("frontend-a", "primary-a"); again == nil || !again.Enabled {
		t.Fatalf("GetScopedGroupPrimary returned shared state: %+v", again)
	}

	if err := store.UpsertGroupPrimary(&GroupPrimary{
		ID:         "primary-b",
		FrontendID: "frontend-b",
		ChatID:     "chat-1",
		ChatType:   "group",
		Enabled:    true,
	}); err != nil {
		t.Fatalf("UpsertGroupPrimary rejected independent frontend state: %v", err)
	}
	if got := store.GroupPrimariesByChat("frontend-a", "group", "chat-1"); len(got) != 1 || !got[0].Enabled {
		t.Fatalf("GroupPrimariesByChat(frontend-a) = %+v", got)
	}
	if got := store.GroupPrimariesByChat("frontend-b", "group", "chat-1"); len(got) != 1 || !got[0].Enabled {
		t.Fatalf("GroupPrimariesByChat(frontend-b) = %+v", got)
	}
}

func TestEnsureGroupPrimaryPreservesExistingStateAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	initial := &GroupPrimary{ID: " primary-group ", FrontendID: " frontend-a ", ChatType: " GROUP ", ChatID: " chat-1 "}
	created, err := store.EnsureGroupPrimary(initial)
	if err != nil || created == nil || created.ID != "primary-group" || created.FrontendID != "frontend-a" || created.Enabled {
		t.Fatalf("EnsureGroupPrimary(initial) = %+v, %v", created, err)
	}
	if initial.ID != " primary-group " || initial.ChatType != " GROUP " || initial.ChatID != " chat-1 " {
		t.Fatalf("EnsureGroupPrimary mutated its input: %+v", initial)
	}
	stale := *created
	stale.Enabled = true
	if got, err := store.EnsureGroupPrimary(&stale); err != nil || got == nil || *got != *created {
		t.Fatalf("EnsureGroupPrimary overwrote initialized state: %+v, %v", got, err)
	}
	manual := *created
	manual.Enabled = true
	if err := store.UpsertGroupPrimary(&manual); err != nil {
		t.Fatal(err)
	}
	expected := store.GetGroupPrimary(created.ID)
	got, err := store.EnsureGroupPrimary(&stale)
	if err != nil || got == nil || *got != *expected {
		t.Fatalf("EnsureGroupPrimary overwrote explicit owner: %+v, %v; want %+v", got, err, expected)
	}
	got.Enabled = false
	if again := store.GetScopedGroupPrimary("frontend-a", created.ID); again == nil || *again != *expected {
		t.Fatalf("EnsureGroupPrimary returned shared state: %+v", again)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if persisted := reopened.GetScopedGroupPrimary("frontend-a", created.ID); persisted == nil || *persisted != *expected {
		t.Fatalf("persisted primary = %+v, want %+v", persisted, expected)
	}
}
