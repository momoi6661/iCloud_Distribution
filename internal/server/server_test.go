package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"icloud_distribution/internal/account"
	"icloud_distribution/internal/auth"
	"icloud_distribution/internal/share"
)

// newTestServer 构建一个启用 UI 鉴权的测试服务 (无网络依赖)。
// token 为空时使用管理员账号模式 (未初始化)。
func newTestServer(t *testing.T, token string) *Server {
	t.Helper()
	mgr, err := account.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	logins := auth.NewLoginStore()
	t.Cleanup(logins.Close)
	shares, err := share.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	var ui *auth.UIAuth
	var creds *auth.CredentialStore
	if token != "" {
		ui = auth.NewUIAuth(token)
	} else {
		creds, err = auth.NewCredentialStore(t.TempDir())
		if err != nil {
			t.Fatalf("NewCredentialStore: %v", err)
		}
		ui = auth.NewUIAuthFromCredentials(creds)
	}
	return New(mgr, logins, ui, creds, shares, nil, true)
}

// loginAndGetCookie 完成 UI 登录并返回会话 Cookie。
func loginAndGetCookie(t *testing.T, s *Server, token string) *http.Cookie {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ui/login", strings.NewReader(`{"token":"`+token+`"}`))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UI 登录失败: %d %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("登录后应返回会话 Cookie")
	}
	return cookies[0]
}

func TestUIAuth_Required(t *testing.T) {
	s := newTestServer(t, "test-token")

	// 未登录访问受保护接口 → 401 且带 ui_auth_expired 标记
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/accounts", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("未登录应返回 401, 实际 %d", w.Code)
	}
	var resp struct {
		Data struct {
			Reason string `json:"reason"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Data.Reason != "ui_auth_expired" {
		t.Errorf("UI 鉴权 401 应带 ui_auth_expired 标记, 实际 %q", resp.Data.Reason)
	}

	// 错误口令 → 401
	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ui/login", strings.NewReader(`{"token":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("错误口令应返回 401, 实际 %d", w.Code)
	}

	// 正确口令 → 200,后续请求放行
	cookie := loginAndGetCookie(t, s, "test-token")
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/accounts", nil)
	req.AddCookie(cookie)
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("登录后应返回 200, 实际 %d", w.Code)
	}
}

func TestUIAuth_Disabled(t *testing.T) {
	s := newTestServer(t, "") // 空口令 = 关闭鉴权

	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/accounts", nil))
	if w.Code != http.StatusOK {
		t.Errorf("关闭鉴权时应直接放行, 实际 %d", w.Code)
	}
}

func TestHandlerParamValidation(t *testing.T) {
	s := newTestServer(t, "")
	h := s.Handler()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"创建别名缺少 account_id", "POST", "/api/create", `{"label":"x"}`, http.StatusBadRequest},
		{"批量创建缺少 count", "POST", "/api/accounts/acc_x/aliases/batch", `{}`, http.StatusBadRequest},
		{"批量创建超过上限", "POST", "/api/accounts/acc_x/aliases/batch", `{"count":51}`, http.StatusBadRequest},
		{"login/start 缺少 password", "POST", "/api/accounts/acc_x/login/start", `{}`, http.StatusBadRequest},
		{"login/otp 缺少参数", "POST", "/api/accounts/acc_x/login/otp", `{"code":"123456"}`, http.StatusBadRequest},
		{"login/otp 无效会话", "POST", "/api/accounts/acc_x/login/otp", `{"session_id":"bad","code":"123456"}`, http.StatusGone},
		{"inbox 缺少 account_id", "GET", "/api/inbox", ``, http.StatusBadRequest},
		{"inbox/message uid 非数字", "GET", "/api/inbox/message?account_id=a&uid=abc", ``, http.StatusBadRequest},
		{"添加账号缺少 name", "POST", "/api/accounts", `{"email":"a@b.c"}`, http.StatusBadRequest},
		{"设置密码缺少字段", "POST", "/api/accounts/acc_x/password", `{"icloud_email":"a@b.c"}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			h.ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Errorf("状态码 = %d, 期望 %d, body: %s", w.Code, tt.want, w.Body.String())
			}
		})
	}
}

func TestAddAndListAccount(t *testing.T) {
	s := newTestServer(t, "")
	h := s.Handler()

	// 添加账号 (无 Cookie → pending)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/accounts", strings.NewReader(`{"name":"主号","email":"me@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("添加账号失败: %d %s", w.Code, w.Body.String())
	}

	var resp apiResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data := resp.Data.(map[string]interface{})
	if data["status"] != "pending" {
		t.Errorf("新账号状态应为 pending, 实际 %v", data["status"])
	}
	if data["cookies"] != nil {
		t.Error("返回的账号不应包含 cookies (脱敏)")
	}

	// 列表应包含该账号且不含 cookies
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/accounts", nil))
	var listResp struct {
		Success bool              `json:"success"`
		Data    []json.RawMessage `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &listResp)
	if len(listResp.Data) != 1 {
		t.Fatalf("账号数量 = %d, 期望 1", len(listResp.Data))
	}
}

func TestSetupFlow(t *testing.T) {
	s := newTestServer(t, "") // 管理员账号模式,未初始化
	h := s.Handler()

	// 未初始化时 API 放行 (等待 setup)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/accounts", nil))
	if w.Code != http.StatusOK {
		t.Errorf("未初始化时应放行, 实际 %d", w.Code)
	}

	// status 应显示未初始化
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/ui/status", nil))
	var status struct {
		Data struct {
			Initialized bool `json:"initialized"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &status)
	if status.Data.Initialized {
		t.Error("初始应为未初始化")
	}

	// setup 创建管理员
	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ui/setup", strings.NewReader(`{"username":"admin","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("setup 失败: %d %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("setup 后应签发会话 Cookie")
	}

	// 重复 setup 应 403
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/ui/setup", strings.NewReader(`{"username":"x","password":"yyyyyy"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("重复 setup 应 403, 实际 %d", w.Code)
	}

	// 初始化后无 Cookie → 401
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/accounts", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("初始化后未登录应 401, 实际 %d", w.Code)
	}

	// 错误密码登录 → 401
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/ui/login", strings.NewReader(`{"username":"admin","password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("错误密码应 401, 实际 %d", w.Code)
	}

	// 正确密码登录 → 200
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/ui/login", strings.NewReader(`{"username":"admin","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("正确密码应 200, 实际 %d: %s", w.Code, w.Body.String())
	}
}

func TestSPAFallback(t *testing.T) {
	s := newTestServer(t, "")

	// 未知 API 路径 → 404 JSON
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/nonexistent", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("未知 API 应返回 404, 实际 %d", w.Code)
	}

	// static 为 nil 时前端路由 → 503 提示
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/accounts/acc_x", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("无前端资源时应返回 503, 实际 %d", w.Code)
	}
}
