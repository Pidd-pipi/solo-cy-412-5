package service

import (
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

// dueDate 取期次起始与计划开始日的较晚者，避免为计划生效前的期次生成任务。
func dueDate(cycle string, start, now time.Time) time.Time {
	p := periodStart(cycle, now)
	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, now.Location())
	if startDay.After(p) {
		return startDay
	}
	return p
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
