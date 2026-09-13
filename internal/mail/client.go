// Package mail 实现 iCloud 邮件 IMAP 读取客户端。
//
// 通过 Apple 应用专用密码连接 imap.mail.me.com:993,
// 拉取隐私邮箱别名收到的邮件。对应原 Python 项目 icloud_mail.py。
package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	stdhtml "html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/charset"
	xhtml "golang.org/x/net/html"
)

const (
	IMAPServer = "imap.mail.me.com"
	IMAPPort   = 993
)

// Message 是一封邮件的摘要信息。
type Message struct {
	ID      string `json:"id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Date    string `json:"date"`
	Preview string `json:"preview"`
	Code    string `json:"code,omitempty"`
	Folder  string `json:"folder,omitempty"` // 所在文件夹 (IMAP uid 按文件夹生效,读取正文时需要)
}

// FullMessage 是一封邮件的完整内容(含正文)。
type FullMessage struct {
	Message
	Body        string `json:"body"`
	ContentType string `json:"content_type"`
}

// Client 是 iCloud 邮件 IMAP 客户端。
type Client struct {
	host        string
	port        int
	email       string
	appPassword string
	cli         *client.Client
}

// NewClient 创建 IMAP 客户端。需在调用其它方法前先 Connect。
func NewClient(appleID, appPassword string) *Client {
	return NewGenericClient(IMAPServer, IMAPPort, appleID, appPassword)
}

// NewGenericClient 创建一个使用 TLS 的通用 IMAP 客户端。
// 用于读取隐藏邮箱所转发到的 Gmail、QQ Mail、Outlook 等邮箱。
func NewGenericClient(host string, port int, email, password string) *Client {
	return &Client{host: host, port: port, email: email, appPassword: password}
}

// Connect 连接并登录 IMAP 服务器。
func (c *Client) Connect() error {
	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	cli, err := client.DialTLS(addr, nil)
	if err != nil {
		return fmt.Errorf("IMAP 连接失败: %w", err)
	}
	if err := cli.Login(c.email, c.appPassword); err != nil {
		_ = cli.Logout()
		return fmt.Errorf("IMAP 登录失败 — 请检查邮箱地址和应用专用密码/授权码: %s — %w", c.email, err)
	}
	c.cli = cli
	return nil
}

var forwardedEmailPattern = regexp.MustCompile(`(?i)[A-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[A-Z0-9.-]+\.[A-Z]{2,}`)

// ListForwardedByAlias 从转发邮箱中读取匹配指定隐藏邮箱地址的邮件摘要。
// 优先按 To 邮件头搜索，找不到时才回退全文搜索；列表阶段只批量读取
// 信封、必要邮件头和最多 2KB 正文开头，完整正文留到用户点击后获取。
func (c *Client) ListForwardedByAlias(alias string, folders []string, limit, days int) ([]Message, error) {
	if c.cli == nil {
		return nil, fmt.Errorf("未连接")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if len(folders) == 0 {
		folders = []string{"INBOX"}
	}
	target := strings.ToLower(strings.TrimSpace(alias))
	var out []Message
	var lastErr error
	for _, folder := range folders {
		folder = strings.TrimSpace(folder)
		if folder == "" {
			continue
		}
		if _, err := c.cli.Select(folder, true); err != nil {
			lastErr = err
			continue
		}
		uids, err := c.searchForwardedUIDs(target, days)
		if err != nil {
			lastErr = err
			continue
		}
		if len(uids) > limit {
			uids = uids[len(uids)-limit:]
		}
		messages, err := c.fetchForwardedMessages(uids, folder, target)
		if err != nil {
			lastErr = err
			continue
		}
		out = append(out, messages...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	if len(out) > limit {
		out = out[:limit]
	}
	if len(out) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return out, nil
}

func (c *Client) searchForwardedUIDs(target string, days int) ([]uint32, error) {
	criteria := imap.NewSearchCriteria()
	if target != "" {
		criteria.Header.Add("To", target)
	}
	if days > 0 {
		criteria.Since = time.Now().AddDate(0, 0, -days)
	}
	uids, err := c.cli.UidSearch(criteria)
	if err != nil || len(uids) > 0 || target == "" {
		return uids, err
	}

	// 某些转发服务会改写 To，仅在必要时进行兼容性的全文搜索。
	criteria = imap.NewSearchCriteria()
	criteria.Text = []string{target}
	if days > 0 {
		criteria.Since = time.Now().AddDate(0, 0, -days)
	}
	return c.cli.UidSearch(criteria)
}

// CountForwardedByAlias 只执行 IMAP SEARCH，不下载邮件内容。
func (c *Client) CountForwardedByAlias(alias string, folders []string, days int) (int, error) {
	if c.cli == nil {
		return 0, fmt.Errorf("未连接")
	}
	if len(folders) == 0 {
		folders = []string{"INBOX"}
	}
	target := strings.ToLower(strings.TrimSpace(alias))
	total := 0
	var lastErr error
	for _, folder := range folders {
		if _, err := c.cli.Select(strings.TrimSpace(folder), true); err != nil {
			lastErr = err
			continue
		}
		criteria := imap.NewSearchCriteria()
		if target != "" {
			criteria.Text = []string{target}
		}
		if days > 0 {
			criteria.Since = time.Now().AddDate(0, 0, -days)
		}
		uids, err := c.cli.UidSearch(criteria)
		if err != nil {
			lastErr = err
			continue
		}
		total += len(uids)
	}
	if total == 0 && lastErr != nil {
		return 0, lastErr
	}
	return total, nil
}

func (c *Client) fetchForwardedMessages(uids []uint32, folder, target string) ([]Message, error) {
	if len(uids) == 0 {
		return []Message{}, nil
	}
	seqset := new(imap.SeqSet)
	for _, uid := range uids {
		seqset.AddNum(uid)
	}
	section := &imap.BodySectionName{Peek: true, BodyPartName: imap.BodyPartName{
		Specifier: imap.HeaderSpecifier,
		Fields: []string{
			"From", "To", "Cc", "Subject", "Date", "Delivered-To", "X-Original-To",
			"Envelope-To", "Resent-To", "Original-Recipient", "X-Envelope-To", "X-Forwarded-To", "Received",
			"Content-Type", "Content-Transfer-Encoding",
		},
	}}
	previewSection := &imap.BodySectionName{Peek: true, Partial: []int{0, 8192}, BodyPartName: imap.BodyPartName{Specifier: imap.TextSpecifier}}
	rawPreviewSection := &imap.BodySectionName{Peek: true, Partial: []int{0, 16384}}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchInternalDate, section.FetchItem(), previewSection.FetchItem(), rawPreviewSection.FetchItem()}
	messages := make(chan *imap.Message, len(uids))
	done := make(chan error, 1)
	go func() { done <- c.cli.UidFetch(seqset, items, messages) }()
	var out []Message
	for msg := range messages {
		rawReader := msg.GetBody(section)
		if rawReader == nil {
			continue
		}
		raw, err := io.ReadAll(rawReader)
		if err != nil {
			continue
		}
		summary, matches, err := parseForwardedSummary(msg, raw, folder, target)
		if err == nil && matches {
			if previewReader := msg.GetBody(previewSection); previewReader != nil {
				summary.Preview = previewFromRaw(previewReader)
				summary.Code = ExtractVerificationCode(summary.Subject + "\n" + summary.Preview)
			}
			if summary.Preview == "" {
				if rawReader := msg.GetBody(rawPreviewSection); rawReader != nil {
					summary.Preview = previewFromRaw(rawReader)
					summary.Code = ExtractVerificationCode(summary.Subject + "\n" + summary.Preview)
				}
			}
			out = append(out, *summary)
		}
	}
	if err := <-done; err != nil {
		return nil, err
	}
	return out, nil
}

func parseForwardedSummary(msg *imap.Message, raw []byte, folder, target string) (*Message, bool, error) {
	parsed, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, false, err
	}
	headerText := strings.Join([]string{
		parsed.Header.Get("Delivered-To"), parsed.Header.Get("X-Original-To"),
		parsed.Header.Get("Envelope-To"), parsed.Header.Get("Resent-To"),
		parsed.Header.Get("Original-Recipient"), parsed.Header.Get("X-Envelope-To"),
		parsed.Header.Get("X-Forwarded-To"), parsed.Header.Get("Received"),
		parsed.Header.Get("To"), parsed.Header.Get("Cc"),
	}, "\n")
	base := toMessage(msg)
	base.Folder = folder
	if base.Subject == "" {
		base.Subject = decodeHeader(parsed.Header.Get("Subject"))
	}
	if base.From == "" {
		base.From = decodeHeader(parsed.Header.Get("From"))
	}
	if base.To == "" {
		base.To = decodeHeader(parsed.Header.Get("To"))
	}
	if base.Date == "" {
		if parsedDate, dateErr := mail.ParseDate(parsed.Header.Get("Date")); dateErr == nil {
			base.Date = parsedDate.Format(time.RFC3339)
		}
	}
	base.Code = ExtractVerificationCode(base.Subject)
	return &base, target == "" || containsExactEmail(headerText, target), nil
}

func parseForwardedMessage(msg *imap.Message, raw []byte, folder, target string) (*FullMessage, bool, error) {
	parsed, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, false, err
	}
	headerText := strings.Join([]string{
		parsed.Header.Get("Delivered-To"), parsed.Header.Get("X-Original-To"),
		parsed.Header.Get("Envelope-To"), parsed.Header.Get("Resent-To"),
		parsed.Header.Get("To"), parsed.Header.Get("Cc"),
	}, "\n")
	body, bodyErr := readBody(parsed)
	if bodyErr != nil {
		body = ""
	}
	matches := target == "" || containsExactEmail(headerText+"\n"+body, target)
	base := toMessage(msg)
	base.Folder = folder
	if base.Subject == "" {
		base.Subject = decodeHeader(parsed.Header.Get("Subject"))
	}
	if base.From == "" {
		base.From = decodeHeader(parsed.Header.Get("From"))
	}
	if base.To == "" {
		base.To = decodeHeader(parsed.Header.Get("To"))
	}
	if base.Date == "" {
		if parsedDate, dateErr := mail.ParseDate(parsed.Header.Get("Date")); dateErr == nil {
			base.Date = parsedDate.Format(time.RFC3339)
		}
	}
	preview := strings.Join(strings.Fields(body), " ")
	runes := []rune(preview)
	if len(runes) > 240 {
		preview = string(runes[:240])
	}
	base.Preview = preview
	base.Code = ExtractVerificationCode(base.Subject + "\n" + body)
	return &FullMessage{Message: base, Body: strings.TrimSpace(body), ContentType: "text/plain"}, matches, nil
}

func containsExactEmail(value, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, candidate := range forwardedEmailPattern.FindAllString(value, -1) {
		if strings.EqualFold(candidate, target) {
			return true
		}
	}
	return false
}

// Disconnect 登出并关闭连接。
func (c *Client) Disconnect() {
	if c.cli != nil {
		_ = c.cli.Logout()
		c.cli = nil
	}
}

// Noop 发送 NOOP 保活,用于连接池健康检查。
func (c *Client) Noop() error {
	if c.cli == nil {
		return fmt.Errorf("未连接")
	}
	return c.cli.Noop()
}

// Connected 返回当前是否有活动连接。
func (c *Client) Connected() bool { return c.cli != nil }

// SelectFolder 选择文件夹 (调试/底层操作用)。
func (c *Client) SelectFolder(folder string) error {
	if c.cli == nil {
		return fmt.Errorf("未连接")
	}
	_, err := c.cli.Select(folder, true)
	return err
}

// UidFetch 底层 UID 拉取 (调试用)。
func (c *Client) UidFetch(seqset *imap.SeqSet, items []imap.FetchItem, messages chan *imap.Message) error {
	if c.cli == nil {
		return fmt.Errorf("未连接")
	}
	return c.cli.UidFetch(seqset, items, messages)
}

// InboxCount 返回收件箱邮件总数。
func (c *Client) InboxCount() (int, error) {
	if c.cli == nil {
		return 0, fmt.Errorf("未连接")
	}
	mbox, err := c.cli.Select("INBOX", false)
	if err != nil {
		return 0, err
	}
	return int(mbox.Messages), nil
}

// ListInbox 拉取收件箱最近 limit 封邮件摘要。
//
// days 用于过滤只看近 N 天的邮件(0 表示不限制)。
// 返回按时间倒序排列。
// MailFolders 读邮件时扫描的文件夹。
// HME 转发到 iCloud 邮箱的邮件可能被 iCloud 判为垃圾邮件,
// 只查 INBOX 会漏掉 (实测如此)。
var MailFolders = []string{"INBOX", "Junk"}

func (c *Client) ListInbox(limit int, days int) ([]Message, error) {
	return c.ListFolder("INBOX", limit, days)
}

// ListFolder 列出指定文件夹的邮件。
func (c *Client) ListFolder(folder string, limit int, days int) ([]Message, error) {
	if c.cli == nil {
		return nil, fmt.Errorf("未连接")
	}
	if limit <= 0 {
		limit = 50
	}

	mbox, err := c.cli.Select(folder, true)
	if err != nil {
		return nil, err
	}
	total := int(mbox.Messages)
	if total == 0 {
		return []Message{}, nil
	}

	// 计算起始序号(只取最近 limit 封)
	from := uint32(1)
	if uint32(limit) < mbox.Messages {
		from = mbox.Messages - uint32(limit) + 1
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(from, mbox.Messages)

	// 列表阶段只取标题、发件人、收件人和时间。正文仅在用户点击邮件后读取。
	items := []imap.FetchItem{
		imap.FetchUid,
		imap.FetchEnvelope,
		imap.FetchInternalDate,
	}

	messages := make(chan *imap.Message, limit)
	done := make(chan error, 1)
	go func() {
		done <- c.cli.Fetch(seqset, items, messages)
	}()

	var out []Message
	for msg := range messages {
		m := toMessage(msg)
		m.Folder = folder
		// days 过滤
		if days > 0 {
			if t, err := time.Parse(time.RFC1123Z, m.Date); err == nil {
				if time.Since(t) > time.Duration(days)*24*time.Hour {
					continue
				}
			}
		}
		out = append(out, m)
	}
	if err := <-done; err != nil {
		return nil, err
	}
	return out, nil
}

// FindByRecipient 查找发给指定隐私邮箱别名的邮件 (扫描 INBOX + Junk)。
//
// 每个文件夹先尝试 IMAP TO 搜索;失败则拉取后本地过滤。
func (c *Client) FindByRecipient(recipient string, limit int, days int) ([]Message, error) {
	if c.cli == nil {
		return nil, fmt.Errorf("未连接")
	}
	if limit <= 0 {
		limit = 20
	}

	var out []Message
	var lastErr error
	for _, folder := range MailFolders {
		msgs, err := c.findByRecipientInFolder(folder, recipient, limit, days)
		if err != nil {
			lastErr = err
			continue
		}
		out = append(out, msgs...)
	}

	// 按日期倒序合并,截断到 limit
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	if len(out) > limit {
		out = out[:limit]
	}
	if len(out) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return out, nil
}

// CountByRecipient 只搜索 UID 并返回命中数量，不抓取标题、摘要、正文或附件。
func (c *Client) CountByRecipient(recipient string, days int) (int, error) {
	if c.cli == nil {
		return 0, fmt.Errorf("未连接")
	}
	total := 0
	var lastErr error
	for _, folder := range MailFolders {
		if _, err := c.cli.Select(folder, true); err != nil {
			lastErr = err
			continue
		}
		criteria := imap.NewSearchCriteria()
		criteria.Header.Add("To", recipient)
		if days > 0 {
			criteria.Since = time.Now().AddDate(0, 0, -days)
		}
		uids, err := c.cli.UidSearch(criteria)
		if err != nil {
			lastErr = err
			continue
		}
		total += len(uids)
	}
	if total == 0 && lastErr != nil {
		return 0, lastErr
	}
	return total, nil
}

// findByRecipientInFolder 在单个文件夹内按收件人查找。
func (c *Client) findByRecipientInFolder(folder, recipient string, limit int, days int) ([]Message, error) {
	// 先尝试服务端 TO 搜索
	if _, err := c.cli.Select(folder, true); err != nil {
		return nil, err
	}
	criteria := imap.NewSearchCriteria()
	criteria.Header.Add("To", recipient)
	if days > 0 {
		since := time.Now().AddDate(0, 0, -days)
		criteria.Since = since
	}
	uids, err := c.cli.UidSearch(criteria)
	if err == nil && len(uids) > 0 {
		msgs, err := c.fetchByUIDs(uids, limit)
		if err != nil {
			return nil, err
		}
		for i := range msgs {
			msgs[i].Folder = folder
		}
		return msgs, nil
	}

	// fallback: 拉取后本地过滤
	all, err := c.ListFolder(folder, limit*3, days)
	if err != nil {
		return nil, err
	}
	recipient = strings.ToLower(recipient)
	var out []Message
	for _, m := range all {
		if strings.Contains(strings.ToLower(m.To), recipient) {
			out = append(out, m)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (c *Client) fetchByUIDs(uids []uint32, limit int) ([]Message, error) {
	if len(uids) == 0 {
		return []Message{}, nil
	}
	// 取最近 limit 条(UID 倒序)
	if len(uids) > limit {
		uids = uids[len(uids)-limit:]
	}
	seqset := new(imap.SeqSet)
	for _, uid := range uids {
		seqset.AddNum(uid)
	}

	// 列表阶段一次批量取信封和最多 2KB 正文开头作为摘要；完整正文
	// 仍严格等到用户点击后再读取，避免逐封追加网络请求。
	previewSection := &imap.BodySectionName{Peek: true, Partial: []int{0, 8192}, BodyPartName: imap.BodyPartName{Specifier: imap.TextSpecifier}}
	rawPreviewSection := &imap.BodySectionName{Peek: true, Partial: []int{0, 16384}}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchInternalDate, previewSection.FetchItem(), rawPreviewSection.FetchItem()}
	messages := make(chan *imap.Message, len(uids))
	done := make(chan error, 1)
	go func() {
		done <- c.cli.UidFetch(seqset, items, messages)
	}()

	var out []Message
	for msg := range messages {
		m := toMessage(msg)
		if previewReader := msg.GetBody(previewSection); previewReader != nil {
			m.Preview = previewFromRaw(previewReader)
			m.Code = ExtractVerificationCode(m.Subject + "\n" + m.Preview)
		}
		if m.Preview == "" {
			if rawReader := msg.GetBody(rawPreviewSection); rawReader != nil {
				m.Preview = previewFromRaw(rawReader)
				m.Code = ExtractVerificationCode(m.Subject + "\n" + m.Preview)
			}
		}
		out = append(out, m)
	}
	if err := <-done; err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) fetchPreview(uid uint32, part selectedBodyPart) (string, error) {
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	section := &imap.BodySectionName{Peek: true, Partial: []int{0, 4096}, BodyPartName: imap.BodyPartName{Path: part.path}}
	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() { done <- c.cli.UidFetch(seqset, []imap.FetchItem{section.FetchItem()}, messages) }()
	msg := <-messages
	if err := <-done; err != nil {
		return "", err
	}
	if msg == nil || msg.GetBody(section) == nil {
		return "", nil
	}
	return decodePreviewBody(msg.GetBody(section), part.contentType, part.encoding, part.charset)
}

// GetFull 获取单封邮件的完整内容(含正文)。folder 为邮件所在文件夹
// (IMAP uid 按文件夹生效),空字符串默认 INBOX。
func (c *Client) GetFull(uid uint32, folder string) (*FullMessage, error) {
	if c.cli == nil {
		return nil, fmt.Errorf("未连接")
	}
	if folder == "" {
		folder = "INBOX"
	}
	if _, err := c.cli.Select(folder, true); err != nil {
		return nil, err
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)

	// 第一次只取结构和信封，先定位真正的正文 part，避免下载附件。
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchInternalDate, imap.FetchBodyStructure}
	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- c.cli.UidFetch(seqset, items, messages)
	}()

	msg := <-messages
	if err := <-done; err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, fmt.Errorf("邮件不存在 (uid=%d)", uid)
	}

	if msg.BodyStructure == nil {
		return nil, fmt.Errorf("邮件缺少正文结构 (uid=%d)", uid)
	}
	part, ok := selectBodyPart(msg.BodyStructure)
	if !ok {
		return &FullMessage{Message: toMessage(msg), ContentType: "text/plain"}, nil
	}

	section := &imap.BodySectionName{Peek: true, BodyPartName: imap.BodyPartName{Path: part.path}}
	bodyMessages := make(chan *imap.Message, 1)
	bodyDone := make(chan error, 1)
	go func() { bodyDone <- c.cli.UidFetch(seqset, []imap.FetchItem{section.FetchItem()}, bodyMessages) }()
	bodyMessage := <-bodyMessages
	if err := <-bodyDone; err != nil {
		return nil, err
	}
	if bodyMessage == nil || bodyMessage.GetBody(section) == nil {
		return nil, fmt.Errorf("邮件正文不存在 (uid=%d)", uid)
	}
	body, err := decodeTextBody(bodyMessage.GetBody(section), part.contentType, part.encoding, part.charset)
	if err != nil {
		return nil, err
	}
	full := &FullMessage{Message: toMessage(msg), Body: body, ContentType: part.contentType}
	full.Folder = folder
	full.Code = ExtractVerificationCode(full.Subject + "\n" + full.Body)
	return full, nil
}

type selectedBodyPart struct {
	path        []int
	contentType string
	encoding    string
	charset     string
}

// selectBodyPart 排除附件，并在整棵 MIME 树中优先选择 text/plain，其次 text/html。
func selectBodyPart(bs *imap.BodyStructure) (selectedBodyPart, bool) {
	var plain, rich *selectedBodyPart
	bs.Walk(func(path []int, part *imap.BodyStructure) bool {
		if strings.EqualFold(part.Disposition, "attachment") {
			return false
		}
		if !strings.EqualFold(part.MIMEType, "text") {
			return true
		}
		subtype := strings.ToLower(part.MIMESubType)
		if subtype != "plain" && subtype != "html" {
			return true
		}
		candidate := &selectedBodyPart{
			path:        append([]int(nil), path...),
			contentType: "text/" + subtype,
			encoding:    part.Encoding,
			charset:     part.Params["charset"],
		}
		if subtype == "plain" && plain == nil {
			plain = candidate
		} else if subtype == "html" && rich == nil {
			rich = candidate
		}
		return true
	})
	if plain != nil {
		return *plain, true
	}
	if rich != nil {
		return *rich, true
	}
	return selectedBodyPart{}, false
}

// ---- 解析工具 ----

func toMessage(msg *imap.Message) Message {
	m := Message{}
	if msg.Uid > 0 {
		m.ID = fmt.Sprintf("%d", msg.Uid)
	}
	if msg.Envelope != nil {
		if len(msg.Envelope.From) > 0 {
			m.From = msg.Envelope.From[0].Address()
		}
		if len(msg.Envelope.To) > 0 {
			addrs := make([]string, 0, len(msg.Envelope.To))
			for _, a := range msg.Envelope.To {
				addrs = append(addrs, a.Address())
			}
			m.To = strings.Join(addrs, ", ")
		}
		m.Subject = decodeHeader(msg.Envelope.Subject)
		if !msg.Envelope.Date.IsZero() {
			m.Date = msg.Envelope.Date.Format(time.RFC3339)
		}
	}
	if m.From != "" {
		m.From = decodeHeader(m.From)
	}
	if m.To != "" {
		m.To = decodeHeader(m.To)
	}
	return m
}

// toMessageWithBody 在 toMessage 基础上解析正文填充 Preview(供 OTP 提取)。
// 兼容部分拉取 (BODY.PEEK[TEXT]<0,4096>): 遍历 msg.Body 取第一个可读节。
func toMessageWithBody(msg *imap.Message) Message {
	m := toMessage(msg)
	for section, r := range msg.Body {
		if r == nil {
			continue
		}
		if em, err := mail.ReadMessage(r); err == nil {
			if body, err := readBody(em); err == nil {
				m.Preview = strings.TrimSpace(body)
			}
		} else {
			// 部分拉取的内容可能不是完整 MIME 消息,退回粗暴去标签
			m.Preview = previewFromRaw(r)
		}
		_ = section
		break
	}
	return m
}

// previewFromRaw 从原始 (可能截断的) 正文提取简短预览。
func previewFromRaw(r io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(r, 4096))
	if err != nil {
		return ""
	}
	text := string(raw)
	if body, ok := previewFromMIME(raw); ok {
		text = body
	} else {
		// IMAP partial fetches can start inside a quoted-printable body and
		// therefore no longer contain enough MIME headers for previewFromMIME.
		// Decode the common transfer markers before stripping HTML so fragments
		// such as "Your code is 482913=2E=20" remain readable and searchable.
		if strings.Contains(text, "=20") || strings.Contains(text, "=3D") || strings.Contains(text, "=2E") || strings.Contains(text, "=0A") {
			if decoded, decodeErr := io.ReadAll(quotedprintable.NewReader(strings.NewReader(text))); decodeErr == nil {
				text = string(decoded)
			}
		}
		text = stripHTML(text)
	}
	text = stripForwardedHeaderPreamble(text)
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > 200 {
		text = string(runes[:200])
	}
	return text
}

// stripForwardedHeaderPreamble 去掉转发服务写入正文开头的原始邮件头。
// 这些字段不是邮件内容，出现在摘要里会挤掉真正的正文，也会干扰验证码识别。
func stripForwardedHeaderPreamble(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	headerNames := map[string]bool{
		"return-path": true, "original-recipient": true, "delivered-to": true,
		"received": true, "x-original-to": true, "envelope-to": true,
		"resent-to": true, "x-forwarded-to": true, "content-type": true,
	}
	seenHeader := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if seenHeader {
				return strings.Join(lines[index+1:], "\n")
			}
			continue
		}
		name, _, ok := strings.Cut(trimmed, ":")
		if !ok || !headerNames[strings.ToLower(strings.TrimSpace(name))] {
			break
		}
		seenHeader = true
	}
	return text
}

// previewFromMIME 解码列表阶段的 BODY[TEXT] 片段。
// 对 multipart 邮件，BODY[TEXT] 往往从 boundary 开始而不是从完整邮件头开始；
// 直接 stripHTML 会把 boundary 和 Content-Type 当正文显示。
func previewFromMIME(raw []byte) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}

	if message, err := mail.ReadMessage(bytes.NewReader(raw)); err == nil && message.Header.Get("Content-Type") != "" {
		if body, bodyErr := readBody(message); bodyErr == nil && strings.TrimSpace(body) != "" {
			return body, true
		}
	}

	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	boundary := ""
	for index, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "--") || len(line) <= 2 || index+1 >= len(lines) {
			continue
		}
		candidate := strings.TrimSuffix(strings.TrimPrefix(line, "--"), "--")
		if candidate == "" {
			continue
		}
		for _, headerLine := range lines[index+1 : minInt(index+12, len(lines))] {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(headerLine)), "content-type:") {
				boundary = candidate
				break
			}
		}
		if boundary != "" {
			break
		}
	}
	if boundary == "" {
		return "", false
	}

	mr := multipart.NewReader(strings.NewReader(text), boundary)
	var htmlFallback string
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		body, kind, ok := readPartBody(part)
		if !ok || strings.TrimSpace(body) == "" {
			continue
		}
		if kind == "text/plain" {
			return body, true
		}
		if htmlFallback == "" {
			htmlFallback = body
		}
	}
	return htmlFallback, htmlFallback != ""
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

// decodeHeader 解码 RFC 2047 编码的邮件头(如 =?UTF-8?B?xxx?=)。
func decodeHeader(s string) string {
	if s == "" {
		return ""
	}
	dec := mime.WordDecoder{CharsetReader: charset.Reader}
	out, err := dec.DecodeHeader(s)
	if err != nil {
		return s
	}
	return out
}

// readBody 读取邮件正文,优先 text/plain,其次从 HTML 提取纯文本。
// 支持 multipart (含 multipart/signed/mixed/alternative)，在整棵树中选择最佳正文。
func readBody(msg *mail.Message) (string, error) {
	ct := msg.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/") {
		mr := multipart.NewReader(msg.Body, multipartBoundary(ct))
		var htmlFallback string
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			body, kind, ok := readPartBody(part)
			if !ok {
				continue
			}
			if kind == "text/plain" {
				return body, nil
			}
			if htmlFallback == "" {
				htmlFallback = body
			}
		}
		return htmlFallback, nil
	}
	return readTextPart(ct, msg.Header.Get("Content-Transfer-Encoding"), msg.Body)
}

// multipartBoundary 从 Content-Type 提取 boundary。
func multipartBoundary(ct string) string {
	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		return ""
	}
	return params["boundary"]
}

// readPartBody 读取 multipart 的一个 part; 文本直接读,嵌套 multipart 递归。
func readPartBody(part *multipart.Part) (string, string, bool) {
	if strings.EqualFold(strings.TrimSpace(strings.Split(part.Header.Get("Content-Disposition"), ";")[0]), "attachment") {
		return "", "", false
	}
	ct := part.Header.Get("Content-Type")
	if ct == "" {
		ct = "text/plain"
	}
	if strings.HasPrefix(ct, "multipart/") {
		mr := multipart.NewReader(part, multipartBoundary(ct))
		var htmlFallback string
		for {
			sub, err := mr.NextPart()
			if err != nil {
				break
			}
			body, kind, ok := readPartBody(sub)
			if !ok {
				continue
			}
			if kind == "text/plain" {
				return body, kind, true
			}
			if htmlFallback == "" {
				htmlFallback = body
			}
		}
		if htmlFallback != "" {
			return htmlFallback, "text/html", true
		}
		return "", "", false
	}
	if strings.HasPrefix(ct, "text/plain") || strings.HasPrefix(ct, "text/html") {
		body, err := readTextPart(ct, part.Header.Get("Content-Transfer-Encoding"), part)
		if err == nil && strings.TrimSpace(body) != "" {
			kind, _, _ := mime.ParseMediaType(ct)
			return body, strings.ToLower(kind), true
		}
	}
	return "", "", false
}

// readTextPart 读取单个文本部分 (plain 直接返回, html 去标签)。
func readTextPart(ct, encoding string, r io.Reader) (string, error) {
	mediaType, params, _ := mime.ParseMediaType(ct)
	return decodeTextBody(r, strings.ToLower(mediaType), encoding, params["charset"])
}

func decodeTextBody(r io.Reader, contentType, encoding, bodyCharset string) (string, error) {
	var decoded io.Reader = r
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "quoted-printable":
		decoded = quotedprintable.NewReader(decoded)
	case "base64":
		decoded = base64.NewDecoder(base64.StdEncoding, decoded)
	}
	if bodyCharset != "" && !strings.EqualFold(bodyCharset, "utf-8") && !strings.EqualFold(bodyCharset, "us-ascii") {
		converted, err := charset.Reader(bodyCharset, decoded)
		if err != nil {
			return "", fmt.Errorf("不支持的邮件字符集 %s: %w", bodyCharset, err)
		}
		decoded = converted
	}
	raw, err := io.ReadAll(decoded)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(contentType, "text/html") {
		return stripHTML(string(raw)), nil
	}
	return string(raw), nil
}

func decodePreviewBody(r io.Reader, contentType, encoding, bodyCharset string) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(r, 4096))
	if err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "quoted-printable":
		if decoded, decodeErr := io.ReadAll(quotedprintable.NewReader(strings.NewReader(string(raw)))); decodeErr == nil {
			raw = decoded
		}
	case "base64":
		compact := strings.NewReplacer("\r", "", "\n", "", " ", "", "\t", "").Replace(string(raw))
		compact = compact[:len(compact)-len(compact)%4]
		if decoded, decodeErr := base64.StdEncoding.DecodeString(compact); decodeErr == nil || len(decoded) > 0 {
			raw = decoded
		}
	}
	if bodyCharset != "" && !strings.EqualFold(bodyCharset, "utf-8") && !strings.EqualFold(bodyCharset, "us-ascii") {
		if converted, convertErr := charset.Reader(bodyCharset, strings.NewReader(string(raw))); convertErr == nil {
			if decoded, readErr := io.ReadAll(converted); readErr == nil {
				raw = decoded
			}
		}
	}
	text := string(raw)
	if strings.EqualFold(contentType, "text/html") {
		text = stripHTML(text)
	}
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > 240 {
		text = string(runes[:240])
	}
	return text, nil
}

// stripHTML 使用 HTML tokenizer 提取可读文本，忽略脚本/样式并保留块级换行。
func stripHTML(source string) string {
	z := xhtml.NewTokenizer(strings.NewReader(source))
	var out strings.Builder
	skipDepth := 0
	for {
		typeOfToken := z.Next()
		if typeOfToken == xhtml.ErrorToken {
			break
		}
		token := z.Token()
		name := strings.ToLower(token.Data)
		switch typeOfToken {
		case xhtml.StartTagToken:
			if name == "script" || name == "style" || name == "head" {
				skipDepth++
				continue
			}
			if skipDepth == 0 && (name == "br" || name == "p" || name == "div" || name == "tr" || name == "li" || name == "h1" || name == "h2" || name == "h3" || name == "h4") {
				out.WriteByte('\n')
			}
		case xhtml.EndTagToken:
			if name == "script" || name == "style" || name == "head" {
				if skipDepth > 0 {
					skipDepth--
				}
				continue
			}
			if skipDepth == 0 && (name == "p" || name == "div" || name == "tr" || name == "li" || name == "h1" || name == "h2" || name == "h3" || name == "h4") {
				out.WriteByte('\n')
			}
		case xhtml.TextToken:
			if skipDepth == 0 {
				out.WriteString(stdhtml.UnescapeString(token.Data))
			}
		}
	}
	lines := strings.Split(out.String(), "\n")
	clean := lines[:0]
	for i, l := range lines {
		lines[i] = strings.Join(strings.Fields(l), " ")
		if lines[i] != "" && (len(clean) == 0 || clean[len(clean)-1] != lines[i]) {
			clean = append(clean, lines[i])
		}
	}
	return strings.TrimSpace(strings.Join(clean, "\n"))
}
