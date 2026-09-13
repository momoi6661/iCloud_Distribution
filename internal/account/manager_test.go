package account

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCookieInput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr bool
	}{
		{
			name:  "JSON 格式",
			input: `{"X-APPLE-WEBAUTH-TOKEN":"abc","session":"xyz"}`,
			want:  map[string]string{"X-APPLE-WEBAUTH-TOKEN": "abc", "session": "xyz"},
		},
		{
			name:  "JSON 格式过滤空值",
			input: `{"a":"1","b":""}`,
			want:  map[string]string{"a": "1"},
		},
		{
			name:  "Header String 格式",
			input: "name1=value1; name2=value2",
			want:  map[string]string{"name1": "value1", "name2": "value2"},
		},
		{
			name:  "Header String 值含等号",
			input: "token=abc=def=; other=x",
			want:  map[string]string{"token": "abc=def=", "other": "x"},
		},
		{
			name:    "空输入",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "全部空值的 JSON",
			input:   `{"a":""}`,
			wantErr: true,
		},
		{
			name:    "无等号的垃圾输入",
			input:   "garbage-no-equals",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCookieInput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望错误, 实际成功: %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("意外错误: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("cookie 数量 = %d, 期望 %d (%v)", len(got), len(tt.want), got)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("cookie[%q] = %q, 期望 %q", k, got[k], v)
				}
			}
		})
	}
}

func TestManager_AddRemoveList(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	acc, err := m.AddAccount("测试号", "", "", "")
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}
	if acc.Status != "pending" {
		t.Errorf("无 Cookie 账号状态应为 pending, 实际 %q", acc.Status)
	}

	list := m.ListAccounts()
	if len(list) != 1 {
		t.Fatalf("账号数量 = %d, 期望 1", len(list))
	}

	if !m.RemoveAccount(acc.ID) {
		t.Error("RemoveAccount 应返回 true")
	}
	if m.RemoveAccount(acc.ID) {
		t.Error("重复删除应返回 false")
	}
	if len(m.ListAccounts()) != 0 {
		t.Error("删除后列表应为空")
	}
}

func TestManager_SetLoginEmail(t *testing.T) {
	m, _ := NewManager(t.TempDir())
	acc, _ := m.AddAccount("测试", "", "", "")

	if err := m.SetLoginEmail(acc.ID, "me@example.com"); err != nil {
		t.Fatalf("SetLoginEmail: %v", err)
	}
	got, _ := m.GetAccount(acc.ID)
	if got.RealEmail != "me@example.com" {
		t.Errorf("RealEmail = %q", got.RealEmail)
	}

	if err := m.SetLoginEmail("不存在", "x@y.z"); err == nil {
		t.Error("不存在的账号应返回错误")
	}
}

func TestManager_NewLoginClient_NoEmail(t *testing.T) {
	m, _ := NewManager(t.TempDir())
	acc, _ := m.AddAccount("测试", "", "", "")

	if _, _, err := m.NewLoginClient(acc.ID); err == nil {
		t.Error("未设置邮箱的账号应返回错误")
	}

	_ = m.SetLoginEmail(acc.ID, "me@example.com")
	client, email, err := m.NewLoginClient(acc.ID)
	if err != nil {
		t.Fatalf("NewLoginClient: %v", err)
	}
	if email != "me@example.com" {
		t.Errorf("email = %q", email)
	}
	if client == nil {
		t.Error("client 不应为 nil")
	}
}

func TestManager_DeactivateRestorePersistsAndBlocksUse(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir)
	acc, _ := m.AddAccount("测试", "", "", "")
	if err := m.DeactivateAccount(acc.ID); err != nil {
		t.Fatalf("DeactivateAccount: %v", err)
	}
	got, _ := m.GetAccount(acc.ID)
	if got.Status != statusDisabled {
		t.Fatalf("状态 = %q, 期望 disabled", got.Status)
	}
	if _, _, err := m.NewLoginClient(acc.ID); err == nil {
		t.Fatal("disabled 账号不应允许登录")
	}
	if len(m.ListDisabledAccounts()) != 1 {
		t.Fatal("禁用列表应包含账号")
	}

	m2, err := NewManager(dir)
	if err != nil {
		t.Fatalf("重新加载 accounts.json: %v", err)
	}
	if got, _ := m2.GetAccount(acc.ID); got.Status != statusDisabled {
		t.Fatalf("重载状态 = %q, 期望 disabled", got.Status)
	}
	if err := m2.RestoreAccount(acc.ID); err != nil {
		t.Fatalf("RestoreAccount: %v", err)
	}
	if got, _ := m2.GetAccount(acc.ID); got.Status != "pending" {
		t.Fatalf("恢复无 Cookie 账号状态 = %q, 期望 pending", got.Status)
	}
}

