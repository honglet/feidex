package config

import (
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSetupFeishuAutoModes(t *testing.T) {
	withDefaultTransport(t, setupRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "open.feishu.cn" {
			return testHTTPResponse(`{"code":0,"tenant_access_token":"token"}`), nil
		}
		body, _ := io.ReadAll(req.Body)
		values, _ := url.ParseQuery(string(body))
		switch values.Get("action") {
		case "init":
			return testHTTPResponse(`{"supported_auth_methods":["client_secret"]}`), nil
		case "begin":
			return testHTTPResponse(`{"device_code":"dev-1","verification_uri_complete":"https://example.test/scan","interval":0,"expire_in":60}`), nil
		case "poll":
			return testHTTPResponse(`{"client_id":"new-id","client_secret":"new-secret"}`), nil
		default:
			return testHTTPResponse(`{}`), nil
		}
	}))

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := SetupFeishu(FeishuSetupAuto, FeishuSetupOptions{ConfigPath: cfgPath, AppPair: "bind-id:bind-secret"}); err != nil {
		t.Fatalf("SetupFeishu(auto bind) error = %v", err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load(auto bind) error = %v", err)
	}
	if cfg.Feishu.AppID != "bind-id" {
		t.Fatalf("auto bind app id = %q, want bind-id", cfg.Feishu.AppID)
	}

	cfgPath = filepath.Join(t.TempDir(), "config.toml")
	if err := SetupFeishu(FeishuSetupAuto, FeishuSetupOptions{ConfigPath: cfgPath, Timeout: time.Second}); err != nil {
		t.Fatalf("SetupFeishu(auto new) error = %v", err)
	}
	cfg, err = Load(cfgPath)
	if err != nil {
		t.Fatalf("Load(auto new) error = %v", err)
	}
	if cfg.Feishu.AppID != "new-id" {
		t.Fatalf("auto new app id = %q, want new-id", cfg.Feishu.AppID)
	}
}

func TestSetupFeishuBindSupportsLarkPlatform(t *testing.T) {
	var hosts []string
	withDefaultTransport(t, setupRoundTripper(func(req *http.Request) (*http.Response, error) {
		hosts = append(hosts, req.URL.Host)
		return testHTTPResponse(`{"code":0,"tenant_access_token":"token"}`), nil
	}))

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := SetupFeishu(FeishuSetupBind, FeishuSetupOptions{
		ConfigPath: cfgPath,
		AppPair:    "bind-id:bind-secret",
		Platform:   LarkPlatform,
	}); err != nil {
		t.Fatalf("SetupFeishu(lark bind) error = %v", err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load(lark bind) error = %v", err)
	}
	if cfg.Feishu.Platform != LarkPlatform {
		t.Fatalf("saved platform = %q, want lark", cfg.Feishu.Platform)
	}
	if len(hosts) != 1 || hosts[0] != "open.larksuite.com" {
		t.Fatalf("validation hosts = %+v, want open.larksuite.com", hosts)
	}
}

func TestSetupFeishuNewRejectsLarkPlatform(t *testing.T) {
	err := SetupFeishu(FeishuSetupNew, FeishuSetupOptions{
		ConfigPath: filepath.Join(t.TempDir(), "config.toml"),
		Platform:   LarkPlatform,
	})
	if err == nil || !strings.Contains(err.Error(), "supported only for Feishu") {
		t.Fatalf("SetupFeishu(new lark) error = %v, want unsupported registration", err)
	}
}

func TestSetupFeishuPreservesExistingPlatformWhenOmitted(t *testing.T) {
	var host string
	withDefaultTransport(t, setupRoundTripper(func(req *http.Request) (*http.Response, error) {
		host = req.URL.Host
		return testHTTPResponse(`{"code":0,"tenant_access_token":"token"}`), nil
	}))

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := Default()
	cfg.Frontends = []FrontendConfig{{
		ID: "lark-main",
		FeishuConfig: FeishuConfig{
			Platform:  LarkPlatform,
			AppID:     "old-id",
			AppSecret: "old-secret",
		},
	}}
	if err := Save(cfgPath, cfg); err != nil {
		t.Fatalf("Save(config) error = %v", err)
	}

	if err := SetupFeishu(FeishuSetupBind, FeishuSetupOptions{
		ConfigPath: cfgPath,
		FrontendID: "lark-main",
		AppPair:    "new-id:new-secret",
	}); err != nil {
		t.Fatalf("SetupFeishu(preserve platform) error = %v", err)
	}
	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load(config) error = %v", err)
	}
	if loaded.Frontends[0].Platform != LarkPlatform {
		t.Fatalf("frontend platform = %q, want lark", loaded.Frontends[0].Platform)
	}
	if host != "open.larksuite.com" {
		t.Fatalf("validation host = %q, want open.larksuite.com", host)
	}
}

func TestRunRegistrationFlowTimeoutAndExpiry(t *testing.T) {
	withDefaultTransport(t, setupRoundTripper(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		values, _ := url.ParseQuery(string(body))
		switch values.Get("action") {
		case "init":
			return testHTTPResponse(`{"supported_auth_methods":["client_secret"]}`), nil
		case "begin":
			return testHTTPResponse(`{"device_code":"dev-1","verification_uri_complete":"https://example.test/scan","interval":0,"expire_in":60}`), nil
		case "poll":
			return testHTTPResponse(`{"error":"slow_down"}`), nil
		default:
			return testHTTPResponse(`{}`), nil
		}
	}))
	if _, _, err := runRegistrationFlow(10*time.Millisecond, ""); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("runRegistrationFlow(timeout) error = %v", err)
	}

	withDefaultTransport(t, setupRoundTripper(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		values, _ := url.ParseQuery(string(body))
		switch values.Get("action") {
		case "init":
			return testHTTPResponse(`{"supported_auth_methods":["client_secret"]}`), nil
		case "begin":
			return testHTTPResponse(`{"device_code":"dev-1","verification_uri_complete":"https://example.test/scan","interval":0,"expire_in":60}`), nil
		case "poll":
			return testHTTPResponse(`{"error":"expired_token"}`), nil
		default:
			return testHTTPResponse(`{}`), nil
		}
	}))
	if _, _, err := runRegistrationFlow(time.Second, ""); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("runRegistrationFlow(expired) error = %v", err)
	}
}

func TestRegistrationCallStatusError(t *testing.T) {
	client := &http.Client{Transport: setupRoundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 500,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":"bad"}`)),
			Request:    req,
		}, nil
	})}
	var poll registrationPollResponse
	if err := registrationCall(client, "poll", map[string]string{"device_code": "device-1"}, &poll); err == nil || !strings.Contains(err.Error(), "status=500") {
		t.Fatalf("registrationCall(status error) = %v, want status error", err)
	}
}

func TestRegistrationCallPollAuthorizationPendingStatus400(t *testing.T) {
	client := &http.Client{Transport: setupRoundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 400,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":"authorization_pending","error_description":"","code":20094}`)),
			Request:    req,
		}, nil
	})}
	var poll registrationPollResponse
	if err := registrationCall(client, "poll", map[string]string{"device_code": "device-1"}, &poll); err != nil {
		t.Fatalf("registrationCall(poll authorization_pending) error = %v", err)
	}
	if poll.Error != "authorization_pending" {
		t.Fatalf("registrationCall(poll authorization_pending) = %+v, want authorization_pending", poll)
	}
}
