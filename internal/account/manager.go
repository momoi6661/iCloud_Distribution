// Package account 实现多账号管理器。
//
// 负责账号 CRUD、Cookie 解析(Header String / JSON)、持久化到 accounts.json,
// 以及创建 HME 客户端和邮件客户端。对应原 Python 项目 account_manager.py。
package account

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"icloud_distribution/internal/hme"
	"icloud_distribution/internal/mail"
)

// Account 描述一个 iCloud 账号。
type Account struct {
	ID             string                   `json:"id"`
	OwnerUserID    string                   `json:"owner_user_id,omitempty"`
	Name           string                   `json:"name"`
	RealEmail      string                   `json:"real_email"`
	ICloudEmail    string                   `json:"icloud_email"`
	Cookies        map[string]string        `json:"cookies,omitempty"`
	Host           string                   `json:"host"`
	Proxy          string                   `json:"proxy,omitempty"` // HTTP/SOCKS5 代理
	AppPassword    string                   `json:"app_password,omitempty"`
	ForwardIMAP    *ForwardIMAPConfig       `json:"forward_imap,omitempty"`
	Status         string                   `json:"status"` // active / error
	AliasTotal     int                      `json:"alias_total"`
	AliasActive    int                      `json:"alias_active"`
	LastValidated  string                   `json:"last_validated"`
	LastError      string                   `json:"last_error,omitempty"`
	CreatedAt      string                   `json:"created_at"`
	Groups         []LocalGroup             `json:"groups,omitempty"`
	AliasMetadata  map[string]AliasMetadata `json:"alias_metadata,omitempty"`
	HasCookies     bool                     `json:"has_cookies,omitempty"`
	HasAppPassword bool                     `json:"has_app_password,omitempty"`
	HasForwardIMAP bool                     `json:"has_forward_imap,omitempty"`
	MailReadMethod string                   `json:"mail_read_method,omitempty"`
}

func (m *Manager) AssignMissingOwners(ownerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := false
	for _, acc := range m.accounts {
		if acc.OwnerUserID == "" {
			acc.OwnerUserID = ownerID
			changed = true
		}
	}
	if changed {
		return m.save()
	}
	return nil
}

func (m *Manager) SetOwner(id, ownerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return fmt.Errorf("账号不存在")
	}
	acc.OwnerUserID = ownerID
	return m.save()
}

func (m *Manager) Owner(id string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return "", false
	}
	return acc.OwnerUserID, true
}

func (m *Manager) ListAccountsFor(ownerID string, includeDisabled bool) []*Account {
	all := m.ListAccounts()
	out := make([]*Account, 0, len(all))
	for _, acc := range all {
		if acc.OwnerUserID == ownerID && (includeDisabled || acc.Status != statusDisabled) {
			out = append(out, acc)
		}
	}
	return out
}

// ForwardIMAPConfig describes the TLS IMAP mailbox that receives forwarded
// Hide My Email messages. The password is persisted but never returned by API lists.
type ForwardIMAPConfig struct {
	Host      string   `json:"host"`
	Port      int      `json:"port"`
	Email     string   `json:"email"`
	Password  string   `json:"password,omitempty"`
	Mailboxes []string `json:"mailboxes,omitempty"`
}

// LocalGroup is a local-only organizer group; it never represents an iCloud resource.
type LocalGroup struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// AliasMetadata stores local organizer information for an alias.
type AliasMetadata struct {
	AliasID   string `json:"alias_id"`
	Email     string `json:"email"`
	Label     string `json:"label"`
	GroupID   string `json:"group_id"`
	Note      string `json:"note"`
	UpdatedAt string `json:"updated_at"`
}

const statusDisabled = "disabled"

// gatewayTTL mccgateway URL 缓存有效期。
const gatewayTTL = time.Hour

// gatewayEntry 缓存的 mccgateway 地址。
type gatewayEntry struct {
	url       string
	expiresAt time.Time
}

// imapConn 一个账号的复用 IMAP 连接。
// mu 串行化该连接上的所有操作 (go-imap 客户端非并发安全)。
type imapConn struct {
	mu     sync.Mutex
	client *mail.Client
}

// Manager 管理多个 iCloud 账号,线程安全。
type Manager struct {
	mu           sync.Mutex
	accounts     map[string]*Account
	gatewayCache map[string]gatewayEntry
	imapPool     map[string]*imapConn
	forwardPool  map[string]*imapConn
	dataDir      string
	dataFile     string
}

