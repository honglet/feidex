package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeFillsDefaultsAndResolvesPaths(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "workspace")
	cfg := &Config{
		DataDir: "data",
		Log: LogConfig{
			Level: " warning ",
		},
		Codex: CodexConfig{
			AppServerDir:        "./codex-home",
			Model:               " gpt-5 ",
			ReasoningEffort:     " high ",
			PlanModel:           " gpt-5-plan ",
			PlanReasoningEffort: " low ",
		},
		Workspaces: []Workspace{
			{
				ID:  " default ",
				Cwd: "./repo",
			},
		},
	}

	if err := cfg.Normalize(baseDir); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if cfg.Codex.Command != "codex" {
		t.Fatalf("Codex.Command = %q, want codex", cfg.Codex.Command)
	}
	if cfg.Codex.Transport != "stdio" {
		t.Fatalf("Codex.Transport = %q, want stdio", cfg.Codex.Transport)
	}
	if cfg.Codex.Model != "gpt-5" || cfg.Codex.ReasoningEffort != "high" || cfg.Codex.PlanModel != "gpt-5-plan" || cfg.Codex.PlanReasoningEffort != "low" {
		t.Fatalf("unexpected codex trimming result: %+v", cfg.Codex)
	}
	if !filepath.IsAbs(cfg.Codex.AppServerDir) || !strings.HasSuffix(cfg.Codex.AppServerDir, filepath.Join("workspace", "codex-home")) {
		t.Fatalf("Codex.AppServerDir = %q, want absolute path", cfg.Codex.AppServerDir)
	}
	if cfg.Log.Level != "warn" {
		t.Fatalf("Log.Level = %q, want warn", cfg.Log.Level)
	}
	if cfg.Feishu.Quiet != QuietModeProgress {
		t.Fatalf("Feishu.Quiet = %q, want progress", cfg.Feishu.Quiet)
	}
	if cfg.Workspaces[0].ID != "default" {
		t.Fatalf("Workspace.ID = %q, want default", cfg.Workspaces[0].ID)
	}
	if cfg.Workspaces[0].Name != "default" {
		t.Fatalf("Workspace.Name = %q, want default", cfg.Workspaces[0].Name)
	}
	if cfg.Workspaces[0].ApprovalPolicy != "on-request" || cfg.Workspaces[0].SandboxMode != "workspace-write" {
		t.Fatalf("unexpected workspace defaults: %+v", cfg.Workspaces[0])
	}
	if !filepath.IsAbs(cfg.Workspaces[0].Cwd) || !strings.HasSuffix(cfg.Workspaces[0].Cwd, filepath.Join("workspace", "repo")) {
		t.Fatalf("Workspace.Cwd = %q, want absolute workspace path", cfg.Workspaces[0].Cwd)
	}
	if !filepath.IsAbs(cfg.DataDir) || !strings.HasSuffix(cfg.DataDir, filepath.Join("workspace", "data")) {
		t.Fatalf("DataDir = %q, want absolute data path", cfg.DataDir)
	}
}

func TestCodexProfileLoadAndUpdatePreservesUnrelatedKeys(t *testing.T) {
	home := t.TempDir()
	profilePath := filepath.Join(home, "xiaolongnv.config.toml")
	if err := os.WriteFile(profilePath, []byte("model = \"gpt-6-astra\"\nmodel_reasoning_effort = \"high\"\ncustom_key = \"keep\"\n"), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	profile, err := LoadCodexProfile(home, "xiaolongnv")
	if err != nil {
		t.Fatalf("LoadCodexProfile: %v", err)
	}
	if profile.Model != "gpt-6-astra" || profile.ReasoningEffort != "high" {
		t.Fatalf("loaded profile = %+v", profile)
	}
	if err := UpdateCodexProfile(home, "xiaolongnv", func(cfg *CodexConfig) {
		cfg.Model = "gpt-6-astra-v2"
	}); err != nil {
		t.Fatalf("UpdateCodexProfile: %v", err)
	}
	content, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, `model = "gpt-6-astra-v2"`) || !strings.Contains(text, `custom_key = "keep"`) {
		t.Fatalf("updated profile lost expected values: %s", text)
	}
}

