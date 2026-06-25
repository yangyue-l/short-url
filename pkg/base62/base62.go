package base62

const charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// Encode 将 uint64 编码为 Base62 字符串
func Encode(id uint64) string {
	if id == 0 {
		return string(charset[0])
	}
	var result []byte
	for id > 0 {
		result = append(result, charset[id%62])
		id /= 62
	}
	// 反转
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return string(result)
}
