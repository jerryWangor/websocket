package util

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)
// 定义一个woker工作节点所需要的基本参数
type OrderWorker struct {
	mu        sync.Mutex // 添加互斥锁 确保并发安全
	timestamp int64      // 记录时间戳
	number    int64      // 当前毫秒已经生成的id序列号(从0开始累加) 1毫秒内最多生成4096个ID
}
var num int64
// 实例化一个工作节点
func NewOrderWorker() (*OrderWorker) {
	return &OrderWorker{timestamp: 0,number:0,}
}
func (w *OrderWorker) GetId(t time.Time) string {
	s := t.Format("20060102150405")

	m := t.UnixNano()/1e6 - t.UnixNano()/1e9*1e3

	ms := sup(m, 3)

	p := os.Getpid() % 1000

	ps := sup(int64(p), 3)

	i := atomic.AddInt64(&num, 1)

	r := i % 10000

	rs := sup(r, 4)

	n := fmt.Sprintf("%s%s%s%s", s, ms, ps, rs)

	return n
}
//对长度不足n的数字前面补0
func sup(i int64, n int) string {
	m := fmt.Sprintf("%d", i)
	for len(m) < n {
		m = fmt.Sprintf("0%s", m)
	}
	return m
}