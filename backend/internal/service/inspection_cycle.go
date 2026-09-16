package service

import (
	"fmt"
	"strconv"
	"time"

	"github.com/smartestate/smartestate/internal/constants"
)

// periodKey 返回某时刻在给定周期下的期次键，作为任务唯一槽位的一部分：
// daily=2026-09-15，weekly=2026-W38，monthly=2006-01，quarterly=2026-Q3，yearly=2026。
func periodKey(cycle string, t time.Time) string {
	y, m, _ := t.Date()
	switch cycle {
	case constants.CycleDaily:
		return t.Format("2006-01-02")
	case constants.CycleWeekly:
		isoY, isoW := t.ISOWeek()
		return strconv.Itoa(isoY) + "-W" + pad2(isoW)
	case constants.CycleMonthly:
		return t.Format("2006-01")
	case constants.CycleQuarterly:
		return strconv.Itoa(y) + "-Q" + strconv.Itoa((int(m)-1)/3+1)
	case constants.CycleYearly:
		return strconv.Itoa(y)
	default:
		return t.Format("2006-01")
	}
}

// periodStart 返回期次起始时刻（本地零点）。
func periodStart(cycle string, t time.Time) time.Time {
	y, m, d := t.Date()
	loc := t.Location()
	switch cycle {
	case constants.CycleDaily:
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	case constants.CycleWeekly:
		isoY, isoW := t.ISOWeek()
		return isoWeekStart(isoY, isoW, loc)
	case constants.CycleMonthly:
		return time.Date(y, m, 1, 0, 0, 0, 0, loc)
	case constants.CycleQuarterly:
		q := (int(m)-1)/3 + 1
		return time.Date(y, time.Month((q-1)*3+1), 1, 0, 0, 0, 0, loc)
	case constants.CycleYearly:
		return time.Date(y, 1, 1, 0, 0, 0, 0, loc)
	default:
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
}

// duePeriod 一个到期期次：Key 为唯一槽位键，Anchor 为该期次在日历上的起始时刻，Due 为任务到期日。
type duePeriod struct {
	Key    string
	Anchor time.Time
	Due    time.Time
}

// maxBackfillPeriods 单次补齐的期次数上限，防止异常超远开始日导致的无限枚举。
const maxBackfillPeriods = 100000

// enumerateDuePeriods 按“计划开始日所在期次”到“当前期次”枚举全部到期期次（含两端），
// 跨月、跨季度、跨年使用同一套日历口径：日=自然日，周=ISO 周（周一），月=1 号，
// 季=季度首月 1 号，年=1 月 1 号。每个期次仅出现一次；开始日在未来时返回空。
func enumerateDuePeriods(cycle string, start, now time.Time) ([]duePeriod, error) {
	if !constants.ValidCycles[cycle] {
		return nil, fmt.Errorf("invalid cycle %q", cycle)
	}
	if start.IsZero() {
		return nil, fmt.Errorf("plan start date is zero")
	}
	loc := now.Location()
	start = start.In(loc)
	currentStart := periodStart(cycle, now)
	anchor := periodStart(cycle, start) // 开始日所在期次的起始
	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)

	out := make([]duePeriod, 0)
	for !anchor.After(currentStart) {
		due := anchor
		if startDay.After(due) {
			due = startDay // 首期：计划在该期次中途生效，到期日取开始日
		}
		out = append(out, duePeriod{Key: periodKey(cycle, anchor), Anchor: anchor, Due: due})
		if len(out) > maxBackfillPeriods {
			return nil, fmt.Errorf("backfill periods exceed limit %d", maxBackfillPeriods)
		}
		anchor = advanceAnchor(cycle, anchor)
	}
	return out, nil
}

// advanceAnchor 把期次起始推进到下一期，始终落在正确的日历边界上。
func advanceAnchor(cycle string, t time.Time) time.Time {
	switch cycle {
	case constants.CycleDaily:
		return t.AddDate(0, 0, 1)
	case constants.CycleWeekly:
		return t.AddDate(0, 0, 7)
	case constants.CycleMonthly:
		return t.AddDate(0, 1, 0)
	case constants.CycleQuarterly:
		return t.AddDate(0, 3, 0)
	case constants.CycleYearly:
		return t.AddDate(1, 0, 0)
	default:
		return t.AddDate(0, 1, 0)
	}
}

// recheckPeriod 复检任务的期次键，保证与常规任务槽位不冲突。
func recheckPeriod(repairID uint) string {
	return "recheck-" + strconv.FormatUint(uint64(repairID), 10)
}

// isoWeekStart 根据 ISO 年与周数计算该周周一。
func isoWeekStart(isoYear, isoWeek int, loc *time.Location) time.Time {
	jan4 := time.Date(isoYear, 1, 4, 0, 0, 0, 0, loc)
	wd := int(jan4.Weekday())
	if wd == 0 {
		wd = 7
	}
	firstMonday := jan4.AddDate(0, 0, -(wd - 1))
	return firstMonday.AddDate(0, 0, (isoWeek-1)*7)
}

func pad2(v int) string {
	if v < 10 {
		return "0" + strconv.Itoa(v)
	}
	return strconv.Itoa(v)
}
