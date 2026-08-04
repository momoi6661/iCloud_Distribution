package account

import "testing"

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
