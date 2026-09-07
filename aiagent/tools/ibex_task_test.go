package tools

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// truncateUTF8 的契约是"按字节截断但绝不切开一个 UTF-8 字符"。判定必须看第一个
// 被排除的字节，看最后一个保留字节会把前导字节留在结果里，产出非法 UTF-8。
func TestTruncateUTF8(t *testing.T) {
	const suffix = "...[truncated]"
	cases := []struct {
		name     string
		s        string
		maxBytes int
		want     string
	}{
		{"under limit", "abc", 8, "abc"},
		{"exactly at limit", "abc", 3, "abc"},
		{"ascii cut", "abcdef", 3, "abc" + suffix},
		// "aa中中" = 61 61 e4b8ad e4b8ad：切点 3/4 落在第一个"中"内部，只能退到 2；
		// 切点 5 正好是第二个"中"的起始，无需退让。修复前这三种都会把 e4 留在末尾。
		{"cut inside first rune", "aa中中", 3, "aa" + suffix},
		{"cut mid rune", "aa中中", 4, "aa" + suffix},
		{"cut on rune boundary", "aa中中", 5, "aa中" + suffix},
		{"cut after full rune", "中中中", 3, "中" + suffix},
		{"emoji", "a😀b", 3, "a" + suffix},
		{"all continuation bytes", "\x80\x80\x80", 2, suffix},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := truncateUTF8(c.s, c.maxBytes)
			if got != c.want {
				t.Fatalf("truncateUTF8(%q, %d) = %q, want %q", c.s, c.maxBytes, got, c.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("truncateUTF8(%q, %d) produced invalid UTF-8: % x", c.s, c.maxBytes, []byte(got))
			}
		})
	}
}

// 输出会进模型上下文与 Redis 效果台账，长度必须收在 maxBytes 附近（截断标记除外），
// 且任何切点都不能产出非法 UTF-8。
func TestTruncateUTF8AlwaysValid(t *testing.T) {
	s := strings.Repeat("中a😀", 64)
	for n := 0; n < len(s); n++ {
		got := truncateUTF8(s, n)
		if !utf8.ValidString(got) {
			t.Fatalf("maxBytes=%d produced invalid UTF-8: % x", n, []byte(got))
		}
		if n < len(s) && len(got) > n+len("...[truncated]") {
			t.Fatalf("maxBytes=%d produced %d bytes, over budget", n, len(got))
		}
	}
}

// ctx 取消必须立即结束等待并返回当前进度回执，而不是把确认腿钉在轮询循环里
// （wait_seconds 上限 300s，期间用户点取消曾经完全无效）。
func TestWaitTaskResultStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan string, 1)
	go func() { done <- waitTaskResult(ctx, 1, "host01", 300) }()

	select {
	case out := <-done:
		if !strings.Contains(out, "host01") {
			t.Fatalf("unexpected result on cancel: %q", out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitTaskResult ignored context cancellation")
	}
}
