// Package hme - iCloud 认证模块
//
// 基于 Go-iClient 项目实现完整的 SRP (Secure Remote Password) 登录流程,
// 支持双重认证 (2FA),登录成功后提取 session token Cookie。
//
// 与原一次性同步 Login 不同,本实现拆分为两段式可恢复流程:
//
//	BeginLogin(username, password) → 完成 SRP 握手
//	  - 无需 2FA: 直接完成,Cookie 就绪
//	  - 需要 2FA: 返回 ErrOTPRequired,Client 内部保存中间状态
//	CompleteOTP(code)              → 提交 2FA 验证码,完成登录
//
// 调用方(HTTP 层)在收到 ErrOTPRequired 时把 *Client 存入会话存储,
// 等用户在网页输入验证码后用同一个 *Client 调用 CompleteOTP。
package hme

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/pbkdf2"

	http "github.com/bogdanfinn/fhttp"
	"icloud_distribution/internal/srp"
)

// AuthEndpoints iCloud 认证 API 端点
const (
	OAuthClientID = "d39ba9916b7251055b22c7f910e2ea796ee65e98b2ddecea8f5dde8d9d1a815d"

	// authorize URL,redirect_uri 按账号区域选择 (国区 www.icloud.com.cn)
	authStartFmt = "https://idmsa.apple.com/appleauth/auth/authorize/signin?frame_id=auth-%[1]s&language=en_US&skVersion=7&iframeId=auth-%[1]s&client_id=%[2]s&redirect_uri=%[3]s&response_type=code&response_mode=web_message&state=auth-%[1]s&authVersion=latest"

	authFederate   = "https://idmsa.apple.com/appleauth/auth/federate?isRememberMeEnabled=true"
	authInit       = "https://idmsa.apple.com/appleauth/auth/signin/init"
	authComplete   = "https://idmsa.apple.com/appleauth/auth/signin/complete?isRememberMeEnabled=true"
	submitSecurity   = "https://idmsa.apple.com/appleauth/auth/verify/%s/securitycode"
	authVerifyDevice = "https://idmsa.apple.com/appleauth/auth/verify/trusteddevice"
	authTrust        = "https://idmsa.apple.com/appleauth/auth/2sv/trust"
	authInfo         = "https://idmsa.apple.com/appleauth/auth"
	authVerifyPhone  = "https://idmsa.apple.com/appleauth/auth/verify/phone"
	authPhoneCode    = "https://idmsa.apple.com/appleauth/auth/verify/phone/securitycode"
	// accountLogin 端点不再硬编码: 国区账号必须走 setup.icloud.com.cn,
	// 统一通过 c.SetupURL() 按 c.Host 选择域名。
)

// webUserAgent 与 X-Apple-I-FD-Client-Info 中的 U 保持一致的 UA。
const webUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// ErrOTPRequired 表示账号启用了双重认证,需要调用 CompleteOTP 提交验证码。
var ErrOTPRequired = errors.New("账号启用了双重认证,需要提供 2FA 验证码")

// authState 保存认证过程中的状态
type authState struct {
	username   string
	frameId    string
	clientId   string
	srpSession string // init 响应的 "c" 字段 (SRP 会话标识),complete 必须回传
	authAttr   string
	sessionID  string
	scnt       string
	authToken  string
	trustToken string
	dsid       string
}

