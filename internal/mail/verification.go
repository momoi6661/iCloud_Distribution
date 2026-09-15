package mail

import (
	"regexp"
	"sort"
	"strings"
)

var (
	// Keywords add confidence but are never required. A new mail language must
	// not require another extraction rule.
	verificationKeywordPrefix      = regexp.MustCompile(`(?i)(?:验证码|校验码|动态码|一次性(?:验证码|代码)|安全码|安全代码|verification(?:\s+code)?|security(?:\s+code)?|one[-\s]?time\s+code|passcode|otp|\bcode\b)\s*(?:is|为|是|的)?\s*[:：=\-]?\s*((?:[A-Z0-9][ \t-]*){4,10})`)
	verificationKeywordSuffix      = regexp.MustCompile(`(?i)((?:[A-Z0-9][ \t-]*){4,10})\s*(?:是你的验证码|为你的验证码|is your code|is your verification code)`)
	verificationChineseInstruction = regexp.MustCompile(`(?i)(?:验证码|校验码|动态码|安全码|安全代码)[^\r\n:：=]{0,16}[:：=]\s*((?:[A-Z0-9][ \t-]*){4,10})`)

	// Generic shapes support numeric, spaced/dashed, and mixed alphanumeric OTPs.
	digitCandidatePattern = regexp.MustCompile(`(?i)(^|[^A-Z0-9])((?:[0-9][ \t-]?){3,7}[0-9])([^A-Z0-9]|$)`)
	alnumCandidatePattern = regexp.MustCompile(`(?i)(^|[^A-Z0-9])([A-Z0-9]{4,10})([^A-Z0-9]|$)`)
	yearCode              = regexp.MustCompile(`^20[2-3]\d$`)
	repeatedDigitCode     = regexp.MustCompile(`^(0{4,10}|1{4,10}|2{4,10}|3{4,10}|4{4,10}|5{4,10}|6{4,10}|7{4,10}|8{4,10}|9{4,10})$`)
	technicalCodePrefix   = regexp.MustCompile(`^(?:URL|HTTP|HTTPS|ID|REF|VER|TEMPLATE|TOKEN)[A-Z0-9]*$`)
	positiveContext       = regexp.MustCompile(`(?i)(verification|security|one[-\s]?time|passcode|otp|pin|code|验证码|校验码|动态码|安全码)`)
	negativeContext       = regexp.MustCompile(`(?i)(order|invoice|receipt|amount|total|price|phone|telephone|mobile|tel|tracking|reference|transaction|address|date|year|订单|发票|金额|合计|价格|电话|手机|运单|编号|流水号|日期)`)
	technicalWord         = regexp.MustCompile(`^(?:HTTP|HTML|UTF8|BASE64|TOKEN|LOGIN|EMAIL|OUTLOOK|MICROSOFT)$`)
)

type verificationCandidate struct {
	code  string
	score int
	index int
}

