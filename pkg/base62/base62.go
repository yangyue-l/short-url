package base62

import (
	"math/rand"
)

const charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// shuffledCharset 随机置换后的字符集（使用固定种子确保确定性）
var shuffledCharset [62]byte

func init() {
	// 使用固定种子生成置换表，确保每次运行结果一致
	rng := rand.New(rand.NewSource(42))
	perm := rng.Perm(62)
	for i, v := range perm {
		shuffledCharset[i] = charset[v]
	}
}

// Encode 将 uint64 编码为 Base62 字符串（使用随机置换，避免连续短码）
func Encode(id uint64) string {
	if id == 0 {
		return string(shuffledCharset[0])
	}
	var result []byte
	for id > 0 {
		result = append(result, shuffledCharset[id%62])
		id /= 62
	}
	// 反转
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return string(result)
}
