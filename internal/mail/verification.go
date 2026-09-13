package mail

import (
	"regexp"
	"sort"
	"strings"
)

var (
	// 只有靠近明确验证码语义的候选值才允许展示，避免把年份、订单号、金额等
	// 正文数字误判成验证码。
	verificationKeywordPrefix = regexp.MustCompile(`(?i)(?:验证码|校验码|动态码|一次性(?:验证码|代码)|安全码|安全代码|verification(?:\s+code)?|security(?:\s+code)?|one[-\s]?time\s+code|passcode|otp|\bcode\b)\s*(?:is|为|是|的)?\s*[:：=\-]?\s*((?:[A-Z0-9]\s*){4,10})`)
	verificationKeywordSuffix = regexp.MustCompile(`(?i)((?:[A-Z0-9]\s*){4,10})\s*(?:是你的验证码|为你的验证码|is your code|is your verification code)`)
	yearCode                  = regexp.MustCompile(`^20[2-3]\d$`)
	repeatedDigitCode         = regexp.MustCompile(`^(0{4,10}|1{4,10}|2{4,10}|3{4,10}|4{4,10}|5{4,10}|6{4,10}|7{4,10}|8{4,10}|9{4,10})$`)
)

type verificationCandidate struct {
	code  string
	score int
	index int
}

// ExtractVerificationCode returns the most likely verification code in mail text.
// Keyword-adjacent candidates win over bare numbers to avoid treating years and IDs as codes.
func ExtractVerificationCode(text string) string {
	sources := []struct {
		pattern *regexp.Regexp
		score   int
	}{
		{verificationKeywordPrefix, 100},
		{verificationKeywordSuffix, 100},
	}
	best := map[string]verificationCandidate{}
	for _, source := range sources {
		for _, match := range source.pattern.FindAllStringSubmatchIndex(text, -1) {
			if len(match) < 4 || match[2] < 0 {
				continue
			}
			code := normalizeVerificationCode(text[match[2]:match[3]])
			if code == "" {
				continue
			}
			candidate := verificationCandidate{code: code, score: source.score, index: match[2]}
			if current, ok := best[code]; !ok || candidate.score > current.score || (candidate.score == current.score && candidate.index < current.index) {
				best[code] = candidate
			}
		}
	}
	candidates := make([]verificationCandidate, 0, len(best))
	for _, candidate := range best {
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score || (candidates[i].score == candidates[j].score && candidates[i].index < candidates[j].index)
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].code
}

func normalizeVerificationCode(value string) string {
	code := strings.ToUpper(strings.NewReplacer(" ", "", "\t", "", "-", "").Replace(value))
	if len(code) < 4 || len(code) > 10 {
		return ""
	}
	hasDigit := false
	for _, char := range code {
		if char >= '0' && char <= '9' {
			hasDigit = true
			continue
		}
		if char < 'A' || char > 'Z' {
			return ""
		}
	}
	if !hasDigit || yearCode.MatchString(code) || repeatedDigitCode.MatchString(code) {
		return ""
	}
	return code
}