// ExtractVerificationCode selects a likely OTP by shape and context. Language
// words are optional evidence, not a gate.
func ExtractVerificationCode(text string) string {
	// Full-message callers may pass rendered HTML. Never scan attributes,
	// tracking URLs, CSS classes, or hidden template IDs as if they were mail
	// content; only visible text is eligible for extraction.
	if strings.Contains(text, "<") && strings.Contains(text, ">") {
		text = stripHTML(text)
	}
	best := map[string]verificationCandidate{}
	add := func(value string, score, index int) {
		code := normalizeVerificationCode(value)
		if code == "" {
			return
		}
		candidate := verificationCandidate{code: code, score: score, index: index}
		if current, ok := best[code]; !ok || score > current.score || (score == current.score && index < current.index) {
			best[code] = candidate
		}
	}

	// Explicitly anchored matches remain strongest when present.
	for _, source := range []struct {
		pattern *regexp.Regexp
		score   int
	}{
		{verificationKeywordPrefix, 100},
		{verificationChineseInstruction, 100},
		{verificationKeywordSuffix, 100},
	} {
		for _, match := range source.pattern.FindAllStringSubmatchIndex(text, -1) {
			if len(match) >= 4 && match[2] >= 0 {
				add(text[match[2]:match[3]], source.score, match[2])
			}
		}
	}

	// Generic candidates work even when the surrounding language is unknown.
	for _, match := range digitCandidatePattern.FindAllStringSubmatchIndex(text, -1) {
		if len(match) >= 6 && match[4] >= 0 {
			add(text[match[4]:match[5]], genericCandidateScore(text, match[4], match[5], 50), match[4])
		}
	}
	for _, match := range alnumCandidatePattern.FindAllStringSubmatchIndex(text, -1) {
		if len(match) < 6 || match[4] < 0 {
			continue
		}
		value := text[match[4]:match[5]]
		upper := strings.ToUpper(value)
		// Bare mixed values need a stronger shape because template and URL IDs
		// frequently look like OTPs. Explicitly labelled codes above remain
		// permissive and can still contain one letter plus digits.
		if letters, digits := alnumCounts(upper); letters >= 2 && digits >= 2 && !technicalCodePrefix.MatchString(upper) {
			add(value, genericCandidateScore(text, match[4], match[5], 35), match[4])
		}
	}

	candidates := make([]verificationCandidate, 0, len(best))
	for _, candidate := range best {
		// Keep the existing confidence floor; mixed codes remain permissive in
		// shape, while very short bare identifiers still need some context.
		if candidate.score >= 45 {
			candidates = append(candidates, candidate)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score ||
			(candidates[i].score == candidates[j].score && candidates[i].index < candidates[j].index)
	})
	if len(candidates) == 0 {
		return ""
	}
	// If two unanchored candidates are equally plausible, guessing is worse than
	// showing no code. The user can still open the full message.
	if len(candidates) > 1 && candidates[0].score < 100 && candidates[0].score == candidates[1].score {
		return ""
	}
	return candidates[0].code
}

// IsPlausibleVerificationCode validates a code extracted from a larger raw
// message fragment. It prevents stale/template IDs from being surfaced when
// the current visible preview contains no actual verification code.
func IsPlausibleVerificationCode(value string) bool {
	code := normalizeVerificationCode(value)
	if code == "" {
		return false
	}
	letters, digits := alnumCounts(code)
	if strings.ContainsAny(code, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return letters >= 2 && digits >= 2 && !technicalCodePrefix.MatchString(code)
	}
	return digits >= 4
}

func alnumCounts(value string) (letters, digits int) {
	for _, char := range value {
		switch {
		case char >= 'A' && char <= 'Z':
			letters++
		case char >= '0' && char <= '9':
			digits++
		}
	}
	return letters, digits
}

func genericCandidateScore(text string, start, end, base int) int {
	code := normalizeVerificationCode(text[start:end])
	score := base
	switch len(code) {
	case 6:
		score += 20
	case 5, 7:
		score += 10
	case 4, 8:
		score += 5
	}
	left, right := start-64, end+64
	if left < 0 {
		left = 0
	}
	if right > len(text) {
		right = len(text)
	}
	context := text[left:right]
	if positiveContext.MatchString(context) {
		score += 25
	}
	if negativeContext.MatchString(context) {
		score -= 45
	}
	lineStart := strings.LastIndex(text[:start], "\n") + 1
	lineEndOffset := strings.Index(text[end:], "\n")
	lineEnd := len(text)
	if lineEndOffset >= 0 {
		lineEnd = end + lineEndOffset
	}
	line := strings.TrimSpace(text[lineStart:lineEnd])
	if normalizeVerificationCode(line) == code {
		// A code in its own visual block is a strong language-independent signal.
		score += 30
	}
	if prefix := text[:start]; strings.HasSuffix(prefix, ":") || strings.HasSuffix(prefix, "：") || strings.HasSuffix(prefix, "=") {
		score += 25
	}
	return score
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
	if !hasDigit || yearCode.MatchString(code) || repeatedDigitCode.MatchString(code) || technicalWord.MatchString(code) {
		return ""
	}
	return code
}
