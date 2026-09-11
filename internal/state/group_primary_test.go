package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGroupPrimaryPersistScopeAndClone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.UpsertGroupPrimary(&GroupPrimary{
		ID:           " primary-a ",
		ChatID:       " chat-1 ",
		ChatType:     " GROUP ",
		OwnerBotName: " bot-a ",
	}); err != nil {
		t.Fatalf("UpsertGroupPrimary() error = %v", err)
	}
	gotA := store.GetGroupPrimary("primary-a")
	if gotA == nil || gotA.ChatID != "chat-1" || gotA.ChatType != "group" || gotA.OwnerBotName != "bot-a" {
		t.Fatalf("frontend-a group primary = %+v", gotA)
	}
	gotA.OwnerBotName = "mutated"
	if again := store.GetGroupPrimary("primary-a"); again == nil || again.OwnerBotName != "bot-a" {
		t.Fatalf("GetScopedGroupPrimary returned shared state: %+v", again)
	}

	if err := store.UpsertGroupPrimary(&GroupPrimary{
		ID:           "primary-b",
		ChatID:       "chat-1",
		ChatType:     "group",
		OwnerBotName: "bot-b",
	}); err == nil {
		t.Fatal("UpsertGroupPrimary accepted a second owner record for the same chat")
	}
	if got := store.GroupPrimariesByChat("ignored", "group", "chat-1"); len(got) != 1 || got[0].OwnerBotName != "bot-a" {
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
	if err != nil || created == nil || created.ID != "primary-group" || created.OwnerBotName != "" {
		t.Fatalf("EnsureGroupPrimary(initial) = %+v, %v", created, err)
	}
	if initial.ID != " primary-group " || initial.ChatType != " GROUP " || initial.ChatID != " chat-1 " {
		t.Fatalf("EnsureGroupPrimary mutated its input: %+v", initial)
	}
	stale := *created
	stale.OwnerBotName = "bot-a"
	if got, err := store.EnsureGroupPrimary(&stale); err != nil || got == nil || *got != *created {
		t.Fatalf("EnsureGroupPrimary overwrote initialized empty owner: %+v, %v", got, err)
	}
	manual := *created
	manual.OwnerBotName = "bot-b"
	if err := store.UpsertGroupPrimary(&manual); err != nil {
		t.Fatal(err)
	}
	expected := store.GetGroupPrimary(created.ID)
	got, err := store.EnsureGroupPrimary(&stale)
	if err != nil || got == nil || *got != *expected {
		t.Fatalf("EnsureGroupPrimary overwrote explicit owner: %+v, %v; want %+v", got, err, expected)
	}
	got.OwnerBotName = "mutated"
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

func TestGroupOwnershipMigrationDropsOpenIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	raw := `{"version":10,"group_primaries":{"primary_group_chat-1":{"id":"primary_group_chat-1","chat_id":"chat-1","chat_type":"group","owner_bot_open_id":"unstable-owner"}},"group_announcement_blocks":{"block-record":{"id":"block-record","frontend_id":"default","chat_id":"chat-1","chat_type":"group","bot_open_id":"unstable-bot","block_id":"existing-block","marker":"feidex-status-region:qnap-feidex:unstable-bot"}}}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if p := store.GetGroupPrimary("primary_group_chat-1"); p == nil || p.OwnerBotName != "" {
		t.Fatalf("legacy identity interpreted as name: %+v", p)
	}
	blocks := store.AllGroupAnnouncementBlocks()
	if len(blocks) != 1 || blocks[0].BlockID != "existing-block" || blocks[0].Marker != "" {
		t.Fatalf("block migration = %+v", blocks)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "open_id") || strings.Contains(string(data), "unstable-") {
		t.Fatalf("legacy IDs persisted: %s", data)
	}
	if err := store.UpsertGroupPrimary(&GroupPrimary{ID: "primary_group_chat-1", ChatID: "chat-1", ChatType: "group", OwnerBotName: "qnap-feidex"}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.GetGroupPrimary("primary_group_chat-1").OwnerBotName != "qnap-feidex" {
		t.Fatal("name not retained across restart")
	}
}
