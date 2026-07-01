package bloom

import (
	"hash/fnv"
	"sync"
)

// Filter 本地内存布隆过滤器，用于快速判断短码是否存在（防缓存穿透）
type Filter struct {
	mu     sync.RWMutex
	bits   []uint64 // 位数组，每个 uint64 存 64 位
	size   uint32   // 位数组总位数（uint64 个数 × 64）
	hashes uint32   // 哈希函数个数
}

// New 创建布隆过滤器
// n: 预估元素数量
// p: 目标误判率（如 0.001 表示 0.1%）
func New(n uint32, p float64) *Filter {
	// 计算最佳位数组大小和哈希函数个数
	size := uint32(-float64(n) * p / (1.44 * 1.44)) // m = -n*ln(p)/(ln(2)^2)
	if size < 64 {
		size = 64
	}
	hashes := uint32(0.7 * float64(size) / float64(n)) // k = (m/n)*ln(2)
	if hashes < 1 {
		hashes = 1
	}
	if hashes > 30 {
		hashes = 30
	}

	words := (size + 63) / 64
	return &Filter{
		bits:   make([]uint64, words),
		size:   words * 64,
		hashes: hashes,
	}
}

// hash 双重哈希：h1 + i*h2
func (f *Filter) hash(data []byte, i uint32) uint32 {
	h1 := fnv.New32a()
	h1.Write(data)
	a := h1.Sum32()

	h2 := fnv.New32()
	h2.Write(data)
	b := h2.Sum32()

	return a + i*b
}

// Add 添加元素到过滤器
func (f *Filter) Add(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	data := []byte(key)
	for i := uint32(0); i < f.hashes; i++ {
		pos := f.hash(data, i) % f.size
		f.bits[pos/64] |= 1 << (pos % 64)
	}
}

// MayExist 判断元素是否可能存在（true=可能存在，false=一定不存在）
func (f *Filter) MayExist(key string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	data := []byte(key)
	for i := uint32(0); i < f.hashes; i++ {
		pos := f.hash(data, i) % f.size
		if f.bits[pos/64]&(1<<(pos%64)) == 0 {
			return false
		}
	}
	return true
}

// Stats 返回过滤器状态
func (f *Filter) Stats() (size, hashes uint32) {
	return f.size, f.hashes
}