func TestNormalizeClaudeEffort(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty", input: "", want: ""},
		{name: "default", input: "default", want: ""},
		{name: "auto", input: "auto", want: ""},
		{name: "trimmed", input: " High ", want: "high"},
		{name: "max", input: "max", want: "max"},
		{name: "invalid", input: "ultra", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeClaudeEffort(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("NormalizeClaudeEffort() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeClaudeEffort() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeClaudeEffort() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeRejectsClaudeAutoPermissionMode(t *testing.T) {
	cases := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "global auto",
			cfg: &Config{
				Claude:     ClaudeConfig{PermissionMode: "auto"},
				Workspaces: []Workspace{{ID: "default", Cwd: "."}},
			},
		},
		{
			name: "workspace auto",
			cfg: &Config{
				Workspaces: []Workspace{{ID: "default", Cwd: ".", ClaudePermissionMode: "auto"}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Normalize(t.TempDir()); err == nil {
				t.Fatal("expected Normalize() to reject auto permission mode")
			}
		})
	}
}

func TestNormalizeRejectsInvalidWorkspaceConfigurations(t *testing.T) {
	cases := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "missing workspace",
			cfg:  &Config{},
		},
		{
			name: "missing id",
			cfg: &Config{
				Workspaces: []Workspace{{Cwd: "."}},
			},
		},
		{
			name: "missing cwd",
			cfg: &Config{
				Workspaces: []Workspace{{ID: "default"}},
			},
		},
		{
			name: "duplicate ids",
			cfg: &Config{
				Workspaces: []Workspace{
					{ID: "dup", Cwd: "."},
					{ID: "dup", Cwd: "./other"},
				},
			},
		},
		{
			name: "removed websocket transport",
			cfg: &Config{
				Codex:      CodexConfig{Transport: "ws"},
				Workspaces: []Workspace{{ID: "default", Cwd: "."}},
			},
		},
		{
			name: "removed websocket url",
			cfg: &Config{
				Codex:      CodexConfig{WSURL: "wss://example.test/ws"},
				Workspaces: []Workspace{{ID: "default", Cwd: "."}},
			},
		},
		{
			name: "claude bypass without dangerous flag",
			cfg: &Config{
				Claude:     ClaudeConfig{PermissionMode: "bypassPermissions"},
				Workspaces: []Workspace{{ID: "default", Cwd: "."}},
			},
		},
		{
			name: "workspace claude bypass without dangerous flag",
			cfg: &Config{
				Workspaces: []Workspace{{ID: "default", Cwd: ".", ClaudePermissionMode: "bypassPermissions"}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Normalize(t.TempDir()); err == nil {
				t.Fatal("expected Normalize() to return an error")
			}
		})
	}
}

func TestSaveLoadAndFindWorkspaceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg := &Config{
		DataDir: ".feidex-data",
		Log: LogConfig{
			Level: "info",
		},
		Feishu: FeishuConfig{
			Quiet: QuietModeNormal,
		},
		Codex: CodexConfig{
			Command:   "codex",
			Transport: "stdio",
		},
		Workspaces: []Workspace{
			{ID: "default", Name: "Default", Cwd: ".", ApprovalPolicy: "on-request", SandboxMode: "workspace-write"},
			{ID: "repo", Name: "Repo", Cwd: "./repo", ApprovalPolicy: "never", SandboxMode: "danger-full-access"},
		},
	}

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.DataDir == "" || !filepath.IsAbs(loaded.DataDir) {
		t.Fatalf("loaded.DataDir = %q, want absolute path", loaded.DataDir)
	}
	if loaded.Feishu.Quiet != QuietModeNormal {
		t.Fatalf("loaded.Feishu.Quiet = %q, want normal", loaded.Feishu.Quiet)
	}
	if ws := FindWorkspace(loaded, "repo"); ws == nil || ws.Name != "Repo" {
		t.Fatalf("FindWorkspace(repo) = %+v, want Repo", ws)
	}
	if FindWorkspace(loaded, "missing") != nil {
		t.Fatal("FindWorkspace(missing) should return nil")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(content), "[[workspace]]") {
		t.Fatalf("saved config missing workspace sections:\n%s", string(content))
	}
}

func TestLoadMissingFileAndSaveNilConfig(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Fatal("expected Load() on missing file to fail")
	}
	if err := Save(filepath.Join(t.TempDir(), "config.toml"), nil); err == nil {
		t.Fatal("expected Save(nil) to fail")
	}
}

