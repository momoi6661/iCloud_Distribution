// Package mail - iCloud Web 邮件客户端
//
// 使用 Cookie 认证通过 iCloud Web API 读取邮件，
// 无需 App Password。基于 mccgateway 服务。
package mail

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/google/uuid"
)

// WebClientBuildNumber 是与浏览器一致的 mccgateway 邮件接口构建号。
const WebClientBuildNumber = "2624Build13"

// WebClient 是 iCloud Web 邮件客户端。
type WebClient struct {
	cookies       map[string]string
	dsid          string
	clientID      string
	mccGatewayURL string
	host          string // "icloud.com" 或 "icloud.com.cn"
	httpc         tls_client.HttpClient
	jar           tls_client.CookieJar
	Verbose       bool // 调试: 打印每个请求的状态

	debugDumpedBody   string // 已 dump 过的 thread/get 样本 (只 dump 一次)
	debugDumpedSearch string // 已 dump 过的 thread/search 样本 (只 dump 一次)
}

// logf 调试日志。
func (c *WebClient) logf(format string, args ...any) {
	if c.Verbose {
		fmt.Printf("  [mail] %s\n", fmt.Sprintf(format, args...))
	}
}

// NewWebClient 创建一个 Web 邮件客户端。
func NewWebClient(cookies map[string]string, dsid, host string) *WebClient {
	jar := tls_client.NewCookieJar()
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithClientProfile(profiles.Chrome_146),
		tls_client.WithCookieJar(jar),
		tls_client.WithNotFollowRedirects(),
	}

	httpc, _ := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)

	if host == "" {
		host = "icloud.com"
	}

	c := &WebClient{
		cookies:  cookies,
		dsid:     dsid,
		clientID: uuid.New().String(),
		host:     host,
		httpc:    httpc,
		jar:      jar,
	}

	// 设置 Cookie 到所有相关域名(确保跨域请求能传递 Cookie)
	if len(cookies) > 0 {
		suffix := "icloud.com"
		if host == "icloud.com.cn" {
			suffix = "icloud.com.cn"
		}
		domains := []string{
			"https://setup." + suffix,
			"https://www." + suffix,
			"https://p217-mccgateway." + suffix,
			"https://p217-maildomainws." + suffix,
		}
		for _, domain := range domains {
			u, _ := url.Parse(domain)
			httpCookies := make([]*http.Cookie, 0, len(cookies))
			for k, v := range cookies {
				httpCookies = append(httpCookies, &http.Cookie{
					Name:  k,
					Value: v,
					Path:  "/",
				})
			}
			jar.SetCookies(u, httpCookies)
		}
	}

	return c
}

// origin 返回当前账号对应的 Web Origin。
func (c *WebClient) origin() string {
	return "https://www." + c.host
}

// setCommonHeaders 设置与浏览器一致的通用请求头。
func (c *WebClient) setCommonHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.origin())
	req.Header.Set("Referer", c.origin()+"/")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")
}

// withParams 给 URL 追加 clientBuildNumber / clientId / dsid 查询参数。
func (c *WebClient) withParams(rawURL string) string {
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%sclientBuildNumber=%s&clientMasteringNumber=%s&clientId=%s&dsid=%s",
		rawURL, sep, WebClientBuildNumber, WebClientBuildNumber, c.clientID, c.dsid)
}

