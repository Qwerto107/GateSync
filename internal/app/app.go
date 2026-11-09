package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"gatesync/internal/webui"
)

type Config struct {
	Address, DataDir, WebDir, PublicURL, PasswordHash, GoogleID, GoogleSecret string
	InitialPassword                                                           string
	AllowedEmails                                                             map[string]bool
}

func LoadConfig() (Config, error) {
	get := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}
	c := Config{Address: get("GATESYNC_ADDRESS", "127.0.0.1:13850"), DataDir: get("GATESYNC_DATA_DIR", "data"), WebDir: os.Getenv("GATESYNC_WEB_DIR"), PublicURL: strings.TrimRight(get("GATESYNC_PUBLIC_URL", "http://localhost:13850"), "/"), PasswordHash: os.Getenv("GATESYNC_PASSWORD_HASH"), GoogleID: os.Getenv("GOOGLE_CLIENT_ID"), GoogleSecret: os.Getenv("GOOGLE_CLIENT_SECRET"), AllowedEmails: map[string]bool{}}
	// 旧登录配置仅在首次建立系统设置时导入，后续以数据库为准。
	c.InitialPassword = os.Getenv("GATESYNC_ACCESS_PASSWORD")
	for _, email := range strings.Split(os.Getenv("GOOGLE_ALLOWED_EMAILS"), ",") {
		email = strings.ToLower(strings.TrimSpace(email))
		if email != "" {
			c.AllowedEmails[email] = true
		}
	}
	return c, nil
}

type oauthAttempt struct {
	State, Verifier, Nonce string
	Expires                time.Time
}
type App struct {
	cfg          Config
	systemMu     sync.RWMutex
	system       SystemSettings
	authMu       sync.RWMutex
	lighthouse   func(Credentials, string) (Cloud, error)
	store        *Store
	installation string
	cloud        func(Credentials, string) (Cloud, error)
	mutate       sync.Mutex
	detectMu     sync.Mutex
	oauthMu      sync.Mutex
	attempts     map[string]oauthAttempt
	loginSlots   chan struct{}
	rateMu       sync.Mutex
	loginRates   map[string][]time.Time
	nextMu       sync.Mutex
	next         time.Time
	wake         chan struct{}
}

func New(c Config) (*App, error) {
	s, e := openStore(c.DataDir, os.Getenv("GATESYNC_MASTER_KEY"))
	if e != nil {
		return nil, e
	}
	a := &App{cfg: c, store: s, cloud: newTencent, lighthouse: newLighthouse, attempts: map[string]oauthAttempt{}, loginSlots: make(chan struct{}, 4), loginRates: map[string][]time.Time{}, wake: make(chan struct{}, 1)}
	if e = a.initSystem(c); e != nil {
		s.db.Close()
		return nil, e
	}
	if e = s.getMeta("installation", &a.installation); errors.Is(e, sql.ErrNoRows) {
		a.installation = randomID()[:16]
		e = s.setMeta("installation", a.installation)
	}
	if e != nil {
		s.db.Close()
		return nil, e
	}
	var settings Settings
	if e = s.getMeta("settings", &settings); errors.Is(e, sql.ErrNoRows) {
		settings = Settings{Enabled: false, Interval: 5, Sources: []string{"https://api.ipify.org", "https://ifconfig.me/ip"}}
		e = nil
	}
	if e == nil && settings.Timezone == "" {
		// 只在迁移时导入旧时区，之后更改环境变量不会覆盖后台设置。
		settings.Timezone = os.Getenv("TZ")
		if _, err := time.LoadLocation(settings.Timezone); settings.Timezone == "" || err != nil {
			settings.Timezone = "Asia/Shanghai"
		}
		e = s.setMeta("settings", settings)
	}
	if e != nil {
		s.db.Close()
		return nil, e
	}
	return a, nil
}
func (a *App) Close() error { return a.store.db.Close() }
func (a *App) settings() (Settings, error) {
	var s Settings
	e := a.store.getMeta("settings", &s)
	if s.Timezone == "" {
		s.Timezone = "Asia/Shanghai"
	}
	if s.DetectOnManualSync == nil {
		value := true
		s.DetectOnManualSync = &value
	}
	if s.DetectOnSchedule == nil {
		value := true
		s.DetectOnSchedule = &value
	}
	return s, e
}
func (a *App) RunScheduler(ctx context.Context) {
	setNext := func() {
		s, e := a.settings()
		var next time.Time
		if e == nil && s.Enabled {
			schedule, e := s.schedule()
			if e == nil {
				next = schedule.Next(time.Now())
			}
		}
		a.nextMu.Lock()
		a.next = next
		a.nextMu.Unlock()
	}
	setNext()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.wake:
			setNext()
		case <-tick.C:
			a.nextMu.Lock()
			due := !a.next.IsZero() && !time.Now().Before(a.next)
			a.nextMu.Unlock()
			if !due {
				continue
			}
			a.mutate.Lock()
			s, e := a.settings()
			if e == nil && s.Enabled {
				run, cancel := context.WithTimeout(ctx, 3*time.Minute)
				a.syncScheduled(run)
				cancel()
			}
			a.mutate.Unlock()
			setNext()
		}
	}
}
func (a *App) syncScheduled(ctx context.Context) {
	targets, e := a.store.targets()
	if e != nil {
		log.Printf("读取自动同步目标失败")
		return
	}
	needsIP := false
	for _, t := range targets {
		needsIP = needsIP || t.AutoSync && t.IPMode == "dynamic"
	}
	ip := ""
	if needsIP {
		ip, _ = a.ipForSync(ctx, "auto")
	}
	for _, t := range targets {
		if !t.AutoSync {
			continue
		}
		if _, e := a.syncTarget(ctx, t, ip, "auto", false); e != nil {
			log.Printf("自动同步 %s 未完成", t.ID)
		}
	}
}

