package service

import (
	"testing"
	"time"

	"github.com/smartestate/smartestate/internal/constants"
)

func loc() *time.Location { return time.Local }

func TestEnumerateDuePeriodsCalendar(t *testing.T) {
	for _, tt := range []struct {
		name       string
		cycle      string
		start, now time.Time
		wantKeys   []string
		firstDue   time.Time
	}{
		{
			"跨月-按月补齐", constants.CycleMonthly,
			time.Date(2026, 1, 15, 9, 0, 0, 0, loc()), time.Date(2026, 3, 10, 0, 0, 0, 0, loc()),
			[]string{"2026-01", "2026-02", "2026-03"},
			time.Date(2026, 1, 15, 0, 0, 0, 0, loc()), // 首期到期取计划开始日
		},
		{
			"跨季度", constants.CycleQuarterly,
			time.Date(2025, 11, 1, 0, 0, 0, 0, loc()), time.Date(2026, 5, 20, 0, 0, 0, 0, loc()),
			[]string{"2025-Q4", "2026-Q1", "2026-Q2"},
			time.Date(2025, 11, 1, 0, 0, 0, 0, loc()), // 计划在季中生效，首期到期取开始日
		},
		{
			"跨年", constants.CycleYearly,
			time.Date(2024, 6, 1, 0, 0, 0, 0, loc()), time.Date(2026, 3, 1, 0, 0, 0, 0, loc()),
			[]string{"2024", "2025", "2026"},
			time.Date(2024, 6, 1, 0, 0, 0, 0, loc()), // 年中生效，首期到期取开始日
		},
		{
			"跨日", constants.CycleDaily,
			time.Date(2026, 9, 13, 8, 0, 0, 0, loc()), time.Date(2026, 9, 15, 0, 0, 0, 0, loc()),
			[]string{"2026-09-13", "2026-09-14", "2026-09-15"},
			time.Date(2026, 9, 13, 0, 0, 0, 0, loc()),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, e := enumerateDuePeriods(tt.cycle, tt.start, tt.now)
			if e != nil {
				t.Fatalf("枚举到期期次失败: %v", e)
			}
			if len(got) != len(tt.wantKeys) {
				t.Fatalf("期次数=%d, 期望 %d（%v）", len(got), len(tt.wantKeys), keysOf(got))
			}
			for i, w := range tt.wantKeys {
				if got[i].Key != w {
					t.Fatalf("第 %d 期键=%s, 期望 %s", i, got[i].Key, w)
				}
			}
			if !got[0].Due.Equal(tt.firstDue) {
				t.Fatalf("首期到期=%v, 期望 %v", got[0].Due, tt.firstDue)
			}
			// 每个期次的锚点必须落在日历边界上。
			for _, p := range got {
				if !p.Anchor.Equal(periodStart(tt.cycle, p.Anchor)) {
					t.Fatalf("期次 %s 锚点不在日历边界: %v", p.Key, p.Anchor)
				}
			}
		})
	}
}

func TestEnumerateDuePeriodsWeeklyUsesISOWeek(t *testing.T) {
	// 2026-09-14 为周一；到第 3 个周一 2026-09-28 应生成 3 个不同 ISO 周键，锚点均为周一。
	start := time.Date(2026, 9, 14, 0, 0, 0, 0, loc())
	now := time.Date(2026, 9, 28, 0, 0, 0, 0, loc())
	got, e := enumerateDuePeriods(constants.CycleWeekly, start, now)
	if e != nil {
		t.Fatalf("枚举周期失败: %v", e)
	}
	if len(got) != 3 {
		t.Fatalf("周期次数=%d, 期望 3（%v）", len(got), keysOf(got))
	}
	seen := map[string]bool{}
	for _, p := range got {
		if p.Anchor.Weekday() != time.Monday {
			t.Fatalf("周锚点不是周一: %v", p.Anchor)
		}
		if p.Key != periodKey(constants.CycleWeekly, p.Anchor) {
			t.Fatalf("周键与锚点不一致: %s", p.Key)
		}
		if seen[p.Key] {
			t.Fatalf("周键重复: %s", p.Key)
		}
		seen[p.Key] = true
	}
}

func TestEnumerateDuePeriodsFutureStart(t *testing.T) {
	got, e := enumerateDuePeriods(constants.CycleMonthly,
		time.Date(2026, 12, 1, 0, 0, 0, 0, loc()),
		time.Date(2026, 9, 1, 0, 0, 0, 0, loc()))
	if e != nil {
		t.Fatalf("枚举失败: %v", e)
	}
	if len(got) != 0 {
		t.Fatalf("开始日在未来不应补齐, got %d (%v)", len(got), keysOf(got))
	}
}

func keysOf(ps []duePeriod) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Key)
	}
	return out
}