// resolveMccGateway 从 validate 响应中获取 mccgateway URL。
func (c *WebClient) resolveMccGateway() error {
	if c.mccGatewayURL != "" {
		return nil
	}

	setupURL := "https://setup." + c.host + "/setup/ws/1/validate"
	req, err := http.NewRequest("POST", c.withParams(setupURL), nil)
	if err != nil {
		return err
	}
	c.setCommonHeaders(req)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("validate 失败: HTTP %d - %s", resp.StatusCode, truncate(string(body), 200))
	}

	var parsed struct {
		Webservices struct {
			Mccgateway struct {
				URL string `json:"url"`
			} `json:"mccgateway"`
		} `json:"webservices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("解析 validate 响应失败: %w", err)
	}

	mccURL := parsed.Webservices.Mccgateway.URL
	if mccURL == "" {
		return fmt.Errorf("未找到 mccgateway URL,响应: %s", truncate(string(body), 200))
	}
	if !strings.HasPrefix(mccURL, "https://") {
		mccURL = "https://" + mccURL
	}
	// 去掉端口号(如 :443)——tls-client 的 cookie jar 按不带端口的 host 存储 Cookie,
	// 带端口的 URL 会导致 Cookie 无法附加,返回 403。
	if u, err := url.Parse(mccURL); err == nil && u.Host != "" {
		u.Host = u.Hostname()
		mccURL = u.String()
	}
	c.mccGatewayURL = strings.TrimRight(mccURL, "/")

	// 把 Cookie 灌给解析出的真实网域 (分区号因账号而异,p217/p205/...,
	// 只灌写死的域名会导致网关请求无 Cookie → 403)
	c.seedCookies(c.mccGatewayURL)
	c.logf("mccgateway 解析成功: %s", c.mccGatewayURL)
	return nil
}

// seedCookies 把账号 Cookie 灌入指定 URL 对应域的 jar。
func (c *WebClient) seedCookies(rawURL string) {
	if len(c.cookies) == 0 {
		return
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	httpCookies := make([]*http.Cookie, 0, len(c.cookies))
	for k, v := range c.cookies {
		httpCookies = append(httpCookies, &http.Cookie{Name: k, Value: v, Path: "/"})
	}
	c.jar.SetCookies(u, httpCookies)
}

// PostRaw 向 mailws2 的指定路径 POST 原始 payload (调试用)。
func (c *WebClient) PostRaw(path, payload string) (string, error) {
	if err := c.resolveMccGateway(); err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", c.withParams(c.mccGatewayURL+path), strings.NewReader(payload))
	if err != nil {
		return "", err
	}
	c.setCommonHeaders(req)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	return string(body), nil
}

// SearchRaw 执行原始 thread/search 请求并返回未解析的响应 (调试用)。
func (c *WebClient) SearchRaw(payload string) (string, error) {
	if err := c.resolveMccGateway(); err != nil {
		return "", err
	}

	searchURL := c.withParams(c.mccGatewayURL + "/mailws2/v1/thread/search")
	req, err := http.NewRequest("POST", searchURL, strings.NewReader(payload))
	if err != nil {
		return "", err
	}
	c.setCommonHeaders(req)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	return string(body), nil
}

// threadSearchResp 是 thread/search 接口的响应结构。
type threadSearchResp struct {
	TotalThreadsReturned int `json:"totalThreadsReturned"`
	ThreadList           []struct {
		ThreadID  string   `json:"threadId"`
		Subject   string   `json:"subject"`
		Senders   []string `json:"senders"`
		Preview   string   `json:"preview"`
		Timestamp int64    `json:"timestamp"`
	} `json:"threadList"`
}

// search 执行 thread/search 请求,返回解析后的邮件列表。
func (c *WebClient) search(payload string) ([]Message, error) {
	if err := c.resolveMccGateway(); err != nil {
		return nil, err
	}

	searchURL := c.withParams(c.mccGatewayURL + "/mailws2/v1/thread/search")
	req, err := http.NewRequest("POST", searchURL, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	c.setCommonHeaders(req)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("获取邮件失败: HTTP %d - %s", resp.StatusCode, truncate(string(body), 300))
	}
	if strings.Contains(string(body), `"success":false`) {
		return nil, fmt.Errorf("获取邮件失败: %s", truncate(string(body), 300))
	}
	// 调试: 打印一次原始搜索响应 (含 folderStatus)
	if c.Verbose && c.debugDumpedSearch == "" {
		c.debugDumpedSearch = string(body)
		c.logf("thread/search 原始响应样本: %.1200s", body)
	}

	var result threadSearchResp
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析邮件响应失败: %w", err)
	}
	c.logf("thread/search → %d 个 thread", len(result.ThreadList))

	messages := make([]Message, 0, len(result.ThreadList))
	for _, t := range result.ThreadList {
		from := ""
		if len(t.Senders) > 0 {
			from = t.Senders[0]
		}
		date := ""
		if t.Timestamp > 0 {
			date = time.UnixMilli(t.Timestamp).Format(time.RFC3339)
		}
		messages = append(messages, Message{
			ID:      t.ThreadID,
			From:    from,
			Subject: t.Subject,
			Preview: t.Preview,
			Date:    date,
		})
	}
	return messages, nil
}

// ListInbox 列出收件箱邮件。
func (c *WebClient) ListInbox(limit int) ([]Message, error) {
	return c.ListFolder("INBOX", limit)
}

// ListFolder 列出指定文件夹的邮件 (INBOX/Junk/Sent Messages/Trash 等)。
func (c *WebClient) ListFolder(folder string, limit int) ([]Message, error) {
	payload := fmt.Sprintf(`{"responseType":"THREAD_DIGEST","includeFolderStatus":true,"maxResults":%d,"sessionHeaders":{"folder":%q,"modseq":null,"threadmodseq":null,"condstore":1,"qresync":1,"threadmode":1}}`, limit, folder)
	return c.search(payload)
}

// SearchMails 搜索邮件。query 为空时等价于 ListInbox。
func (c *WebClient) SearchMails(query string, limit int) ([]Message, error) {
	if query == "" {
		return c.ListInbox(limit)
	}
	payload := fmt.Sprintf(`{"responseType":"THREAD_DIGEST","includeFolderStatus":false,"maxResults":%d,"query":%q,"sessionHeaders":{"folder":"INBOX","condstore":1,"qresync":1,"threadmode":1}}`, limit, query)
	return c.search(payload)
}

// messageMetadata 是 thread/get 返回的单封邮件元数据 (含真实收件人)。
type messageMetadata struct {
	UID      string   `json:"uid"`
	Folder   string   `json:"folder"`
	Subject  string   `json:"subject"`
	From     []string `json:"from"`
	To       []string `json:"to"`
	Cc       []string `json:"cc"`
	SentDate string   `json:"sentDate"`
}

// threadGet 获取一个 thread 的邮件元数据 (含收件人)。
// 对应浏览器邮件 App 打开邮件时调用的 /mailws2/v1/thread/get。
//
// 实测 Junk 文件夹的 thread 用 folder=Junk 查询返回空列表,
// 元数据查询与文件夹无关,统一用 INBOX 作为会话上下文;
// 为空时回退传入的 folder 再试一次。
func (c *WebClient) threadGet(threadID, folder string) ([]messageMetadata, error) {
	meta, err := c.threadGetWithFolder(threadID, "INBOX")
	if err == nil && len(meta) > 0 {
		return meta, nil
	}
	if folder != "INBOX" {
		return c.threadGetWithFolder(threadID, folder)
	}
	return meta, err
}

// threadGetWithFolder 以指定文件夹为会话上下文查询 thread 元数据。
func (c *WebClient) threadGetWithFolder(threadID, folder string) ([]messageMetadata, error) {
	if err := c.resolveMccGateway(); err != nil {
		return nil, err
	}

	payload := fmt.Sprintf(`{"threadId":%q,"includeLabelIds":false,"sessionHeaders":{"folder":%q,"condstore":1,"qresync":1,"threadmode":1}}`, threadID, folder)
	getURL := c.withParams(c.mccGatewayURL + "/mailws2/v1/thread/get")
	req, err := http.NewRequest("POST", getURL, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	c.setCommonHeaders(req)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	c.logf("thread/get %s (folder=%s) → HTTP %d, %d bytes", threadID, folder, resp.StatusCode, len(body))
	if resp.StatusCode != 200 {
		c.logf("thread/get 响应: %s", truncate(string(body), 400))
		return nil, fmt.Errorf("thread/get 失败: HTTP %d", resp.StatusCode)
	}
	if len(body) < 300 {
		// 异常短的响应: 完整打印便于诊断
		c.logf("thread/get 响应: %s", truncate(string(body), 400))
	}
	// 调试: 打印首个 thread 的原始响应,便于核对字段结构
	if c.Verbose && c.debugDumpedBody == "" {
		c.debugDumpedBody = string(body)
		c.logf("thread/get 原始响应样本: %.800s", body)
	}

	var result struct {
		MessageMetadataList []messageMetadata `json:"messageMetadataList"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析 thread/get 响应失败: %w", err)
	}
	return result.MessageMetadataList, nil
}

