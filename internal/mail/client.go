// Package mail 实现 iCloud 邮件 IMAP 读取客户端。
//
// 通过 Apple 应用专用密码连接 imap.mail.me.com:993,
// 拉取隐私邮箱别名收到的邮件。对应原 Python 项目 icloud_mail.py。
package mail

import (
	"fmt"
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
	appleID     string
	appPassword string
	cli         *client.Client
}

// NewClient 创建 IMAP 客户端。需在调用其它方法前先 Connect。
func NewClient(appleID, appPassword string) *Client {
	return &Client{appleID: appleID, appPassword: appPassword}
}

// Connect 连接并登录 IMAP 服务器。
func (c *Client) Connect() error {
	addr := fmt.Sprintf("%s:%d", IMAPServer, IMAPPort)
	cli, err := client.DialTLS(addr, nil)
	if err != nil {
		return fmt.Errorf("IMAP 连接失败: %w", err)
	}
	if err := cli.Login(c.appleID, c.appPassword); err != nil {
		return fmt.Errorf("IMAP 登录失败 — 请检查: 1) 应用专用密码是否正确 2) Apple ID: %s — %w", c.appleID, err)
	}
	c.cli = cli
	return nil
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

	// 轻量拉取: 只取 envelope + 正文前 4KB (预览用),
	// 避免为列表视图下载每封邮件的完整正文 (主要耗时来源)
	section := &imap.BodySectionName{Peek: true, Partial: []int{0, 4096}}
	items := []imap.FetchItem{
		imap.FetchUid,
		imap.FetchEnvelope,
		imap.FetchInternalDate,
		section.FetchItem(),
	}

	messages := make(chan *imap.Message, limit)
	done := make(chan error, 1)
	go func() {
		done <- c.cli.Fetch(seqset, items, messages)
	}()

	var out []Message
	for msg := range messages {
		m := toMessageWithBody(msg)
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

	// 拉取完整正文,以便填充 Preview(OTP 验证码在正文中)。
	// fetchByUIDs 的结果集很小 (服务端搜索命中),全文拉取开销可接受。
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchInternalDate, section.FetchItem()}
	messages := make(chan *imap.Message, len(uids))
	done := make(chan error, 1)
	go func() {
		done <- c.cli.UidFetch(seqset, items, messages)
	}()

	var out []Message
	for msg := range messages {
		out = append(out, toMessageWithBody(msg))
	}
	if err := <-done; err != nil {
		return nil, err
	}
	return out, nil
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

	// 用 BODY.PEEK[] 而非 RFC822: 后者会设置 \Seen,
	// 只读 (examine) 模式下部分邮件(如垃圾邮件)取不回正文
	section := &imap.BodySectionName{Peek: true}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchInternalDate, section.FetchItem()}
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

	full := &FullMessage{Message: toMessage(msg)}
	// 遍历 msg.Body 取第一个可读节——GetBody(空 section) 与
	// FetchRFC822 存储的键不匹配,会取不到正文。
	for _, r := range msg.Body {
		if r == nil {
			continue
		}
		if em, err := mail.ReadMessage(r); err == nil {
			body, _ := readBody(em)
			full.Body = body
			full.ContentType = em.Header.Get("Content-Type")
		}
		break
	}
	return full, nil
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
	text := stripHTML(string(raw))
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 200 {
		text = text[:200]
	}
	return text
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

var htmlTag = regexp.MustCompile(`<[^>]+>`)

// readBody 读取邮件正文,优先 text/plain,其次从 HTML 提取纯文本。
// 支持 multipart (含 multipart/signed/mixed/alternative) 递归取第一个文本部分。
func readBody(msg *mail.Message) (string, error) {
	ct := msg.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/") {
		mr := multipart.NewReader(msg.Body, multipartBoundary(ct))
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			if body, ok := readPartBody(part); ok {
				return body, nil
			}
		}
		return "", nil
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
func readPartBody(part *multipart.Part) (string, bool) {
	ct := part.Header.Get("Content-Type")
	if ct == "" {
		ct = "text/plain"
	}
	if strings.HasPrefix(ct, "multipart/") {
		mr := multipart.NewReader(part, multipartBoundary(ct))
		for {
			sub, err := mr.NextPart()
			if err != nil {
				break
			}
			if body, ok := readPartBody(sub); ok {
				return body, true
			}
		}
		return "", false
	}
	if strings.HasPrefix(ct, "text/plain") || strings.HasPrefix(ct, "text/html") {
		body, err := readTextPart(ct, part.Header.Get("Content-Transfer-Encoding"), part)
		if err == nil && strings.TrimSpace(body) != "" {
			return body, true
		}
	}
	return "", false
}

// readTextPart 读取单个文本部分 (plain 直接返回, html 去标签)。
func readTextPart(ct, encoding string, r io.Reader) (string, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	if strings.Contains(encoding, "quoted-printable") {
		decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(string(raw))))
		if err == nil {
			raw = decoded
		}
	}
	if strings.HasPrefix(ct, "text/html") {
		return stripHTML(string(raw)), nil
	}
	return string(raw), nil
}

// stripHTML 粗略剥离 HTML 标签,保留可读文本。
func stripHTML(html string) string {
	// 换行标签转换行
	html = strings.ReplaceAll(html, "<br>", "\n")
	html = strings.ReplaceAll(html, "<br/>", "\n")
	html = strings.ReplaceAll(html, "<br />", "\n")
	html = strings.ReplaceAll(html, "</p>", "\n")
	html = strings.ReplaceAll(html, "</div>", "\n")
	html = strings.ReplaceAll(html, "</tr>", "\n")
	html = strings.ReplaceAll(html, "<li>", "\n- ")
	// 去掉所有标签
	html = htmlTag.ReplaceAllString(html, "")
	// 反转义常见实体
	html = strings.ReplaceAll(html, "&nbsp;", " ")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	// 压缩多余空白
	lines := strings.Split(html, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
