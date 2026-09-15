package service

import (
	"testing"
	"time"
)

func TestPeriodKey(t *testing.T) {
	loc := time.Local
	for _, tt := range []struct {
		cycle string
		at    time.Time
		want  string
	}{
		{"daily", time.Date(2026, 9, 15, 10, 0, 0, 0, loc), "2026-09-15"},
		{"monthly", time.Date(2026, 9, 1, 0, 0, 0, 0, loc), "2026-09"},
		{"quarterly", time.Date(2026, 8, 1, 0, 0, 0, 0, loc), "2026-Q3"},
		{"quarterly", time.Date(2026, 1, 1, 0, 0, 0, 0, loc), "2026-Q1"},
		{"yearly", time.Date(2026, 12, 31, 0, 0, 0, 0, loc), "2026"},
	} {
		if got := periodKey(tt.cycle, tt.at); got != tt.want {
			t.Fatalf("periodKey(%s,%v)=%s want %s", tt.cycle, tt.at, got, tt.want)
		}
	}
	// 同一周期内不同日期返回相同期次键（重跑幂等的基础）。
	a := periodKey("monthly", time.Date(2026, 9, 2, 8, 0, 0, 0, loc))
	b := periodKey("monthly", time.Date(2026, 9, 30, 23, 0, 0, 0, loc))
	if a != b || a != "2026-09" {
		t.Fatalf("monthly key not stable: %s vs %s", a, b)
	}
	// 跨周期键不同。
	if periodKey("monthly", time.Date(2026, 9, 1, 0, 0, 0, 0, loc)) ==
		periodKey("monthly", time.Date(2026, 10, 1, 0, 0, 0, 0, loc)) {
		t.Fatalf("adjacent months must differ")
	}
}