// FindByAlias 查找发给指定别名的邮件。
//
// 扫描 INBOX + Junk 两个文件夹 (HME 转发邮件可能被 iCloud 判为垃圾邮件),
// 对摘要逐条调 thread/get 补全元数据 (并发 6),按真实 To/Cc 过滤——
// 摘要 (THREAD_DIGEST) 不含收件人,纯本地过滤必然漏检。
func (c *WebClient) FindByAlias(alias string, limit int) ([]Message, error) {
	lower := strings.ToLower(alias)
	seen := make(map[string]bool)
	type indexedMessage struct {
		idx int
		msg Message
	}
	var mu sync.Mutex
	matched := make([]indexedMessage, 0, limit)

	add := func(idx int, m Message) {
		mu.Lock()
		if !seen[m.ID] {
			seen[m.ID] = true
			matched = append(matched, indexedMessage{idx, m})
		}
		mu.Unlock()
	}

	batchSize := limit * 2
	if batchSize < 50 {
		batchSize = 50
	}

	// 扫描的文件夹: 收件箱 + 垃圾邮件
	folders := []string{"INBOX", "Junk"}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6) // 并发上限,避免触发风控
	var errCount int32
	idx := 0

	for _, folder := range folders {
		raw, err := c.ListFolder(folder, batchSize)
		if err != nil {
			c.logf("读取文件夹 %s 失败: %v", folder, err)
			continue
		}
		for _, digest := range raw {
			idx++
			wg.Add(1)
			go func(idx int, folder string, digest Message) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				meta, err := c.threadGet(digest.ID, folder)
				if err != nil || len(meta) == 0 {
					atomic.AddInt32(&errCount, 1)
					return
				}
				for _, mm := range meta {
					recipients := append(append([]string{}, mm.To...), mm.Cc...)
					hit := false
					for _, rcpt := range recipients {
						if strings.Contains(strings.ToLower(rcpt), lower) {
							hit = true
							break
						}
					}
					if !hit {
						continue
					}
					from := digest.From
					if len(mm.From) > 0 {
						from = mm.From[0]
					}
					to := ""
					if len(mm.To) > 0 {
						to = mm.To[0]
					}
					add(idx, Message{
						ID:      digest.ID,
						From:    from,
						To:      to,
						Subject: digest.Subject,
						Preview: digest.Preview,
						Date:    digest.Date,
					})
				}
			}(idx, folder, digest)
		}
	}
	wg.Wait()
	c.logf("补全元数据: %d 失败/空, 匹配 %d", errCount, len(matched))

	// 恢复时间顺序
	sort.SliceStable(matched, func(a, b int) bool { return matched[a].idx < matched[b].idx })
	filtered := make([]Message, 0, limit)
	for _, im := range matched {
		filtered = append(filtered, im.msg)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered, nil
}

// SetGatewayURL 注入已解析的 mccgateway URL (跳过重复的 validate 解析)。
// 同时把 Cookie 灌给该域,保证网关请求携带认证信息。
func (c *WebClient) SetGatewayURL(url string) {
	c.mccGatewayURL = url
	c.seedCookies(url)
}

// GatewayURL 返回已解析的 mccgateway URL (未解析时为空)。
func (c *WebClient) GatewayURL() string {
	return c.mccGatewayURL
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
