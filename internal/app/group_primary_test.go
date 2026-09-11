package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"feidex/internal/feishu"
	"feidex/internal/state"
)

type groupPrimaryProbeClient struct {
	FeishuClient
	probe func(context.Context, string) (int, error)
}

func (c groupPrimaryProbeClient) GetGroupBotCount(ctx context.Context, chatID string) (int, error) {
	return c.probe(ctx, chatID)
}

func TestGroupPrimaryLateInitializationPreservesManualSwitch(t *testing.T) {
	for _, botCount := range []int{1, 2} {
		t.Run(fmt.Sprintf("bot_count_%d", botCount), func(t *testing.T) {
			a, ffA, _ := newTestApp(t)
			a.frontendID = "bot-a"
			ffA.botOpenID = "bot-a-open"
			ffA.botName = "bot-a"
			b, ffB, _ := newTestApp(t)
			b.frontendID = "bot-b"
			b.store = a.store
			ffB.botOpenID = "bot-b-open"
			ffB.botName = "bot-b"
			ffB.botName = "bot-b"
			ffB.groupBotCounts = map[string]int{"chat-primary": 2}
			ffB.announcementBlocks = []feishu.AnnouncementBlock{{
				BlockID: "common-block",
				Text: strings.Join([]string{
					groupAnnouncementCommonTitle,
					groupAnnouncementField("Primary Bot", "old-remote-bot"),
					groupAnnouncementField("Marker", groupAnnouncementCommonMarker),
				}, "\n"),
			}}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			probeStarted := make(chan struct{})
			releaseProbe := make(chan struct{})
			a.feishu = groupPrimaryProbeClient{FeishuClient: a.feishu, probe: func(ctx context.Context, _ string) (int, error) {
				close(probeStarted)
				select {
				case <-releaseProbe:
					return botCount, nil
				case <-ctx.Done():
					return 0, ctx.Err()
				}
			}}
			type initResult struct {
				primary *state.GroupPrimary
				err     error
			}
			initialized := make(chan initResult, 1)
			go func() {
				primary, err := ensureGroupPrimaryInitialized(ctx, a, "group", "chat-primary")
				initialized <- initResult{primary: primary, err: err}
			}()
			select {
			case <-probeStarted:
			case <-ctx.Done():
				t.Fatal("primary initialization did not start its group probe")
			}

			b.HandleFeishuMessage(&feishu.InboundMessage{
				MessageID:        "primary-switch",
				ChatID:           "chat-primary",
				ChatType:         "group",
				UserID:           "user-1",
				Text:             "/primary on",
				MentionedOpenIDs: []string{"bot-b-open"}, MentionedNames: []string{"bot-b"},
				MentionedSelf: true,
			})
			cards := ffB.replyCardsSnapshot()
			if len(cards) != 1 || !strings.Contains(cardMarkdownContent(t, cards[0]), "已更新 primary: `on`") {
				t.Fatalf("primary switch did not acknowledge success: %+v", cards)
			}

			close(releaseProbe)
			select {
			case result := <-initialized:
				if result.err != nil || result.primary == nil || result.primary.OwnerBotName != "bot-b" {
					t.Fatalf("late initialization = %+v, %v; want bot-b to remain owner", result.primary, result.err)
				}
			case <-ctx.Done():
				t.Fatal("primary initialization did not finish")
			}
			input := feishu.GroupMessagePolicyInput{ChatID: "chat-primary", Text: "hello"}
			if shouldDeliverGroupMessageToApp(a, input) || !shouldDeliverGroupMessageToApp(b, input) {
				t.Fatal("unmentioned messages no longer route exclusively to bot-b after initialization")
			}
			if err := refreshGroupAnnouncementStatusNow(ctx, b, "chat-primary"); err != nil {
				t.Fatal(err)
			}
			updates := ffB.announcementUpdateCalls
			if len(updates) != 1 || updates[0].blockID != "common-block" || !strings.Contains(updates[0].content, groupAnnouncementField("Primary Bot", "bot-b")) {
				t.Fatalf("announcement still shows the old remote bot after the switch: %+v", updates)
			}
		})
	}
}