// NewManager 创建管理器。dataDir 用于存放 accounts.json。
func NewManager(dataDir string) (*Manager, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	m := &Manager{
		accounts:     make(map[string]*Account),
		gatewayCache: make(map[string]gatewayEntry),
		imapPool:     make(map[string]*imapConn),
		forwardPool:  make(map[string]*imapConn),
		dataDir:      dataDir,
		dataFile:     filepath.Join(dataDir, "accounts.json"),
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

// Reload 重新加载 accounts.json 配置文件。
func (m *Manager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.load()
}

func (m *Manager) load() error {
	raw, err := os.ReadFile(m.dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var wrapper struct {
		Accounts map[string]*Account `json:"accounts"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return err
	}
	m.accounts = wrapper.Accounts
	if m.accounts == nil {
		m.accounts = make(map[string]*Account)
	}
	return nil
}

func (m *Manager) save() error {
	wrapper := struct {
		Accounts  map[string]*Account `json:"accounts"`
		UpdatedAt string              `json:"updated_at"`
	}{
		Accounts:  m.accounts,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	raw, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.dataFile, raw, 0600)
}

// ParseCookieInput 解析 Cookie 输入,支持两种格式:
//   - Header String: "name1=value1; name2=value2; ..."
//   - JSON: {"name1":"value1","name2":"value2"}
//
// 空输入返回错误。
func ParseCookieInput(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("空白输入 — 请粘贴 Cookie Header String 或 JSON")
	}

	// JSON 格式
	if strings.HasPrefix(raw, "{") {
		var cookies map[string]string
		if err := json.Unmarshal([]byte(raw), &cookies); err == nil && cookies != nil {
			out := make(map[string]string, len(cookies))
			for k, v := range cookies {
				if v != "" {
					out[k] = v
				}
			}
			if len(out) > 0 {
				return out, nil
			}
		}
	}

	// Header String 格式
	cookies := make(map[string]string)
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		idx := strings.Index(part, "=")
		if idx <= 0 {
			continue
		}
		name := strings.TrimSpace(part[:idx])
		value := strings.TrimSpace(part[idx+1:])
		if name != "" {
			cookies[name] = value
		}
	}
	if len(cookies) == 0 {
		return nil, fmt.Errorf("无法解析 Cookie 输入,请提供 Header String 或 JSON 格式")
	}
	return cookies, nil
}

// AddAccount 添加一个账号。cookieInput 可为空,后续可通过 /login 获取。
//
// cookieInput 支持 Header String 或 JSON。校验失败仍会保存账号(status=error),
// 方便用户后续修正 Cookie 后重新校验。
func (m *Manager) AddAccount(name, cookieInput, host, proxy string) (*Account, error) {
	var cookies map[string]string
	if cookieInput != "" {
		var err error
		cookies, err = ParseCookieInput(cookieInput)
		if err != nil {
			return nil, err
		}
	} else {
		cookies = make(map[string]string)
	}
	if host == "" {
		host = "icloud.com"
	}

	acc := &Account{
		ID:        "acc_" + uuid.New().String()[:8],
		Name:      name,
		Cookies:   cookies,
		Host:      host,
		Proxy:     proxy,
		Status:    "pending", // 无 Cookie 时为 pending
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	// 有 Cookie 才校验会话
	if len(cookies) > 0 {
		client, err := hme.NewClient(cookies, host, proxy, false)
		if err != nil {
			return nil, err
		}
		if err := client.ValidateSession(); err != nil {
			acc.Status = "error"
			acc.LastError = truncate(err.Error(), 300)
		} else {
			acc.Status = "active"
			if info := client.AccountInfo(); info != nil {
				acc.RealEmail = firstNonEmpty(info.AppleID, info.PrimaryEmail)
				acc.ICloudEmail = deriveICloudEmail(info)
			}
			if aliases, err := client.ListAliases(); err == nil {
				acc.AliasTotal = len(aliases)
				for _, a := range aliases {
					if a.Active {
						acc.AliasActive++
					}
				}
			}
			acc.LastValidated = time.Now().Format(time.RFC3339)
		}
	}

	m.mu.Lock()
	m.accounts[acc.ID] = acc
	saveErr := m.save()
	m.mu.Unlock()
	if saveErr != nil {
		return nil, saveErr
	}
	return acc, nil
}

// RemoveAccount 删除账号。
func (m *Manager) RemoveAccount(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.accounts[id]; !ok {
		return false
	}
	delete(m.accounts, id)
	_ = m.save()
	return true
}

// DeactivateAccount disables an account without removing its credentials.
func (m *Manager) DeactivateAccount(id string) error {
	m.mu.Lock()
	acc, ok := m.accounts[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("账号不存在: %s", id)
	}
	if acc.Status == statusDisabled {
		m.mu.Unlock()
		return nil
	}
	acc.Status = statusDisabled
	err := m.save()
	m.mu.Unlock()
	if err == nil {
		m.DropIMAP(id)
		m.DropForwardIMAP(id)
	}
	return err
}

// RestoreAccount re-enables a disabled account and preserves its credentials.
func (m *Manager) RestoreAccount(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return fmt.Errorf("账号不存在: %s", id)
	}
	if acc.Status == statusDisabled {
		if len(acc.Cookies) > 0 {
			acc.Status = "active"
		} else {
			acc.Status = "pending"
		}
		return m.save()
	}
	return nil
}

// ListDisabledAccounts returns disabled accounts without cookies.
func (m *Manager) ListDisabledAccounts() []*Account {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listAccountsLocked(func(acc *Account) bool { return acc.Status == statusDisabled })
}

func (m *Manager) listAccountsLocked(include func(*Account) bool) []*Account {
	out := make([]*Account, 0)
	for _, acc := range m.accounts {
		if !include(acc) {
			continue
		}
		cp := *acc
		cp.HasCookies = len(acc.Cookies) > 0
		cp.HasAppPassword = strings.TrimSpace(acc.AppPassword) != ""
		cp.HasForwardIMAP = acc.ForwardIMAP != nil && strings.TrimSpace(acc.ForwardIMAP.Password) != ""
		cp.Cookies = nil
		cp.AppPassword = ""
		if acc.ForwardIMAP != nil {
			forward := *acc.ForwardIMAP
			forward.Password = ""
			forward.Mailboxes = append([]string(nil), acc.ForwardIMAP.Mailboxes...)
			cp.ForwardIMAP = &forward
		}
		out = append(out, &cp)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Status != out[j].Status {
			return out[i].Status != statusDisabled
		}
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt > out[j].CreatedAt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// BatchDeactivate disables each existing account and reports per-action counts.
func (m *Manager) BatchDeactivate(ids []string) (changed, alreadyDisabled, notFound int, err error) {
	if err := validateIDs(ids); err != nil {
		return 0, 0, 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		acc, ok := m.accounts[id]
		if !ok {
			notFound++
		} else if acc.Status == statusDisabled {
			alreadyDisabled++
		} else {
			acc.Status = statusDisabled
			changed++
		}
	}
	if changed > 0 {
		err = m.save()
	}
	return
}

// BatchRemove deletes each existing account and reports per-action counts.
func (m *Manager) BatchRemove(ids []string) (deleted, notFound int, err error) {
	if err := validateIDs(ids); err != nil {
		return 0, 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if _, ok := m.accounts[id]; !ok {
			notFound++
			continue
		}
		delete(m.accounts, id)
		delete(m.gatewayCache, id)
		delete(m.imapPool, id)
		delete(m.forwardPool, id)
		deleted++
	}
	if deleted > 0 {
		err = m.save()
	}
	return
}

func validateIDs(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("ids 不能为空")
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("ids 不能包含空 ID")
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("ids 不能包含重复 ID: %s", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

// GetAccount 返回账号副本。
func (m *Manager) GetAccount(id string) (*Account, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return nil, false
	}
	cp := *acc
	return &cp, true
}

// AppPassword returns the saved iCloud IMAP credentials for an account.
// Callers must still enforce ownership at the HTTP layer; this method only
// provides a synchronized copy for that already-authorized request.
func (m *Manager) AppPassword(id string) (icloudEmail, appPassword string, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return "", "", false
	}
	return acc.ICloudEmail, acc.AppPassword, true
}

// ForwardIMAP returns a copy of the saved forwarding mailbox configuration.
// The HTTP handler exposes it only through the authenticated owner route.
func (m *Manager) ForwardIMAP(id string) (*ForwardIMAPConfig, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return nil, false
	}
	if acc.ForwardIMAP == nil {
		return nil, true
	}
	config := *acc.ForwardIMAP
	config.Mailboxes = append([]string(nil), acc.ForwardIMAP.Mailboxes...)
	return &config, true
}

func (m *Manager) MailReadMethod(id string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return "", false
	}
	return acc.MailReadMethod, true
}

func (m *Manager) SetMailReadMethod(id, method string) error {
	method = strings.TrimSpace(method)
	if method != "forward_imap" && method != "imap" && method != "web_api" {
		return fmt.Errorf("不支持的邮件读取方式: %s", method)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return fmt.Errorf("账号不存在: %s", id)
	}
	if acc.Status == statusDisabled {
		return fmt.Errorf("账号已禁用，请先恢复账号: %s", id)
	}
	if method == "forward_imap" && (acc.ForwardIMAP == nil || strings.TrimSpace(acc.ForwardIMAP.Password) == "") {
		return fmt.Errorf("尚未配置转发邮箱 IMAP")
	}
	if method == "imap" && strings.TrimSpace(acc.AppPassword) == "" {
		return fmt.Errorf("尚未配置 iCloud App 专用密码")
	}
	acc.MailReadMethod = method
	return m.save()
}

// ListAccounts 返回所有账号(脱敏,不含 Cookies),按活跃状态排序。
func (m *Manager) ListAccounts() []*Account {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listAccountsLocked(func(*Account) bool { return true })
}

const (
	maxOrganizerName  = 200
	maxOrganizerNote  = 2000
	maxOrganizerEmail = 320
	maxOrganizerID    = 200
)

// Organizer returns local organizer data only. It performs no network calls.
func (m *Manager) Organizer(id string) ([]LocalGroup, map[string]AliasMetadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return nil, nil, fmt.Errorf("账号不存在: %s", id)
	}
	groups := make([]LocalGroup, len(acc.Groups))
	copy(groups, acc.Groups)
	metadata := make(map[string]AliasMetadata, len(acc.AliasMetadata))
	for aliasID, meta := range acc.AliasMetadata {
		metadata[aliasID] = meta
	}
	return groups, metadata, nil
}

func (m *Manager) CreateGroup(accountID, name string) (LocalGroup, error) {
	name, err := organizerName(name)
	if err != nil {
		return LocalGroup{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[accountID]
	if !ok {
		return LocalGroup{}, fmt.Errorf("账号不存在: %s", accountID)
	}
	group := LocalGroup{ID: "group_" + uuid.New().String()[:8], Name: name, CreatedAt: time.Now().Format(time.RFC3339)}
	acc.Groups = append(acc.Groups, group)
	if err := m.save(); err != nil {
		return LocalGroup{}, err
	}
	return group, nil
}

func (m *Manager) UpdateGroup(accountID, groupID, name string) error {
	name, err := organizerName(name)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[accountID]
	if !ok {
		return fmt.Errorf("账号不存在: %s", accountID)
	}
	for i := range acc.Groups {
		if acc.Groups[i].ID == groupID {
			acc.Groups[i].Name = name
			return m.save()
		}
	}
	return fmt.Errorf("分组不存在: %s", groupID)
}

// ReorderGroups changes only the local display order of an account's groups.
// The complete ID list is required so a stale client cannot accidentally drop
// a group while saving a drag operation.
func (m *Manager) ReorderGroups(accountID string, groupIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[accountID]
	if !ok {
		return fmt.Errorf("账号不存在: %s", accountID)
	}
	if len(groupIDs) != len(acc.Groups) {
		return fmt.Errorf("分组列表已变化，请刷新后重试")
	}
	byID := make(map[string]LocalGroup, len(acc.Groups))
	for _, group := range acc.Groups {
		byID[group.ID] = group
	}
	ordered := make([]LocalGroup, 0, len(groupIDs))
	seen := make(map[string]struct{}, len(groupIDs))
	for _, id := range groupIDs {
		group, exists := byID[id]
		if !exists {
			return fmt.Errorf("分组列表已变化，请刷新后重试")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("分组排序数据无效")
		}
		seen[id] = struct{}{}
		ordered = append(ordered, group)
	}
	acc.Groups = ordered
	return m.save()
}

func (m *Manager) DeleteGroup(accountID, groupID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[accountID]
	if !ok {
		return fmt.Errorf("账号不存在: %s", accountID)
	}
	index := -1
	for i := range acc.Groups {
		if acc.Groups[i].ID == groupID {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("分组不存在: %s", groupID)
	}
	acc.Groups = append(acc.Groups[:index], acc.Groups[index+1:]...)
	for aliasID, meta := range acc.AliasMetadata {
		if meta.GroupID == groupID {
			meta.GroupID = ""
			acc.AliasMetadata[aliasID] = meta
		}
	}
	return m.save()
}

func (m *Manager) UpdateAliasMetadata(accountID string, meta AliasMetadata) (AliasMetadata, error) {
	updated, err := m.UpdateAliasMetadataBatch(accountID, []AliasMetadata{meta})
	if err != nil {
		return AliasMetadata{}, err
	}
	return updated[0], nil
}

// UpdateAliasMetadataBatch validates and persists one organizer batch with a
// single write. This keeps bulk creation fast and prevents partially saved
// local grouping data.
func (m *Manager) UpdateAliasMetadataBatch(accountID string, metas []AliasMetadata) ([]AliasMetadata, error) {
	if len(metas) == 0 {
		return []AliasMetadata{}, nil
	}
	updated := make([]AliasMetadata, len(metas))
	copy(updated, metas)
	for i := range updated {
		meta := &updated[i]
		meta.AliasID = strings.TrimSpace(meta.AliasID)
		meta.Email = strings.TrimSpace(meta.Email)
		meta.Label = strings.TrimSpace(meta.Label)
		meta.GroupID = strings.TrimSpace(meta.GroupID)
		meta.Note = strings.TrimSpace(meta.Note)
		if meta.AliasID == "" || len(meta.AliasID) > maxOrganizerID {
			return nil, fmt.Errorf("alias_id 不能为空且长度不能超过 %d", maxOrganizerID)
		}
		if len(meta.Email) > maxOrganizerEmail || len(meta.Label) > maxOrganizerName || len(meta.Note) > maxOrganizerNote {
			return nil, fmt.Errorf("email、label 或 note 超出长度限制")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[accountID]
	if !ok {
		return nil, fmt.Errorf("账号不存在: %s", accountID)
	}
	for _, meta := range updated {
		if meta.GroupID == "" {
			continue
		}
		found := false
		for _, group := range acc.Groups {
			if group.ID == meta.GroupID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("分组不存在: %s", meta.GroupID)
		}
	}
	now := time.Now().Format(time.RFC3339)
	if acc.AliasMetadata == nil {
		acc.AliasMetadata = make(map[string]AliasMetadata)
	}
	for i := range updated {
		updated[i].UpdatedAt = now
		acc.AliasMetadata[updated[i].AliasID] = updated[i]
	}
	if err := m.save(); err != nil {
		return nil, err
	}
	return updated, nil
}

func organizerName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("分组名称不能为空")
	}
	if len(name) > maxOrganizerName {
		return "", fmt.Errorf("分组名称长度不能超过 %d", maxOrganizerName)
	}
	return name, nil
}

// HMEClient 为指定账号创建一个新的 HME 客户端。
// 必须有有效的 Cookie 才能使用 HME 功能。
func (m *Manager) HMEClient(id string, verbose bool) (*hme.Client, error) {
	m.mu.Lock()
	acc, ok := m.accounts[id]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("账号不存在: %s", id)
	}
	if acc.Status == statusDisabled {
		return nil, fmt.Errorf("账号已禁用，请先恢复账号: %s", id)
	}
	if len(acc.Cookies) == 0 {
		return nil, fmt.Errorf("账号未配置 Cookie，无法使用 HME 功能")
	}
	return hme.NewClient(acc.Cookies, acc.Host, acc.Proxy, verbose)
}

// NewLoginClient 为指定账号创建一个无 Cookie 的 HME 客户端用于密码登录。
// 返回客户端和登录邮箱 (优先 ICloudEmail,回退 RealEmail)。
// 登录成功后应调用 UpdateCookies 持久化 Cookie 并刷新账号状态。
func (m *Manager) NewLoginClient(id string) (*hme.Client, string, error) {
	m.mu.Lock()
	acc, ok := m.accounts[id]
	m.mu.Unlock()
	if !ok {
		return nil, "", fmt.Errorf("账号不存在: %s", id)
	}
	if acc.Status == statusDisabled {
		return nil, "", fmt.Errorf("账号已禁用，请先恢复账号: %s", id)
	}

	email := acc.ICloudEmail
	if email == "" {
		email = acc.RealEmail
	}
	if email == "" {
		return nil, "", fmt.Errorf("账号未设置邮箱地址,请先通过 Cookie 添加账号或设置 iCloud 邮箱")
	}

	client, err := hme.NewClient(nil, acc.Host, acc.Proxy, false)
	if err != nil {
		return nil, "", err
	}
	return client, email, nil
}

// SetLoginEmail 设置账号的登录邮箱 (Apple ID),用于两段式密码登录。
// 用户显式指定的邮箱优先,始终覆盖已有值。自动去除首尾空白并转小写
// (SRP 用户名哈希对输入格式敏感)。
func (m *Manager) SetLoginEmail(id, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return fmt.Errorf("账号不存在: %s", id)
	}
	acc.RealEmail = strings.ToLower(strings.TrimSpace(email))
	return m.save()
}

// MailClient 为指定账号创建 IMAP 邮件客户端。
// 需要事先设置 iCloud 邮箱和 App 专用密码。
func (m *Manager) MailClient(id string) (*mail.Client, error) {
	m.mu.Lock()
	acc, ok := m.accounts[id]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("账号不存在: %s", id)
	}
	if acc.Status == statusDisabled {
		return nil, fmt.Errorf("账号已禁用，请先恢复账号: %s", id)
	}
	imapEmail := acc.ICloudEmail
	if imapEmail == "" {
		imapEmail = acc.RealEmail
	}
	if !isICloudDomain(imapEmail) {
		return nil, fmt.Errorf("账号未设置 iCloud 邮箱 (当前: %s)", imapEmail)
	}
	if acc.AppPassword == "" {
		return nil, fmt.Errorf("账号未设置 App 专用密码")
	}
	return mail.NewClient(imapEmail, acc.AppPassword), nil
}

// AcquireIMAP 获取账号的复用 IMAP 连接。
//
// 每次请求新建连接要付出 TCP+TLS+登录数秒的开销,池化后只有首次付费。
// 返回的 mutex 已锁定——调用方持有期间独占连接,用完必须 Unlock。
// 连接失效时自动重建。
func (m *Manager) AcquireIMAP(id string) (*mail.Client, *sync.Mutex, error) {
	m.mu.Lock()
	acc, ok := m.accounts[id]
	if !ok {
		m.mu.Unlock()
		return nil, nil, fmt.Errorf("账号不存在: %s", id)
	}
	if acc.Status == statusDisabled {
		m.mu.Unlock()
		return nil, nil, fmt.Errorf("账号已禁用，请先恢复账号: %s", id)
	}
	imapEmail := acc.ICloudEmail
	if imapEmail == "" {
		imapEmail = acc.RealEmail
	}
	appPassword := acc.AppPassword
	conn, ok := m.imapPool[id]
	if !ok {
		conn = &imapConn{}
		m.imapPool[id] = conn
	}
	m.mu.Unlock()

	if !isICloudDomain(imapEmail) {
		return nil, nil, fmt.Errorf("账号未设置 iCloud 邮箱 (当前: %s)", imapEmail)
	}
	if appPassword == "" {
		return nil, nil, fmt.Errorf("账号未设置 App 专用密码")
	}

	conn.mu.Lock()
	// 健康检查: 连接不存在或 NOOP 失败则重建
	if conn.client != nil {
		if err := conn.client.Noop(); err == nil {
			return conn.client, &conn.mu, nil
		}
		conn.client.Disconnect()
		conn.client = nil
	}

	client := mail.NewClient(imapEmail, appPassword)
	if err := client.Connect(); err != nil {
		conn.mu.Unlock()
		return nil, nil, err
	}
	conn.client = client
	return client, &conn.mu, nil
}

// DropIMAP 移除并关闭账号的池化连接 (改密码/换 Cookie 后调用)。
func (m *Manager) DropIMAP(id string) {
	m.mu.Lock()
	conn, ok := m.imapPool[id]
	if ok {
		delete(m.imapPool, id)
	}
	m.mu.Unlock()
	if ok {
		conn.mu.Lock()
		if conn.client != nil {
			conn.client.Disconnect()
		}
		conn.mu.Unlock()
	}
}

// SetForwardIMAP validates and stores a TLS IMAP mailbox used for forwarded mail.
func (m *Manager) SetForwardIMAP(id string, config ForwardIMAPConfig) error {
	m.mu.Lock()
	acc, ok := m.accounts[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("账号不存在: %s", id)
	}
	if acc.Status == statusDisabled {
		return fmt.Errorf("账号已禁用，请先恢复账号: %s", id)
	}
	config.Host = strings.TrimSpace(config.Host)
	config.Email = strings.TrimSpace(config.Email)
	config.Password = strings.TrimSpace(config.Password)
	if config.Port == 0 {
		config.Port = 993
	}
	if config.Host == "" || len(config.Host) > 253 || strings.ContainsAny(config.Host, "/\\ \t\r\n") {
		return fmt.Errorf("IMAP 服务器地址无效")
	}
	if config.Port < 1 || config.Port > 65535 {
		return fmt.Errorf("IMAP 端口必须在 1 到 65535 之间")
	}
	if config.Email == "" || !strings.Contains(config.Email, "@") {
		return fmt.Errorf("转发邮箱地址无效")
	}
	if config.Password == "" {
		return fmt.Errorf("应用专用密码或 IMAP 授权码不能为空")
	}
	mailboxes := make([]string, 0, len(config.Mailboxes))
	seen := make(map[string]struct{})
	for _, mailbox := range config.Mailboxes {
		mailbox = strings.TrimSpace(mailbox)
		if mailbox == "" {
			continue
		}
		if len(mailbox) > 128 || strings.ContainsRune(mailbox, '\x00') {
			return fmt.Errorf("邮件文件夹名称无效")
		}
		if _, exists := seen[mailbox]; exists {
			continue
		}
		seen[mailbox] = struct{}{}
		mailboxes = append(mailboxes, mailbox)
		if len(mailboxes) > 8 {
			return fmt.Errorf("最多配置 8 个邮件文件夹")
		}
	}
	if len(mailboxes) == 0 {
		mailboxes = []string{"INBOX"}
	}
	config.Mailboxes = mailboxes

	client := mail.NewGenericClient(config.Host, config.Port, config.Email, config.Password)
	if err := client.Connect(); err != nil {
		return err
	}
	if err := client.ValidateFolders(config.Mailboxes); err != nil {
		client.Disconnect()
		return err
	}
	client.Disconnect()

	m.mu.Lock()
	acc, ok = m.accounts[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("账号不存在: %s", id)
	}
	copyConfig := config
	copyConfig.Mailboxes = append([]string(nil), config.Mailboxes...)
	acc.ForwardIMAP = &copyConfig
	err := m.save()
	m.mu.Unlock()
	if err == nil {
		m.DropForwardIMAP(id)
	}
	return err
}

// AcquireForwardIMAP returns a pooled forwarding-mailbox connection and its folders.
func (m *Manager) AcquireForwardIMAP(id string) (*mail.Client, *sync.Mutex, []string, error) {
	m.mu.Lock()
	acc, ok := m.accounts[id]
	if !ok {
		m.mu.Unlock()
		return nil, nil, nil, fmt.Errorf("账号不存在: %s", id)
	}
	if acc.Status == statusDisabled {
		m.mu.Unlock()
		return nil, nil, nil, fmt.Errorf("账号已禁用，请先恢复账号: %s", id)
	}
	if acc.ForwardIMAP == nil || strings.TrimSpace(acc.ForwardIMAP.Password) == "" {
		m.mu.Unlock()
		return nil, nil, nil, fmt.Errorf("账号未配置转发邮箱 IMAP")
	}
	config := *acc.ForwardIMAP
	config.Mailboxes = append([]string(nil), acc.ForwardIMAP.Mailboxes...)
	conn, exists := m.forwardPool[id]
	if !exists {
		conn = &imapConn{}
		m.forwardPool[id] = conn
	}
	m.mu.Unlock()

	conn.mu.Lock()
	if conn.client != nil {
		if err := conn.client.Noop(); err == nil {
			return conn.client, &conn.mu, config.Mailboxes, nil
		}
		conn.client.Disconnect()
		conn.client = nil
	}
	client := mail.NewGenericClient(config.Host, config.Port, config.Email, config.Password)
	if err := client.Connect(); err != nil {
		conn.mu.Unlock()
		return nil, nil, nil, err
	}
	conn.client = client
	return client, &conn.mu, config.Mailboxes, nil
}

// DropForwardIMAP removes and closes the forwarding mailbox connection.
func (m *Manager) DropForwardIMAP(id string) {
	m.mu.Lock()
	conn, ok := m.forwardPool[id]
	if ok {
		delete(m.forwardPool, id)
	}
	m.mu.Unlock()
	if ok {
		conn.mu.Lock()
		if conn.client != nil {
			conn.client.Disconnect()
		}
		conn.mu.Unlock()
	}
}

// WebMailClient 为指定账号创建 Web 邮件客户端。
// 使用 Cookie 认证，无需 App Password。
// mccgateway URL 按账号缓存 1 小时,避免每次读邮件都重新 validate (秒级)。
func (m *Manager) WebMailClient(id string) (*mail.WebClient, error) {
	m.mu.Lock()
	acc, ok := m.accounts[id]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("账号不存在: %s", id)
	}
	if acc.Status == statusDisabled {
		return nil, fmt.Errorf("账号已禁用，请先恢复账号: %s", id)
	}
	if len(acc.Cookies) == 0 {
		return nil, fmt.Errorf("账号未配置 Cookie，无法读取邮件")
	}
	// 从 cookies 中获取 dsid
	dsid := ""
	if v, ok := acc.Cookies["X-APPLE-WEBAUTH-USER"]; ok {
		// 解析 "v=1:s=1:d=22789132008" 格式
		parts := strings.Split(v, ":d=")
		if len(parts) == 2 {
			dsid = parts[1]
		}
	}

	wc := mail.NewWebClient(acc.Cookies, dsid, acc.Host)

	// 注入缓存的网关地址 (首次解析后由 CacheGateway 回填)
	m.mu.Lock()
	if gw, ok := m.gatewayCache[id]; ok && time.Now().Before(gw.expiresAt) {
		wc.SetGatewayURL(gw.url)
	}
	m.mu.Unlock()
	return wc, nil
}

// CacheGateway 缓存账号已解析的 mccgateway URL (由 server 在读邮件成功后调用)。
func (m *Manager) CacheGateway(id, url string) {
	if url == "" {
		return
	}
	m.mu.Lock()
	m.gatewayCache[id] = gatewayEntry{url: url, expiresAt: time.Now().Add(gatewayTTL)}
	m.mu.Unlock()
}

// SetAppPassword 设置 iCloud 邮箱和 App 专用密码,并测试 IMAP 连接。
func (m *Manager) SetAppPassword(id, icloudEmail, appPassword string) error {
	m.mu.Lock()
	acc, ok := m.accounts[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("账号不存在: %s", id)
	}
	if acc.Status == statusDisabled {
		return fmt.Errorf("账号已禁用，请先恢复账号: %s", id)
	}
	icloudEmail = strings.TrimSpace(icloudEmail)
	appPassword = strings.TrimSpace(appPassword)
	if icloudEmail == "" {
		return fmt.Errorf("iCloud 邮箱不能为空")
	}
	if !isICloudDomain(icloudEmail) {
		return fmt.Errorf("请输入 iCloud 邮箱（@icloud.com、@me.com 或 @mac.com），不要填写 Apple ID 登录邮箱")
	}
	if appPassword == "" {
		return fmt.Errorf("App 专用密码不能为空")
	}

	// 只验证 IMAP 登录。INBOX 选择/计数不是凭据验证，且可能因邮箱状态失败。
	mc := mail.NewClient(icloudEmail, appPassword)
	if err := mc.Connect(); err != nil {
		return err
	}
	mc.Disconnect()
	return m.persistAppPassword(id, icloudEmail, appPassword)
}

func (m *Manager) persistAppPassword(id, icloudEmail, appPassword string) error {
	m.mu.Lock()
	acc, ok := m.accounts[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("账号不存在: %s", id)
	}
	if acc.Status == statusDisabled {
		m.mu.Unlock()
		return fmt.Errorf("账号已禁用，请先恢复账号: %s", id)
	}
	acc.ICloudEmail = icloudEmail
	acc.AppPassword = appPassword
	err := m.save()
	m.mu.Unlock()
	if err == nil {
		m.DropIMAP(id) // 密码变了,丢弃旧连接
	}
	return err
}

// SaveCookies 保存指定账号的最新 Cookie（HMEClient 操作后刷新的 token）。
// 用于客户端 validate/操作过程中从 Set-Cookie 获取了新 token 后持久化。
func (m *Manager) SaveCookies(id string, cookies map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return fmt.Errorf("账号不存在: %s", id)
	}
	acc.Cookies = cookies
	return m.save()
}

// UpdateCookies 更新指定账号的 Cookie,并自动校验会话有效性。
func (m *Manager) UpdateCookies(id string, cookies map[string]string) error {
	if len(cookies) == 0 {
		return fmt.Errorf("cookies 不能为空")
	}
	m.mu.Lock()
	acc, ok := m.accounts[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("账号不存在: %s", id)
	}
	if acc.Status == statusDisabled {
		return fmt.Errorf("账号已禁用，请先恢复账号: %s", id)
	}

	// 自动校验 Cookie 是否有效
	acc.Cookies = cookies
	if acc.Host == "" {
		acc.Host = "icloud.com"
	}
	client, err := hme.NewClient(cookies, acc.Host, acc.Proxy, false)
	if err != nil {
		m.mu.Lock()
		acc.Status = "error"
		acc.LastError = "创建客户端失败: " + err.Error()
		m.accounts[id] = acc
		_ = m.save()
		m.mu.Unlock()
		return err
	}
	if err := client.ValidateSession(); err != nil {
		acc.Status = "error"
		acc.LastError = "Cookie 校验失败: " + err.Error()
	} else {
		acc.Status = "active"
		acc.LastValidated = time.Now().Format(time.RFC3339)
		acc.LastError = ""
		if info := client.AccountInfo(); info != nil {
			acc.RealEmail = firstNonEmpty(info.AppleID, info.PrimaryEmail)
			if acc.ICloudEmail == "" {
				acc.ICloudEmail = deriveICloudEmail(info)
			}
		}
	}

	m.mu.Lock()
	m.accounts[id] = acc
	saveErr := m.save()
	m.mu.Unlock()
	return saveErr
}

// ---- 辅助函数 ----

// deriveICloudEmail 从账号身份推导 iCloud 邮箱地址(用于 IMAP 登录)。
//
// 规则:
//  1. primaryEmail 是 @icloud.com/@me.com/@mac.com → 直接用
//  2. appleId 是上述域名 → 直接用
//  3. appleId 是第三方邮箱(如 @qq.com) → 取 local part 拼 @icloud.com
func deriveICloudEmail(info *hme.AccountInfo) string {
	primary := strings.TrimSpace(info.PrimaryEmail)
	appleID := strings.TrimSpace(info.AppleID)

	if isICloudDomain(primary) {
		return primary
	}
	if isICloudDomain(appleID) {
		return appleID
	}
	if strings.Contains(appleID, "@") {
		local := strings.SplitN(appleID, "@", 2)[0]
		return local + "@icloud.com"
	}
	return firstNonEmpty(primary, appleID)
}

func isICloudDomain(email string) bool {
	normalized := strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndex(normalized, "@")
	if at <= 0 || at == len(normalized)-1 {
		return false
	}
	switch normalized[at+1:] {
	case "icloud.com", "me.com", "mac.com":
		return true
	default:
		return false
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