func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	jsonResponse(w, status, map[string]string{"error": msg})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		fail(w, 400, "请求格式有误")
		return false
	}
	if d.Decode(&struct{}{}) != io.EOF {
		fail(w, 400, "请求包含多余内容")
		return false
	}
	return true
}
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { jsonResponse(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/auth/config", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, 200, map[string]bool{"google": a.systemSettings().GoogleEnabled})
	})
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("GET /api/auth/google", a.googleStart)
	mux.HandleFunc("GET /api/auth/google/callback", a.googleCallback)
	mux.Handle("POST /api/auth/logout", a.auth(http.HandlerFunc(a.logout)))
	mux.Handle("GET /api/system", a.auth(http.HandlerFunc(a.getSystem)))
	mux.Handle("PUT /api/system", a.auth(http.HandlerFunc(a.updateSystem)))
	mux.Handle("GET /api/accounts/{id}/regions", a.auth(http.HandlerFunc(a.regions)))
	mux.Handle("POST /api/accounts/regions", a.auth(http.HandlerFunc(a.formRegions)))
	mux.Handle("GET /api/state", a.auth(http.HandlerFunc(a.state)))
	mux.Handle("POST /api/ip/detect", a.auth(http.HandlerFunc(a.detect)))
	mux.Handle("PUT /api/settings", a.auth(http.HandlerFunc(a.saveSettings)))
	mux.Handle("POST /api/accounts", a.auth(http.HandlerFunc(a.saveAccount)))
	mux.Handle("PUT /api/accounts/{id}", a.auth(http.HandlerFunc(a.saveAccount)))
	mux.Handle("POST /api/accounts/test", a.auth(http.HandlerFunc(a.testCredentials)))
	mux.Handle("POST /api/accounts/{id}/test", a.auth(http.HandlerFunc(a.testAccount)))
	mux.Handle("GET /api/accounts/{id}/groups", a.auth(http.HandlerFunc(a.groups)))
	mux.Handle("DELETE /api/accounts/{id}", a.auth(http.HandlerFunc(a.deleteAccount)))
	mux.Handle("PUT /api/targets/order", a.auth(http.HandlerFunc(a.reorderTargets)))
	mux.Handle("POST /api/targets", a.auth(http.HandlerFunc(a.saveTarget)))
	mux.Handle("PUT /api/targets/{id}", a.auth(http.HandlerFunc(a.saveTarget)))
	mux.Handle("PATCH /api/targets/{id}/auto", a.auth(http.HandlerFunc(a.toggleTarget)))
	mux.Handle("DELETE /api/targets/{id}", a.auth(http.HandlerFunc(a.deleteTarget)))
	mux.Handle("POST /api/targets/{id}/sync", a.auth(http.HandlerFunc(a.syncOne)))
	mux.Handle("POST /api/sync", a.auth(http.HandlerFunc(a.syncAll)))
	mux.Handle("GET /api/logs", a.auth(http.HandlerFunc(a.logs)))
	mux.Handle("GET /api/logs/export", a.auth(http.HandlerFunc(a.exportLogs)))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "接口不存在") })
	webFiles := webui.Files(a.cfg.WebDir)
	fileServer := http.FileServer(http.FS(webFiles))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if _, e := fs.Stat(webFiles, "index.html"); e != nil {
			http.Error(w, "请先在 web 目录执行 npm install 和 npm run build", 503)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
	mux.Handle("/assets/", fileServer)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			// 自定义请求头不能由跨站表单携带，且本服务不开放 CORS。
			if r.Header.Get("X-GateSync-Request") != "1" {
				fail(w, 403, "请求来源校验失败")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, e := url.Parse(origin)
				if e != nil || (origin != a.systemSettings().PublicURL && u.Host != r.Host) {
					fail(w, 403, "请求来源校验失败")
					return
				}
			}
		}
		mux.ServeHTTP(w, r)
	})
}
