package mail

import "testing"

func TestExtractVerificationCode(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"Chinese keyword", "你的验证码是 123 456，请勿分享", "123456"},
		{"English suffix", "A1B2C3 is your verification code", "A1B2C3"},
		{"Keyword beats year", "Acme 2026. Security code: 987654", "987654"},
		{"Reject year only", "Copyright 2026 Acme", ""},
		{"Reject repeated digits", "Your code is 000000", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractVerificationCode(tt.text); got != tt.want {
				t.Fatalf("ExtractVerificationCode() = %q, want %q", got, tt.want)
			}
		})
	}
}