func TestLoadLegacyConfigWithoutDaemonSectionKeepsDefaultDaemonServiceName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := strings.Join([]string{
		`data_dir = ".feidex-data"`,
		``,
		`[log]`,
		`level = "info"`,
		``,
		`[feishu]`,
		`app_id = "app-id"`,
		`app_secret = "app-secret"`,
		``,
		`[codex]`,
		`command = "codex"`,
		`transport = "stdio"`,
		``,
		`[[workspace]]`,
		`id = "default"`,
		`name = "Default"`,
		`cwd = "."`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(legacy config without daemon section) error = %v", err)
	}
	if cfg.Daemon.ServiceName != "feidex" {
		t.Fatalf("Daemon.ServiceName = %q, want default feidex", cfg.Daemon.ServiceName)
	}
	if cfg.Codex.ServiceName != "feidex" {
		t.Fatalf("Codex.ServiceName = %q, want default feidex", cfg.Codex.ServiceName)
	}
	if len(cfg.Workspaces) != 1 || cfg.Workspaces[0].ID != "default" {
		t.Fatalf("Workspaces = %+v, want preserved legacy workspace", cfg.Workspaces)
	}
}

func TestResolvedFrontendsSupportsLegacyAndMultiFrontendConfigs(t *testing.T) {
	legacy := Default()
	legacy.Workspaces[0].Cwd = t.TempDir()
	legacyFrontends := legacy.ResolvedFrontends()
	if len(legacyFrontends) != 1 {
		t.Fatalf("legacy ResolvedFrontends() = %+v, want single frontend", legacyFrontends)
	}
	if legacyFrontends[0].ID != DefaultFrontendID || legacyFrontends[0].Backend != "" {
		t.Fatalf("legacy ResolvedFrontends()[0] = %+v", legacyFrontends[0])
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := strings.Join([]string{
		`data_dir = ".feidex-data"`,
		``,
		`[log]`,
		`level = "info"`,
		``,
		`[codex]`,
		`command = "codex"`,
		`transport = "stdio"`,
		``,
		`[claude]`,
		`command = "claude"`,
		``,
		`[[frontend]]`,
		`id = "codex-main"`,
		`backend = "codex"`,
		`app_id = "cli_codex"`,
		`app_secret = "secret-1"`,
		`reply_in_thread = true`,
		`quiet = "progress"`,
		``,
		`[[frontend]]`,
		`id = "claude-main"`,
		`backend = "claude"`,
		`app_id = "cli_claude"`,
		`app_secret = "secret-2"`,
		`reply_in_thread = true`,
		`quiet = "final"`,
		``,
		`[[workspace]]`,
		`id = "default"`,
		`cwd = "."`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(multi frontend) error = %v", err)
	}
	frontends := cfg.ResolvedFrontends()
	if len(frontends) != 2 {
		t.Fatalf("ResolvedFrontends() len = %d, want 2", len(frontends))
	}
	if frontends[0].ID != "codex-main" || frontends[0].Backend != RuntimeBackendCodex || frontends[0].ConfigIndex != 0 {
		t.Fatalf("frontends[0] = %+v", frontends[0])
	}
	if frontends[1].ID != "claude-main" || frontends[1].Backend != RuntimeBackendClaude || frontends[1].ConfigIndex != 1 {
		t.Fatalf("frontends[1] = %+v", frontends[1])
	}
	if frontends[1].Feishu.Quiet != QuietModeFinal {
		t.Fatalf("frontends[1].Feishu.Quiet = %q, want final", frontends[1].Feishu.Quiet)
	}

	roundTripPath := filepath.Join(dir, "roundtrip.toml")
	roundTrip := &Config{
		DataDir: ".feidex-data",
		Frontends: []FrontendConfig{
			{
				ID:           "codex-main",
				CodexProfile: "main",
				CodexHome:    "/tmp/codex-main",
				FeishuConfig: FeishuConfig{
					Backend:       RuntimeBackendCodex,
					AppID:         "cli_codex",
					AppSecret:     "secret-1",
					ReplyInThread: true,
					Quiet:         QuietModeProgress,
				},
			},
			{
				ID: "claude-main",
				FeishuConfig: FeishuConfig{
					Backend:       RuntimeBackendClaude,
					AppID:         "cli_claude",
					AppSecret:     "secret-2",
					ReplyInThread: true,
					Quiet:         QuietModeFinal,
				},
			},
		},
		Workspaces: []Workspace{{ID: "default", Cwd: "."}},
	}
	if err := Save(roundTripPath, roundTrip); err != nil {
		t.Fatalf("Save(roundtrip frontend config) error = %v", err)
	}
	loadedRoundTrip, err := Load(roundTripPath)
	if err != nil {
		t.Fatalf("Load(roundtrip frontend config) error = %v", err)
	}
	if got := loadedRoundTrip.ResolvedFrontends(); len(got) != 2 || got[0].Backend != RuntimeBackendCodex || got[1].Backend != RuntimeBackendClaude || got[0].CodexProfile != "main" || got[0].CodexHome != "/tmp/codex-main" {
		t.Fatalf("roundtrip ResolvedFrontends() = %+v", got)
	}
}

func TestResolvedFrontendsPreservesUnsetBackend(t *testing.T) {
	cfg := Default()
	cfg.Workspaces[0].Cwd = t.TempDir()
	cfg.Frontends = []FrontendConfig{{
		ID: "unset-main",
		FeishuConfig: FeishuConfig{
			AppID:         "cli_unset",
			AppSecret:     "secret-1",
			ReplyInThread: true,
			Quiet:         QuietModeProgress,
		},
	}}
	if err := cfg.Normalize(t.TempDir()); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	frontends := cfg.ResolvedFrontends()
	if len(frontends) != 1 {
		t.Fatalf("ResolvedFrontends() len = %d, want 1", len(frontends))
	}
	if frontends[0].Backend != "" {
		t.Fatalf("ResolvedFrontends()[0].Backend = %q, want empty", frontends[0].Backend)
	}
}

func TestNormalizeFrontendCodexProfileAndHome(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "config")
	cfg := &Config{
		Frontends: []FrontendConfig{{
			ID:           "xiaolongnv",
			CodexProfile: " xiaolongnv ",
			CodexHome:    "./codex-home",
		}},
		Workspaces: []Workspace{{ID: "default", Cwd: "."}},
	}
	if err := cfg.Normalize(baseDir); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	frontends := cfg.ResolvedFrontends()
	if len(frontends) != 1 {
		t.Fatalf("ResolvedFrontends() len = %d, want 1", len(frontends))
	}
	if frontends[0].CodexProfile != "xiaolongnv" {
		t.Fatalf("CodexProfile = %q, want xiaolongnv", frontends[0].CodexProfile)
	}
	if !filepath.IsAbs(frontends[0].CodexHome) || !strings.HasSuffix(frontends[0].CodexHome, filepath.Join("config", "codex-home")) {
		t.Fatalf("CodexHome = %q, want absolute config/codex-home path", frontends[0].CodexHome)
	}
}

func TestNormalizeFeishuPlatform(t *testing.T) {
	cfg := Default()
	cfg.Workspaces[0].Cwd = t.TempDir()
	cfg.Feishu.Platform = ""
	if err := cfg.Normalize(t.TempDir()); err != nil {
		t.Fatalf("Normalize(default platform) error = %v", err)
	}
	if cfg.Feishu.Platform != FeishuPlatform {
		t.Fatalf("default platform = %q, want feishu", cfg.Feishu.Platform)
	}
	if got := FeishuOpenBaseURLForConfig(cfg.Feishu); got != FeishuOpenBaseURL {
		t.Fatalf("FeishuOpenBaseURLForConfig(default) = %q", got)
	}

	cfg = Default()
	cfg.Workspaces[0].Cwd = t.TempDir()
	cfg.Frontends = []FrontendConfig{{
		ID: "lark-main",
		FeishuConfig: FeishuConfig{
			Platform: LarkPlatform,
			AppID:    "cli_lark",
		},
	}}
	if err := cfg.Normalize(t.TempDir()); err != nil {
		t.Fatalf("Normalize(lark frontend) error = %v", err)
	}
	frontends := cfg.ResolvedFrontends()
	if len(frontends) != 1 || frontends[0].Feishu.Platform != LarkPlatform {
		t.Fatalf("ResolvedFrontends(lark) = %+v", frontends)
	}
	if got := FeishuOpenBaseURLForConfig(frontends[0].Feishu); got != LarkOpenBaseURL {
		t.Fatalf("FeishuOpenBaseURLForConfig(lark) = %q", got)
	}

	cfg = Default()
	cfg.Workspaces[0].Cwd = t.TempDir()
	cfg.Feishu.Platform = "teams"
	if err := cfg.Normalize(t.TempDir()); err == nil {
		t.Fatal("Normalize(unsupported platform) error = nil, want failure")
	}
}

func TestNormalizeRejectsUnsupportedFrontendBackend(t *testing.T) {
	t.Run("legacy feishu backend", func(t *testing.T) {
		cfg := Default()
		cfg.Workspaces[0].Cwd = t.TempDir()
		cfg.Feishu.Backend = "gemini"
		if err := cfg.Normalize(t.TempDir()); err == nil {
			t.Fatal("Normalize() error = nil, want unsupported backend error")
		}
	})

	t.Run("frontend backend", func(t *testing.T) {
		cfg := Default()
		cfg.Workspaces[0].Cwd = t.TempDir()
		cfg.Frontends = []FrontendConfig{{
			ID: "bad-main",
			FeishuConfig: FeishuConfig{
				Backend: "gemini",
			},
		}}
		if err := cfg.Normalize(t.TempDir()); err == nil {
			t.Fatal("Normalize() error = nil, want unsupported backend error")
		}
	})
}

func TestLoadIgnoresLegacyWorkspaceBackendField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := strings.Join([]string{
		`data_dir = ".feidex-data"`,
		``,
		`[log]`,
		`level = "info"`,
		``,
		`[feishu]`,
		`backend = "claude"`,
		`app_id = "legacy-app"`,
		`app_secret = "legacy-secret"`,
		``,
		`[codex]`,
		`command = "codex"`,
		`transport = "stdio"`,
		``,
		`[[workspace]]`,
		`id = "default"`,
		`cwd = "."`,
		`backend = "legacy-workspace-only"`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(legacy workspace backend config) error = %v", err)
	}
	frontends := cfg.ResolvedFrontends()
	if len(frontends) != 1 {
		t.Fatalf("ResolvedFrontends() len = %d, want 1", len(frontends))
	}
	if frontends[0].ID != DefaultFrontendID || frontends[0].Backend != RuntimeBackendClaude {
		t.Fatalf("ResolvedFrontends()[0] = %+v", frontends[0])
	}

	roundTripPath := filepath.Join(dir, "roundtrip.toml")
	if err := Save(roundTripPath, cfg); err != nil {
		t.Fatalf("Save(roundtrip legacy workspace backend config) error = %v", err)
	}
	saved, err := os.ReadFile(roundTripPath)
	if err != nil {
		t.Fatalf("ReadFile(roundtrip config) error = %v", err)
	}
	if strings.Contains(string(saved), "legacy-workspace-only") {
		t.Fatalf("roundtrip config should not persist legacy workspace backend field:\n%s", string(saved))
	}
}

func TestQuietModeValidationAndConfigFallback(t *testing.T) {
	if got, err := NormalizeQuietMode(""); err != nil || got != QuietModeProgress {
		t.Fatalf("NormalizeQuietMode(empty) = %q, %v", got, err)
	}
	if got, err := NormalizeQuietMode("progress"); err != nil || got != QuietModeProgress {
		t.Fatalf("NormalizeQuietMode(progress) = %q, %v", got, err)
	}
	if got, err := NormalizeQuietMode("normal"); err != nil || got != QuietModeNormal {
		t.Fatalf("NormalizeQuietMode(normal) = %q, %v", got, err)
	}
	if got, err := ParseQuietMode("final"); err != nil || got != QuietModeFinal {
		t.Fatalf("ParseQuietMode(final) = %q, %v", got, err)
	}
	if _, err := ParseQuietMode("folded"); err == nil {
		t.Fatal("ParseQuietMode(folded) should fail")
	}

	for _, tc := range []struct {
		name      string
		quietLine string
	}{
		{name: "legacy_string", quietLine: `quiet = "folded"`},
		{name: "legacy_bool", quietLine: `quiet = true`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.toml")
			content := strings.Join([]string{
				`data_dir = ".feidex-data"`,
				``,
				`[log]`,
				`level = "info"`,
				``,
				`[feishu]`,
				tc.quietLine,
				``,
				`[codex]`,
				`command = "codex"`,
				`transport = "stdio"`,
				``,
				`[[workspace]]`,
				`id = "default"`,
				`name = "Default"`,
				`cwd = "."`,
			}, "\n")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("WriteFile(config) error = %v", err)
			}
			loaded, err := Load(path)
			if err != nil {
				t.Fatalf("Load(invalid quiet value) error = %v", err)
			}
			if loaded.Feishu.Quiet != QuietModeNormal {
				t.Fatalf("invalid quiet value loaded as %q, want normal fallback", loaded.Feishu.Quiet)
			}
		})
	}
}
