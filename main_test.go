package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	// 1. Create a valid temporary config yaml
	validYAML := `
discord:
  bot_token: "test_bot_token"
  channel_id: "test_channel_id"
web:
  password: "test_password"
  port: 9090
`
	tmpFile, err := os.CreateTemp("", "config_test_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(validYAML)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Test loading valid config
	err = loadConfig(tmpFile.Name())
	if err != nil {
		t.Errorf("loadConfig failed unexpectedly: %v", err)
	}

	if config.Discord.BotToken != "test_bot_token" || config.Web.Password != "test_password" || config.Web.Port != 9090 {
		t.Errorf("loadConfig loaded incorrect values: %+v", config)
	}

	// 2. Test invalid config
	invalidYAML := `
discord:
  bot_token: "YOUR_DISCORD_BOT_TOKEN" # Default value
web:
  password: ""
`
	tmpFile2, err := os.CreateTemp("", "config_test_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile2.Name())

	tmpFile2.Write([]byte(invalidYAML))
	tmpFile2.Close()

	err = loadConfig(tmpFile2.Name())
	if err == nil {
		t.Error("loadConfig should have failed on default/empty config values, but it succeeded")
	}
}

func TestHandleLogin(t *testing.T) {
	// Set test credentials
	config.Web.Password = "supersecret"

	// 1. Test wrong password
	reqBody, _ := json.Marshal(map[string]string{"password": "wrongpassword"})
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(reqBody))
	w := httptest.NewRecorder()

	handleLogin(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	if !strings.Contains(w.Body.String(), "비밀번호가 올바르지 않습니다.") {
		t.Errorf("Expected body to contain error message, got %s", w.Body.String())
	}

	// 2. Test correct password
	reqBody, _ = json.Marshal(map[string]string{"password": "supersecret"})
	req = httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(reqBody))
	w = httptest.NewRecorder()

	handleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	respCookie := w.Result().Cookies()
	var sessionID string
	for _, c := range respCookie {
		if c.Name == "session_id" {
			sessionID = c.Value
		}
	}

	if sessionID == "" {
		t.Error("Expected session_id cookie to be set")
	}

	// Verify session is active in map
	sessionsMu.Lock()
	_, exists := sessions[sessionID]
	sessionsMu.Unlock()
	if !exists {
		t.Error("Session was not added to the internal sessions map")
	}
}

func TestHandleInviteAPI(t *testing.T) {
	config.Web.Password = "secret"
	
	// Pre-populate invite cache to avoid network call
	inviteCacheMu.Lock()
	inviteCache = &InviteCache{
		InviteURL:    "https://discord.gg/testcode123",
		GuildName:    "Test Guild",
		GuildIconURL: "https://cdn.discordapp.com/icons/123/456.png",
		ChannelName:  "welcome",
		ExpiresAt:    time.Now().Add(10 * time.Hour),
	}
	inviteCacheMu.Unlock()

	// 1. Test access without authorization
	req := httptest.NewRequest(http.MethodGet, "/api/invite", nil)
	w := httptest.NewRecorder()
	handleInviteAPI(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected unauthorized status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	// 2. Test access with valid session
	// Log in to get session
	loginBody, _ := json.Marshal(map[string]string{"password": "secret"})
	loginReq := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(loginBody))
	loginW := httptest.NewRecorder()
	handleLogin(loginW, loginReq)

	var sessionCookie *http.Cookie
	for _, c := range loginW.Result().Cookies() {
		if c.Name == "session_id" {
			sessionCookie = c
		}
	}

	if sessionCookie == nil {
		t.Fatal("Could not obtain session cookie for test")
	}

	// Request invite api with cookie
	req = httptest.NewRequest(http.MethodGet, "/api/invite", nil)
	req.AddCookie(sessionCookie)
	w = httptest.NewRecorder()
	handleInviteAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var inviteResp InviteCache
	if err := json.Unmarshal(w.Body.Bytes(), &inviteResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if inviteResp.InviteURL != "https://discord.gg/testcode123" || inviteResp.GuildName != "Test Guild" {
		t.Errorf("Unexpected invite data returned: %+v", inviteResp)
	}
}
