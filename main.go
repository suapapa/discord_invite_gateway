package main

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds configuration parameters
type Config struct {
	Discord struct {
		BotToken  string `yaml:"bot_token"`
		ChannelID string `yaml:"channel_id"`
	} `yaml:"discord"`
	Web struct {
		Password string `yaml:"password"`
		Port     int    `yaml:"port"`
	} `yaml:"web"`
}

// InviteCache caches the Discord invite details in memory
type InviteCache struct {
	InviteURL    string    `json:"invite_url"`
	GuildName    string    `json:"guild_name"`
	GuildIconURL string    `json:"guild_icon_url"`
	ChannelName  string    `json:"channel_name"`
	ExpiresAt    time.Time `json:"-"`
}

// Discord API response structures
type discordGuild struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}

type discordChannel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type discordInviteResponse struct {
	Code    string         `json:"code"`
	Guild   discordGuild   `json:"guild"`
	Channel discordChannel `json:"channel"`
}

// Go 1.16+ embed FS to embed the HTML template into the single binary
//
//go:embed templates/index.html
var templatesFS embed.FS

var (
	config     Config
	sessions   = make(map[string]time.Time)
	sessionsMu sync.Mutex

	inviteCache   *InviteCache
	inviteCacheMu sync.Mutex
)

func main() {
	log.Println("서버 설정을 로드하는 중...")
	if err := loadConfig("config.yaml"); err != nil {
		log.Fatalf("설정 파일을 불러오지 못했습니다: %v", err)
	}

	// 1시간마다 만료된 세션을 정리하는 백그라운드 고루틴 실행
	go cleanupExpiredSessions(1 * time.Hour)

	// 라우터 설정
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/login", handleLogin)
	http.HandleFunc("/api/invite", handleInviteAPI)

	addr := fmt.Sprintf(":%d", config.Web.Port)
	log.Printf("서버가 시작되었습니다. http://localhost%s 에서 접속 가능합니다.", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("서버 기동 실패: %v", err)
	}
}

// loadConfig reads and parses config.yaml
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	if config.Discord.BotToken == "" || config.Discord.BotToken == "YOUR_DISCORD_BOT_TOKEN" {
		return fmt.Errorf("discord.bot_token이 설정되지 않았거나 기본값입니다")
	}
	if config.Discord.ChannelID == "" || config.Discord.ChannelID == "YOUR_DISCORD_CHANNEL_ID" {
		return fmt.Errorf("discord.channel_id가 설정되지 않았거나 기본값입니다")
	}
	if config.Web.Password == "" || config.Web.Password == "YOUR_WEB_PASSWORD" {
		return fmt.Errorf("web.password가 설정되지 않았거나 기본값입니다")
	}
	if config.Web.Port == 0 {
		config.Web.Port = 8080
	}
	return nil
}

// handleIndex serves the single-page HTML frontend
func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	htmlContent, err := templatesFS.ReadFile("templates/index.html")
	if err != nil {
		log.Printf("Template read error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(htmlContent)
}

// handleLogin validates password and sets a transient session cookie
func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// ConstantTimeCompare to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(config.Web.Password)) != 1 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "비밀번호가 올바르지 않습니다."}`))
		return
	}

	// Generate safe session token
	token := generateSessionToken()
	if token == "" {
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Save session in memory (expires in 12 hours)
	sessionsMu.Lock()
	sessions[token] = time.Now().Add(12 * time.Hour)
	sessionsMu.Unlock()

	// Set transient cookie (no MaxAge/Expires so it dies when browser closes)
	cookie := &http.Cookie{
		Name:     "session_id",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success": true}`))
}

// handleInviteAPI returns the Discord invite link and server info
func handleInviteAPI(w http.ResponseWriter, r *http.Request) {
	if !isSessionValid(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "인증이 필요합니다."}`))
		return
	}

	invite, err := getOrFetchInvite()
	if err != nil {
		log.Printf("Discord API error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf(`{"error": "디스코드 초대장을 생성하는 데 실패했습니다: %v"}`, err)))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invite)
}

// getOrFetchInvite returns cached invite or fetches a new one if expired
func getOrFetchInvite() (*InviteCache, error) {
	inviteCacheMu.Lock()
	defer inviteCacheMu.Unlock()

	// If cache exists and has at least 5 minutes before expiring, reuse it
	if inviteCache != nil && time.Now().Before(inviteCache.ExpiresAt.Add(-5*time.Minute)) {
		return inviteCache, nil
	}

	// Cache expired or missing, fetch a new one
	log.Println("디스코드 API를 통해 새 초대 코드 생성 중...")
	invite, err := fetchNewDiscordInvite(config.Discord.BotToken, config.Discord.ChannelID)
	if err != nil {
		return nil, err
	}

	inviteCache = invite
	return inviteCache, nil
}

// fetchNewDiscordInvite calls the Discord API to create a 24-hour invite link
func fetchNewDiscordInvite(botToken, channelID string) (*InviteCache, error) {
	url := fmt.Sprintf("https://discord.com/api/v10/channels/%s/invites", channelID)

	// Create invite for 24 hours (86400 seconds)
	reqBody, _ := json.Marshal(map[string]interface{}{
		"max_age":   86400,
		"max_uses":  0,
		"temporary": false,
		"unique":    false, // Reuse existing similar invite if possible
	})

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bot "+botToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DiscordBot (https://github.com/suapapa/discord_invite_gateway, v1.0)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API response status %s: %s", resp.Status, string(bodyBytes))
	}

	var discordResp discordInviteResponse
	if err := json.NewDecoder(resp.Body).Decode(&discordResp); err != nil {
		return nil, err
	}

	if discordResp.Code == "" {
		return nil, fmt.Errorf("초대 코드가 응답에 포함되어 있지 않습니다")
	}

	var iconURL string
	if discordResp.Guild.Icon != "" {
		iconURL = fmt.Sprintf("https://cdn.discordapp.com/icons/%s/%s.png", discordResp.Guild.ID, discordResp.Guild.Icon)
	}

	return &InviteCache{
		InviteURL:    fmt.Sprintf("https://discord.gg/%s", discordResp.Code),
		GuildName:    discordResp.Guild.Name,
		GuildIconURL: iconURL,
		ChannelName:  discordResp.Channel.Name,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}, nil
}

// check if cookie token is valid
func isSessionValid(r *http.Request) bool {
	cookie, err := r.Cookie("session_id")
	if err != nil {
		return false
	}

	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	expiry, exists := sessions[cookie.Value]
	if !exists {
		return false
	}

	if time.Now().After(expiry) {
		delete(sessions, cookie.Value)
		return false
	}

	return true
}

func generateSessionToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

func cleanupExpiredSessions(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		sessionsMu.Lock()
		now := time.Now()
		for token, expiry := range sessions {
			if now.After(expiry) {
				delete(sessions, token)
			}
		}
		sessionsMu.Unlock()
	}
}