// BeginLogin 开始登录流程(阶段一)。
//
// 返回 nil 表示登录完成,Cookie 已就绪 (c.Cookies)。
// 返回 ErrOTPRequired 表示需要 2FA,此时必须对同一个 *Client 调用 CompleteOTP。
// 返回其他错误表示登录失败(密码错误、网络问题等)。
func (c *Client) BeginLogin(username, password string) error {
	state := &authState{username: username}

	// 1. 初始化 frameId 和 clientId
	if err := c.authStart(state); err != nil {
		return fmt.Errorf("auth start: %w", err)
	}

	// 2. 提交用户名
	if err := c.authFederate(state); err != nil {
		return fmt.Errorf("auth federate: %w", err)
	}

	// 3. SRP 协议初始化
	params := srp.GetParams(2048)
	params.NoUserNameInX = true
	srpClient := srp.NewSRPClient(params, nil)

	// 4. 获取 salt 和 B
	authInitResp, err := c.authInit(state, base64.StdEncoding.EncodeToString(srpClient.GetABytes()))
	if err != nil {
		return fmt.Errorf("auth init: %w", err)
	}

	// 5. 解码 salt 和 B
	bDec, err := base64.StdEncoding.DecodeString(authInitResp.B)
	if err != nil {
		return fmt.Errorf("decode B: %w", err)
	}
	saltDec, err := base64.StdEncoding.DecodeString(authInitResp.Salt)
	if err != nil {
		return fmt.Errorf("decode salt: %w", err)
	}

	// 6. 生成密码密钥
	passHash := sha256.Sum256([]byte(password))
	passKey := pbkdf2.Key(passHash[:], saltDec, authInitResp.Iteration, 32, sha256.New)

	// 7. 处理挑战
	srpClient.ProcessClientChanllenge([]byte(username), passKey, saltDec, bDec)

	// 8. 提交 SRP 响应 (可能触发 2FA)
	m1 := base64.StdEncoding.EncodeToString(srpClient.M1)
	m2 := base64.StdEncoding.EncodeToString(srpClient.M2)
	if err := c.authComplete(state, m1, m2); err != nil {
		if errors.Is(err, ErrOTPRequired) {
			// 保存中间状态,等待 CompleteOTP
			c.pendingAuth = state
			return ErrOTPRequired
		}
		return fmt.Errorf("auth complete: %w", err)
	}

	// 无需 2FA,直接完成
	return c.finishLogin(state)
}

// CompleteOTP 提交 2FA 验证码(阶段二),完成登录。
//
// 必须在 BeginLogin 返回 ErrOTPRequired 之后、对同一个 *Client 调用。
func (c *Client) CompleteOTP(code string) error {
	state := c.pendingAuth
	if state == nil {
		return fmt.Errorf("无待验证的登录会话,请重新发起登录")
	}

	if err := c.submitSecurityCode(state, code); err != nil {
		return err
	}

	err := c.finishLogin(state)
	if err == nil {
		c.pendingAuth = nil
	}
	return err
}

// finishLogin 登录收尾: 信任设备 → 获取 Web Cookie → 保存到 Client。
func (c *Client) finishLogin(state *authState) error {
	if err := c.getTrust(state); err != nil {
		return fmt.Errorf("get trust: %w", err)
	}

	if err := c.authenticateWeb(state); err != nil {
		return fmt.Errorf("authenticate web: %w", err)
	}

	cookies := c.extractSessionCookies()
	c.Cookies = cookies
	c.log("登录成功,获取到 %d 个 Cookie", len(cookies))
	if len(cookies) == 0 {
		// 调试: dump jar 中所有域的 cookie 名单,定位域归属问题
		for _, d := range []string{"https://idmsa.apple.com", "https://" + c.Host, "https://www." + c.Host, "https://setup." + c.Host} {
			if u, err := url.Parse(d); err == nil {
				var names []string
				for _, ck := range c.httpc.GetCookies(u) {
					names = append(names, ck.Name)
				}
				c.log("  jar[%s]: %v", d, names)
			}
		}
		return fmt.Errorf("登录成功但未获取到会话 Cookie")
	}
	return nil
}

// --- 认证流程的各步骤 ---

