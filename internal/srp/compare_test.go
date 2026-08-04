package srp

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"testing"
)

// TestCompareWithJsSrp 输出与 /tmp/srp_compare.js 相同的固定输入计算结果,
// 用于人工/脚本对比两个实现是否一致。
func TestCompareWithJsSrp(t *testing.T) {
	params := GetParams(2048)
	params.NoUserNameInX = true // GSA 模式: x 不含用户名

	aBytes, _ := hex.DecodeString("0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20")
	client := NewSRPClient(params, aBytes)

	passKey, _ := hex.DecodeString("a1a2a3a4a5a6a7a8a9b0b1b2b3b4b5b6b7b8b9c0c1c2c3c4c5c6c7c8c9d0d1d2")
	salt, _ := base64.StdEncoding.DecodeString("KiacjYK6Wb2Jt8R8TVaHfg==")
	B, _ := base64.StdEncoding.DecodeString("ZMYMUDlFB7O5kXRVXJWjHLY7s2o5BrGWZ8BIH4YSBuAyClvB2wW2a1O6eWPTRqzJk0JTLZcJ0hFaFz4NNPti/RwJKUQhGlZ7aXLK1smWQYjAiK0LAo7L0PbNywk1vA26K64H1iqQaKq5CHBNfIe/qRW1A9fRZZJypMbBQmlOfosaG0hjriF+5IkaGg2E053d6xUv3deEJJD/XG+9Taw8ObRm7JAExvalenS12pDxbBSvezGmmYE+sIjjqnoNEfU/uuqIwPwDkzUxUw15GyPhr8ziLuszowGeKOR1sXoxgKrL68oZuBIYQ0oztljGEZhqIks0kQ1t8XVVe+7knG7Hlw==")

	client.ProcessClientChanllenge([]byte("testuser@example.com"), passKey, salt, B)

	fmt.Printf("A_bigint_hex: %x\n", client.A)
	fmt.Printf("A_bytes_b64: %s\n", base64.StdEncoding.EncodeToString(client.A.Bytes()))
	fmt.Printf("M1_hex: %s\n", hex.EncodeToString(client.M1))
	fmt.Printf("M1_b64: %s\n", base64.StdEncoding.EncodeToString(client.M1))
	fmt.Printf("M2_hex: %s\n", hex.EncodeToString(client.M2))
}