func TestGroupPrimaryAssignmentAcrossSharedFrontends(t *testing.T) {
	for _, targetFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("target_first_%t", targetFirst), func(t *testing.T) {
			a, ffA, _ := newTestApp(t)
			a.frontendID = "bot-a"
			ffA.botOpenID = "bot-a-open"
			ffA.botName = "bot-a"
			b, ffB, _ := newTestApp(t)
			b.frontendID = "bot-b"
			b.store = a.store
			ffB.botOpenID = "bot-b-open"
			ffB.botName = "bot-b"
			if _, err := setGroupPrimary(a, "group", "chat-primary", true); err != nil {
				t.Fatal(err)
			}
			msg := feishu.InboundMessage{
				MessageID:        "primary-switch",
				ChatID:           "chat-primary",
				ChatType:         "group",
				UserID:           "user-1",
				Text:             "@bot-b /primary on",
				MentionedOpenIDs: []string{"bot-b-open"}, MentionedNames: []string{"bot-b"},
			}
			targetMsg := msg
			targetMsg.Text = "/primary on"
			targetMsg.MentionedSelf = true
			if targetFirst {
				b.HandleFeishuMessage(&targetMsg)
				a.HandleFeishuMessage(&msg)
			} else {
				a.HandleFeishuMessage(&msg)
				b.HandleFeishuMessage(&targetMsg)
			}
			if isGroupPrimary(a, "group", "chat-primary") || !isGroupPrimary(b, "group", "chat-primary") {
				t.Fatal("shared primary did not switch from bot-a to bot-b")
			}
			if len(ffA.replyCardsSnapshot()) != 0 || len(ffB.replyCardsSnapshot()) != 1 {
				t.Fatal("only the target bot should acknowledge the primary switch")
			}
		})
	}
}

func TestGroupPrimaryUsesNamesAcrossIndependentBotApps(t *testing.T) {
	for _, targetFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("target_first_%t", targetFirst), func(t *testing.T) {
			a, fa, _ := newTestApp(t)
			b, fb, _ := newTestApp(t)
			fa.botName, fa.botOpenID = "pc-feidex", "pc-local-id"
			fb.botName, fb.botOpenID = "qnap-feidex", "qnap-local-id"
			// Independent stores model bots deployed on different machines/apps.
			for _, bot := range []*App{a, b} {
				if _, err := setGroupPrimaryOwner(bot, "group", "chat-primary", "pc-feidex"); err != nil {
					t.Fatal(err)
				}
			}
			remote := &feishu.InboundMessage{ChatID: "chat-primary", ChatType: "group", MessageID: "switch", UserID: "user-1", Text: "@qnap-feidex /primary on", MentionedNames: []string{"qnap-feidex"}, MentionedOpenIDs: []string{"qnap-id-as-seen-by-pc"}, MentionedAny: true}
			local := *remote
			local.MentionedOpenIDs = []string{"qnap-id-after-rotation"}
			local.MentionedSelf = false // Name selection must not depend on this transport flag.
			if targetFirst {
				b.HandleFeishuMessage(&local)
				a.HandleFeishuMessage(remote)
			} else {
				a.HandleFeishuMessage(remote)
				b.HandleFeishuMessage(&local)
			}
			fa.botOpenID, fb.botOpenID = "changed-pc-id", "changed-qnap-id"
			for _, bot := range []*App{a, b} {
				if got := groupPrimaryOwnerName(bot, "group", "chat-primary"); got != "qnap-feidex" {
					t.Fatalf("owner = %q", got)
				}
			}
			if isGroupPrimary(a, "group", "chat-primary") || !isGroupPrimary(b, "group", "chat-primary") {
				t.Fatal("routing depended on app-scoped IDs")
			}
			if len(fa.replyCardsSnapshot()) != 0 || len(fb.replyCardsSnapshot()) != 1 {
				t.Fatal("only the named target should acknowledge")
			}
		})
	}
}

func TestGroupPrimaryDoesNotFallBackToOpenID(t *testing.T) {
	a, ff, _ := newTestApp(t)
	ff.botName, ff.botOpenID = "", "qnap-feidex"
	if _, err := setGroupPrimary(a, "group", "chat-primary", true); err == nil {
		t.Fatal("unnamed bot claimed primary")
	}
	if _, err := setGroupPrimaryOwner(a, "group", "chat-primary", "qnap-feidex"); err != nil {
		t.Fatal(err)
	}
	if isGroupPrimary(a, "group", "chat-primary") {
		t.Fatal("OpenID was treated as a bot name")
	}
	for _, names := range [][]string{nil, {""}, {"qnap-feidex", "pc-feidex"}} {
		if _, ok := groupPrimaryAssignmentFromMessage(&feishu.InboundMessage{ChatType: "group", Text: "/primary on", MentionedNames: names, MentionedOpenIDs: []string{"qnap-feidex"}}); ok {
			t.Fatalf("ambiguous/missing names accepted: %v", names)
		}
	}
}