// authStart 初始化 frameId 和 clientId
func (c *Client) authStart(state *authState) error {
	state.frameId = strings.ToLower(uuid.New().String())
	state.clientId = OAuthClientID

	req, err := http.NewRequest("GET", fmt.Sprintf(authStartFmt, state.frameId, state.clientId, c.oauthRedirectURI()), nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", webUserAgent)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	state.authAttr = resp.Header.Get("X-Apple-Auth-Attributes")
	// 捕获服务端下发的初始会话凭证
	if scnt := resp.Header.Get("scnt"); scnt != "" {
		state.scnt = scnt
	}
	if sessionID := resp.Header.Get("X-Apple-ID-Session-Id"); sessionID != "" {
		state.sessionID = sessionID
	}
	return nil
}

// authFederate 提交用户名
func (c *Client) authFederate(state *authState) error {
	data := `{"accountName":"` + state.username + `","rememberMe":true}`
	req, err := http.NewRequest("POST", authFederate, bytes.NewReader([]byte(data)))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header = c.updateAuthHeaders(req.Header, state)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

// authInitResp authInit 响应
type authInitResp struct {
	Iteration int    `json:"iteration"`
	Salt      string `json:"salt"`
	Protocol  string `json:"protocol"`
	B         string `json:"b"`
	C         string `json:"c"`
}

// authInit 初始化 SRP 认证
func (c *Client) authInit(state *authState, a string) (*authInitResp, error) {
	reqBody := map[string]interface{}{
		"a":           a,
		"accountName": state.username,
		"protocols":   []string{"s2k", "s2k_fo"},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", authInit, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header = c.updateAuthHeaders(req.Header, state)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 捕获服务端下发的会话凭证,后续请求必须回传,否则 complete 会被拒绝 (-20101)
	c.captureSessionHeaders(state, resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	c.log("authInit 响应: %s", string(body))

	var result authInitResp
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	c.log("authInit: protocol=%s iteration=%d saltLen=%d scnt=%v", result.Protocol, result.Iteration, len(result.Salt), state.scnt != "")
	// 保存 SRP 会话标识,signin/complete 的 "c" 字段必须回传它
	// (不是 OAuth client id —— 上游项目这里一直用错,带 authType 后会被 400)
	state.srpSession = result.C
	return &result, nil
}

// captureSessionHeaders 从响应中捕获 scnt / X-Apple-ID-Session-Id。
// 每次响应(包括错误响应)都可能轮换这两个值,必须总是取最新的。
func (c *Client) captureSessionHeaders(state *authState, resp *http.Response) {
	if scnt := resp.Header.Get("scnt"); scnt != "" {
		state.scnt = scnt
	}
	if sessionID := resp.Header.Get("X-Apple-ID-Session-Id"); sessionID != "" {
		state.sessionID = sessionID
	}
}

// authComplete 提交 SRP 响应。
//
// 需要 2FA 时把 sessionID/scnt 存入 state 并返回 ErrOTPRequired。
//
// 关于 authType: 浏览器抓包中 "authType":"hsa2" 与 trustTokens 里的
// 真实信任令牌配套出现 (HSA2 信任续期); 空 trustTokens + hsa2 会被 400。
// 全新密码 SRP 登录不应携带 authType (icloud-photos-sync 同样不发)。
// 早期 -20101 的真正原因是 c 字段误传 OAuth client id。
func (c *Client) authComplete(state *authState, m1, m2 string) error {
	reqBody := map[string]interface{}{
		"accountName": state.username,
		"rememberMe":  true,
		"trustTokens": []string{},
		"m1":          m1,
		"c":           state.srpSession, // init 响应下发的 SRP 会话标识
		"m2":          m2,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", authComplete, bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header = c.updateAuthHeaders(req.Header, state)

	// 调试: dump 请求头 (敏感值打码),便于与浏览器抓包对比
	if c.Verbose {
		c.log("authComplete 请求头:")
		for k, vs := range req.Header {
			for _, v := range vs {
				c.log("  %s: %s", k, maskLong(v))
			}
		}
	}

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 每次响应(包括 401)都可能轮换 scnt/sessionID,必须取最新值,
	// 否则 -20101 后的 hsa2 重试会因旧 scnt 被 400 拒绝
	c.captureSessionHeaders(state, resp)

	// 调试: 失败时打印响应头 (区分 idmsa 应用层拒绝与边缘 WAF 拒绝)
	if c.Verbose && resp.StatusCode >= 400 {
		c.log("authComplete 失败响应头 (HTTP %d):", resp.StatusCode)
		for k, vs := range resp.Header {
			for _, v := range vs {
				c.log("  %s: %s", k, maskLong(v))
			}
		}
		if u, err := url.Parse("https://idmsa.apple.com"); err == nil {
			var names []string
			for _, ck := range c.httpc.GetCookies(u) {
				names = append(names, ck.Name)
			}
			c.log("当前 idmsa cookie: %v", names)
		}
	}

	switch resp.StatusCode {
	case 200:
		return nil
	case 409:
		// 需要 2FA: 保存会话凭证与 session token,等待用户输入验证码
		state.sessionID = resp.Header.Get("X-Apple-ID-Session-Id")
		state.scnt = resp.Header.Get("scnt")
		// 409 响应下发的 session token 是后续 MFA 请求的必需头部
		state.authToken = resp.Header.Get("X-Apple-Session-Token")
		return ErrOTPRequired
	case 403:
		return fmt.Errorf("用户名或密码错误")
	case 412:
		return fmt.Errorf("需要先在 appleid.apple.com 同意隐私条款")
	default:
		// 401 等: 带上 Apple 返回的错误正文 (通常包含具体原因)
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("auth complete 失败: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

// TrustedPhone 账号的受信任手机号。
type TrustedPhone struct {
	ID                 int    `json:"id"`
	NumberWithDialCode string `json:"numberWithDialCode"` // 脱敏显示,如 +86 138****1234
}

// TrustedPhones 获取账号的受信任手机号列表。必须在 BeginLogin 返回 ErrOTPRequired 之后调用。
func (c *Client) TrustedPhones() ([]TrustedPhone, error) {
	state := c.pendingAuth
	if state == nil {
		return nil, fmt.Errorf("无待验证的登录会话,请重新发起登录")
	}

	req, err := http.NewRequest("GET", authInfo, nil)
	if err != nil {
		return nil, err
	}
	req.Header = c.updateAuthHeaders(req.Header, state)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	c.captureSessionHeaders(state, resp)

	body, _ := io.ReadAll(resp.Body)
	// 完整打印: Apple 返回的手机号本身已脱敏,无敏感信息
	c.log("authInfo 响应 (HTTP %d):\n%s", resp.StatusCode, string(body))

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("获取手机号列表失败: HTTP %d", resp.StatusCode)
	}

	// 实际响应把列表嵌套在 phoneNumberVerification 里 (见抓包),
	// 兼容顶层平铺的旧结构
	var result struct {
		PhoneNumberVerification struct {
			TrustedPhoneNumbers []TrustedPhone `json:"trustedPhoneNumbers"`
		} `json:"phoneNumberVerification"`
		TrustedPhoneNumbers []TrustedPhone `json:"trustedPhoneNumbers"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析手机号列表失败: %w", err)
	}
	phones := result.PhoneNumberVerification.TrustedPhoneNumbers
	if len(phones) == 0 {
		phones = result.TrustedPhoneNumbers
	}
	return phones, nil
}

// SendSMS 向指定受信任手机号发送短信验证码 (PUT /verify/phone, 成功 200)。
func (c *Client) SendSMS(phoneID int) error {
	state := c.pendingAuth
	if state == nil {
		return fmt.Errorf("无待验证的登录会话,请重新发起登录")
	}

	body, _ := json.Marshal(map[string]interface{}{
		"phoneNumber": map[string]int{"id": phoneID},
		"mode":        "sms",
	})
	req, err := http.NewRequest("PUT", authVerifyPhone, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header = c.updateAuthHeaders(req.Header, state)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	c.captureSessionHeaders(state, resp)

	if resp.StatusCode != 200 {
		return fmt.Errorf("发送短信验证码失败: HTTP %d", resp.StatusCode)
	}
	return nil
}

// CompleteSMS 提交短信验证码 (POST /verify/phone/securitycode, 成功 200),完成登录。
func (c *Client) CompleteSMS(phoneID int, code string) error {
	state := c.pendingAuth
	if state == nil {
		return fmt.Errorf("无待验证的登录会话,请重新发起登录")
	}

	body, _ := json.Marshal(map[string]interface{}{
		"securityCode": map[string]string{"code": code},
		"phoneNumber":  map[string]int{"id": phoneID},
		"mode":         "sms",
	})
	req, err := http.NewRequest("POST", authPhoneCode, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header = c.updateAuthHeaders(req.Header, state)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	c.captureSessionHeaders(state, resp)

	if resp.StatusCode != 200 {
		return fmt.Errorf("短信验证码校验失败: HTTP %d", resp.StatusCode)
	}

	err = c.finishLogin(state)
	if err == nil {
		c.pendingAuth = nil
	}
	return err
}

// ResendOTP 请求 Apple 向受信任设备推送 2FA 验证码。
//
// 409 之后 Apple 通常会自动推送一次,本方法用于手动重发。
// 该端点在不同账号/区域的行为不一致 (icloud-photosync 用 PUT 且期望 202,
// 但实测国区 HSA2 账号 PUT 405 / POST 500),因此按候选组合依次探测,
// 任一返回 2xx 即视为成功。
func (c *Client) ResendOTP() error {
	state := c.pendingAuth
	if state == nil {
		return fmt.Errorf("无待验证的登录会话,请重新发起登录")
	}

	candidates := []struct{ method, url string }{
		{"GET", authVerifyDevice}, // 国区 HSA2 账号实测可用 (200)
		{"PUT", authVerifyDevice}, // icloud-photos-sync 的方式 (202)
		{"POST", authVerifyDevice},
		{"PUT", authVerifyDevice + "/securitycode"},
		{"GET", authVerifyDevice + "/securitycode"},
	}

	var lastStatus int
	for _, cand := range candidates {
		req, err := http.NewRequest(cand.method, cand.url, nil)
		if err != nil {
			return err
		}
		req.Header = c.updateAuthHeaders(req.Header, state)
		req.Header.Del("Content-Type") // 该系列端点要求无请求体

		resp, err := c.httpc.Do(req)
		if err != nil {
			return err
		}
		c.captureSessionHeaders(state, resp)
		lastStatus = resp.StatusCode
		c.log("ResendOTP 探测: %s %s → HTTP %d", cand.method, cand.url, resp.StatusCode)

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			resp.Body.Close()
			return nil
		}
		resp.Body.Close()
	}
	return fmt.Errorf("请求发送验证码失败: 全部组合均被拒绝 (最后 HTTP %d)", lastStatus)
}

// submitSecurityCode 提交 2FA 验证码
func (c *Client) submitSecurityCode(state *authState, code string) error {
	reqBody := map[string]interface{}{
		"securityCode": map[string]string{"code": code},
	}

	data, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", fmt.Sprintf(submitSecurity, "trusteddevice"), bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header = c.updateAuthHeaders(req.Header, state)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		return fmt.Errorf("2FA 验证失败: HTTP %d", resp.StatusCode)
	}

	if newScnt := resp.Header.Get("scnt"); newScnt != "" {
		state.scnt = newScnt
	}
	return nil
}

// getTrust 获取 trust token
func (c *Client) getTrust(state *authState) error {
	req, err := http.NewRequest("GET", authTrust, nil)
	if err != nil {
		return err
	}

	req.Header = c.updateAuthHeaders(req.Header, state)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		return fmt.Errorf("trust 失败: HTTP %d", resp.StatusCode)
	}

	state.authToken = resp.Header.Get("X-Apple-Session-Token")
	state.trustToken = resp.Header.Get("X-Apple-TwoSV-Trust-Token")
	return nil
}

// authenticateWeb 认证 iCloud Web 服务。
//
// 国区账号 (Host=icloud.com.cn) 必须请求 setup.icloud.com.cn,
// 请求 setup.icloud.com 会被拒绝并导致登录失败。
func (c *Client) authenticateWeb(state *authState) error {
	body := fmt.Sprintf(`{"dsWebAuthToken":"%s","accountCountryCode":"USA","extended_login":true,"trustToken":"%s"}`,
		state.authToken, state.trustToken)

	req, err := http.NewRequest("POST", c.SetupURL()+"/accountLogin", bytes.NewReader([]byte(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.Origin())
	req.Header.Set("Accept", "*/*")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("auth web 失败: HTTP %d (host=%s)", resp.StatusCode, c.Host)
	}

	var result struct {
		DsInfo struct {
			Dsid string `json:"dsid"`
		} `json:"dsInfo"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	state.dsid = result.DsInfo.Dsid

	// 复制 idmsa.apple.com 的 Cookie 到当前账号所属域名
	u1, _ := url.Parse("https://idmsa.apple.com")
	u2, _ := url.Parse("https://" + c.Host)
	cookies := c.httpc.GetCookies(u1)
	c.httpc.SetCookies(u2, cookies)

	return nil
}

// extractSessionCookies 提取 session token Cookie。
//
// Cookie 分散在多个域下: idmsa.apple.com (认证) 与 *.icloud.com / *.icloud.com.cn
// (Web 会话,由 accountLogin 的 Set-Cookie 写入)。逐一读取并合并,
// 避免单一域读取遗漏 (.com.cn 公共后缀下 jar 域匹配容易踩坑)。
func (c *Client) extractSessionCookies() map[string]string {
	cookies := make(map[string]string)
	domains := []string{
		"https://idmsa.apple.com",
		"https://" + c.Host,
		"https://www." + c.Host,
		"https://setup." + c.Host,
	}
	for _, d := range domains {
		u, err := url.Parse(d)
		if err != nil {
			continue
		}
		for _, cookie := range c.httpc.GetCookies(u) {
			if cookie.Value != "" {
				cookies[cookie.Name] = cookie.Value
			}
		}
	}
	return cookies
}

// updateAuthHeaders 按浏览器实际请求补齐认证头。
//
// 对照 icloud.com.cn 前端抓包,idmsa 要求一整套 X-Apple-OAuth-* / Frame-Id /
// FD-Client-Info 头,缺失时 signin/complete 会以 -20101 拒绝——
// 与密码是否正确无关。
func (c *Client) updateAuthHeaders(header http.Header, state *authState) http.Header {
	if state.scnt != "" {
		header.Set("scnt", state.scnt)
	}
	if state.sessionID != "" {
		header.Set("X-Apple-ID-Session-Id", state.sessionID)
	}
	if state.authAttr != "" {
		header.Set("X-Apple-Auth-Attributes", state.authAttr)
	}
	// MFA 阶段 (409 之后) 的请求必须携带 session token
	if state.authToken != "" {
		header.Set("X-Apple-Session-Token", state.authToken)
	}

	// OAuth 上下文 (对照浏览器抓包)
	header.Set("X-Apple-OAuth-Client-Id", state.clientId)
	header.Set("X-Apple-OAuth-Client-Type", "firstPartyAuth")
	header.Set("X-Apple-OAuth-Redirect-URI", c.oauthRedirectURI())
	header.Set("X-Apple-OAuth-Require-Grant-Code", "true")
	header.Set("X-Apple-OAuth-Response-Mode", "web_message")
	header.Set("X-Apple-OAuth-Response-Type", "code")
	header.Set("X-Apple-OAuth-State", state.frameId)
	header.Set("X-Apple-Widget-Key", state.clientId)
	header.Set("X-Apple-Frame-Id", state.frameId)
	header.Set("X-Apple-Domain-Id", "6")
	header.Set("X-Apple-Locale", "zh_CN")
	header.Set("X-Apple-Offer-Security-Upgrade", "1")
	header.Set("X-Apple-Privacy-Consent", "true")
	header.Set("X-Apple-Privacy-Consent-Accepted", "true")
	header.Set("X-Apple-I-FD-Client-Info", fdClientInfo())

	header.Set("X-Requested-With", "XMLHttpRequest")
	header.Set("Content-Type", "application/json")
	header.Set("Accept", "application/json")
	header.Set("Referer", "https://idmsa.apple.com/")
	header.Set("Origin", "https://idmsa.apple.com")
	header.Set("User-Agent", webUserAgent)

	return header
}

// maskLong 打码超长敏感值,保留前 12 个字符。
func maskLong(v string) string {
	if len(v) <= 20 {
		return v
	}
	return v[:12] + fmt.Sprintf("...<%d chars>", len(v))
}

// oauthRedirectURI 按账号区域返回 OAuth 回调 URI (国区 www.icloud.com.cn)。
func (c *Client) oauthRedirectURI() string {
	return "https://www." + c.Host
}

// fdClientInfo 生成与浏览器格式一致的 X-Apple-I-FD-Client-Info 指纹 JSON。
// F 字段是客户端生成的随机标识,Apple 不校验具体内容,但格式必须像。
func fdClientInfo() string {
	f := ".la44j1e3" + randToken(40) + "." + randToken(28) + "." + randToken(28) + "." + randToken(3)
	return fmt.Sprintf(`{"U":%q,"L":"zh-CN","Z":"GMT+08:00","V":"1.1","F":%q}`, webUserAgent, f)
}

// randToken 生成指定长度的随机标识串 (字母数字与 _ -)。
func randToken(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"
	b := make([]byte, n)
	rnd := make([]byte, n)
	if _, err := rand.Read(rnd); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = alphabet[int(rnd[i])%len(alphabet)]
	}
	return string(b)
}

// Validate 验证当前 Cookie 是否有效
func (c *Client) Validate() (bool, error) {
	if len(c.Cookies) == 0 {
		return false, fmt.Errorf("无 Cookie")
	}
	err := c.ValidateSession()
	if err != nil {
		return false, err
	}
	return true, nil
}
