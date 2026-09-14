package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"icloud_distribution/internal/account"
	"icloud_distribution/internal/auth"
	"icloud_distribution/internal/share"
)

// newTestServer 构建一个 UI 鉴权测试服务 (无网络依赖)。
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

	if token == "" {
		token = "test-token"
	}
	users, err := auth.NewUserStore(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = users.Close() })
	ui, err := auth.NewUIAuth(users, "liuyuquan", token)
	if err != nil {
		t.Fatal(err)
	}
	return New(mgr, logins, ui, shares, nil, true)
}

// loginAndGetCookie 完成 UI 登录并返回会话 Cookie。
func loginAndGetCookie(t *testing.T, s *Server, token string) *http.Cookie {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ui/login", strings.NewReader(`{"username":"liuyuquan","password":"`+token+`"}`))
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

func loginUserAndGetCookie(t *testing.T, s *Server, username, password string) *http.Cookie {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ui/login", strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK || len(w.Result().Cookies()) == 0 {
		t.Fatalf("用户登录失败: %d %s", w.Code, w.Body.String())
	}
	return w.Result().Cookies()[0]
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
	req := httptest.NewRequest("POST", "/api/ui/login", strings.NewReader(`{"username":"liuyuquan","password":"wrong"}`))
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

func TestAccountOwnershipIncludesSuperadmin(t *testing.T) {
	s := newTestServer(t, "test-token")
	adminCookie := loginAndGetCookie(t, s, "test-token")
	if _, err := s.ui.Store().Create("member", "password123", false); err != nil {
		t.Fatal(err)
	}
	memberCookie := loginUserAndGetCookie(t, s, "member", "password123")

	create := func(cookie *http.Cookie, name string) string {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/accounts", strings.NewReader(`{"name":"`+name+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		s.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("创建账号失败: %d %s", w.Code, w.Body.String())
		}
		var response struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response.Data.ID
	}
	adminID := create(adminCookie, "admin-account")
	memberID := create(memberCookie, "member-account")

	assertList := func(cookie *http.Cookie, contains, excludes string) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/accounts", nil)
		req.AddCookie(cookie)
		s.Handler().ServeHTTP(w, req)
		body := w.Body.String()
		if w.Code != http.StatusOK || !strings.Contains(body, contains) || strings.Contains(body, excludes) {
			t.Fatalf("账号归属过滤异常: %d %s", w.Code, body)
		}
	}
	assertList(adminCookie, adminID, memberID)
	assertList(memberCookie, memberID, adminID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/accounts/"+memberID+"/organizer", nil)
	req.AddCookie(adminCookie)
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("管理员不应越权访问普通用户账号: %d %s", w.Code, w.Body.String())
	}
}

func TestHandlerParamValidation(t *testing.T) {
	s := newTestServer(t, "")
	h := s.Handler()
	cookie := loginAndGetCookie(t, s, "test-token")

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
		{"inbox 读取方式无效", "GET", "/api/inbox?account_id=a&method=unknown", ``, http.StatusBadRequest},
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
			req.AddCookie(cookie)
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
	cookie := loginAndGetCookie(t, s, "test-token")

	// 添加账号 (无 Cookie → pending)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/accounts", strings.NewReader(`{"name":"主号","email":"me@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
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
	req = httptest.NewRequest("GET", "/api/accounts", nil)
	req.AddCookie(cookie)
	h.ServeHTTP(w, req)
	var listResp struct {
		Success bool              `json:"success"`
		Data    []json.RawMessage `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &listResp)
	if len(listResp.Data) != 1 {
		t.Fatalf("账号数量 = %d, 期望 1", len(listResp.Data))
	}
}

func TestSetupEndpointRemoved(t *testing.T) {
	s := newTestServer(t, "test-token")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ui/setup", strings.NewReader(`{"username":"admin","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("不应提供用户创建接口, 实际 %d", w.Code)
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

func TestAccountDisableRestoreAndBatchEndpoints(t *testing.T) {
	s := newTestServer(t, "")
	h := s.Handler()
	cookie := loginAndGetCookie(t, s, "test-token")
	create := func(name string) string {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/accounts", strings.NewReader(`{"name":"`+name+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		h.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("创建账号失败: %d %s", w.Code, w.Body.String())
		}
		var resp struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		return resp.Data.ID
	}
	a, b := create("a"), create("b")

	post := func(path, body string) (int, string) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		h.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	if code, body := post("/api/accounts/"+a+"/deactivate", `{}`); code != http.StatusOK {
		t.Fatalf("禁用失败: %d %s", code, body)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/accounts/disabled", nil)
	req.AddCookie(cookie)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), a) {
		t.Fatalf("禁用列表异常: %d %s", w.Code, w.Body.String())
	}
	if code, body := post("/api/accounts/"+a+"/restore", `{}`); code != http.StatusOK || !strings.Contains(body, "pending") {
		t.Fatalf("恢复失败: %d %s", code, body)
	}
	if code, body := post("/api/accounts/batch/deactivate", `{"ids":["`+a+`","`+b+`","missing"]}`); code != http.StatusOK || !strings.Contains(body, `"changed":2`) || !strings.Contains(body, `"not_found":1`) {
		t.Fatalf("批量禁用结果异常: %d %s", code, body)
	}
	if code, body := post("/api/accounts/batch/delete", `{"ids":["`+a+`","`+b+`"]}`); code != http.StatusOK || !strings.Contains(body, `"deleted":2`) {
		t.Fatalf("批量删除结果异常: %d %s", code, body)
	}
	if code, _ := post("/api/accounts/batch/delete", `{"ids":[" "]}`); code != http.StatusBadRequest {
		t.Fatalf("空白 ID 应返回 400, 实际 %d", code)
	}
}

func TestOrganizerRoutesAndValidation(t *testing.T) {
	s := newTestServer(t, "")
	h := s.Handler()
	cookie := loginAndGetCookie(t, s, "test-token")
	request := func(method, path, body string) (int, string) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		h.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	code, body := request("POST", "/api/accounts", `{"name":"organizer"}`)
	if code != http.StatusCreated {
		t.Fatalf("创建账号失败: %d %s", code, body)
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal([]byte(body), &created)
	id := created.Data.ID
	if code, _ = request("POST", "/api/accounts/"+id+"/groups", `{"name":"  Team  "}`); code != http.StatusCreated {
		t.Fatalf("创建组状态 = %d", code)
	}
	var groupResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_, body = request("POST", "/api/accounts/"+id+"/groups", `{"name":"Second"}`)
	json.Unmarshal([]byte(body), &groupResp)
	groupID := groupResp.Data.ID
	if code, _ = request("PUT", "/api/accounts/"+id+"/alias-meta", `{"alias_id":"a1","group_id":"missing"}`); code != http.StatusNotFound {
		t.Fatalf("不存在组应返回 404, 实际 %d", code)
	}
	if code, _ = request("PUT", "/api/accounts/"+id+"/alias-meta", `{"alias_id":"a1","group_id":"`+groupID+`","note":" x "}`); code != http.StatusOK {
		t.Fatalf("更新元数据状态 = %d", code)
	}
	if code, _ = request("DELETE", "/api/accounts/"+id+"/groups/"+groupID, ``); code != http.StatusOK {
		t.Fatalf("删除组状态 = %d", code)
	}
	if code, body = request("GET", "/api/accounts/"+id+"/organizer", ``); code != http.StatusOK || strings.Contains(body, `"group_id":"`+groupID+`"`) {
		t.Fatalf("组删除后元数据异常: %d %s", code, body)
	}
	if code, _ = request("POST", "/api/accounts/"+id+"/groups", `{"name":"   "}`); code != http.StatusBadRequest {
		t.Fatalf("空组名应返回 400, 实际 %d", code)
	}
	if code, _ = request("GET", "/api/accounts/no-such/organizer", ``); code != http.StatusNotFound {
		t.Fatalf("不存在账号应返回 404, 实际 %d", code)
	}
}