func TestManager_BatchAccountOperations(t *testing.T) {
	m, _ := NewManager(t.TempDir())
	a, _ := m.AddAccount("a", "", "", "")
	b, _ := m.AddAccount("b", "", "", "")
	changed, already, missing, err := m.BatchDeactivate([]string{a.ID, b.ID, "missing"})
	if err != nil || changed != 2 || already != 0 || missing != 1 {
		t.Fatalf("批量禁用结果 = %d/%d/%d, err=%v", changed, already, missing, err)
	}
	changed, already, missing, err = m.BatchDeactivate([]string{a.ID, "missing"})
	if err != nil || changed != 0 || already != 1 || missing != 1 {
		t.Fatalf("重复批量禁用结果 = %d/%d/%d, err=%v", changed, already, missing, err)
	}
	if _, _, err := m.BatchRemove([]string{a.ID, a.ID}); err == nil {
		t.Fatal("批量删除重复 ID 应返回错误")
	}
	deleted, missing, err := m.BatchRemove([]string{a.ID, "missing"})
	if err != nil || deleted != 1 || missing != 1 {
		t.Fatalf("批量删除结果 = %d/%d, err=%v", deleted, missing, err)
	}
}

func TestManager_OrganizerPersistenceAndGroupDeletion(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir)
	acc, _ := m.AddAccount("local", "", "", "")
	group, err := m.CreateGroup(acc.ID, "  Friends  ")
	if err != nil || group.Name != "Friends" {
		t.Fatalf("CreateGroup = %+v, err=%v", group, err)
	}
	meta, err := m.UpdateAliasMetadata(acc.ID, AliasMetadata{AliasID: "alias-1", Email: " a@example.com ", GroupID: group.ID, Note: " note "})
	if err != nil || meta.Email != "a@example.com" || meta.Note != "note" || meta.UpdatedAt == "" {
		t.Fatalf("UpdateAliasMetadata = %+v, err=%v", meta, err)
	}
	if err := m.DeleteGroup(acc.ID, group.ID); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	groups, metadata, _ := m.Organizer(acc.ID)
	if len(groups) != 0 || metadata["alias-1"].GroupID != "" {
		t.Fatalf("删除分组后 organizer = groups=%v metadata=%v", groups, metadata)
	}
	m2, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, metadata, _ = m2.Organizer(acc.ID)
	if metadata["alias-1"].Note != "note" || metadata["alias-1"].GroupID != "" {
		t.Fatalf("重载 metadata = %+v", metadata["alias-1"])
	}
}

func TestManager_OrganizerLoadsOldAccountsJSON(t *testing.T) {
	dir := t.TempDir()
	raw := `{"accounts":{"acc_old":{"id":"acc_old","name":"old","status":"active"}},"updated_at":"2020-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(dir, "accounts.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	groups, metadata, err := m.Organizer("acc_old")
	if err != nil || len(groups) != 0 || len(metadata) != 0 {
		t.Fatalf("旧格式 organizer = %v/%v, err=%v", groups, metadata, err)
	}
}

func TestManager_PersistAppPasswordUpdatesCurrentAccount(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir)
	acc, _ := m.AddAccount("mail", "", "", "")
	if err := m.persistAppPassword(acc.ID, "icloud@example.com", "app-secret"); err != nil {
		t.Fatalf("persistAppPassword: %v", err)
	}
	got, ok := m.GetAccount(acc.ID)
	if !ok || got.ICloudEmail != "icloud@example.com" || got.AppPassword != "app-secret" {
		t.Fatalf("保存后的账号 = %+v", got)
	}
	reloaded, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok = reloaded.GetAccount(acc.ID)
	if !ok || got.ICloudEmail != "icloud@example.com" || got.AppPassword != "app-secret" {
		t.Fatalf("重载后的账号 = %+v", got)
	}
}

func TestManager_ListAccountsRedactsForwardIMAPPassword(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	acc, err := m.AddAccount("forward", "", "icloud.com", "")
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.accounts[acc.ID].ForwardIMAP = &ForwardIMAPConfig{
		Host: "imap.example.com", Port: 993, Email: "inbox@example.com",
		Password: "secret", Mailboxes: []string{"INBOX", "Junk"},
	}
	if err := m.save(); err != nil {
		m.mu.Unlock()
		t.Fatal(err)
	}
	m.mu.Unlock()

	listed := m.ListAccounts()
	if len(listed) != 1 || !listed[0].HasForwardIMAP || listed[0].ForwardIMAP == nil {
		t.Fatalf("forward IMAP public state missing: %+v", listed)
	}
	if listed[0].ForwardIMAP.Password != "" {
		t.Fatal("forward IMAP password leaked in account list")
	}
	if got := listed[0].ForwardIMAP.Mailboxes; len(got) != 2 || got[1] != "Junk" {
		t.Fatalf("mailboxes not preserved: %#v", got)
	}
}

func TestIsICloudDomain(t *testing.T) {
	tests := map[string]bool{
		"owner@icloud.com":          true,
		"OWNER@ME.COM":              true,
		"owner@mac.com":             true,
		"owner@gmail.com":           false,
		"owner@icloud.com.evil.test": false,
		"icloud.com":                false,
	}
	for email, want := range tests {
		if got := isICloudDomain(email); got != want {
			t.Errorf("isICloudDomain(%q) = %v, want %v", email, got, want)
		}
	}
}
