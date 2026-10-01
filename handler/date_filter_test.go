package handler

import (
	"testing"
	"time"
)

// 日期必须按本地时区解析。用 time.Parse 会得到 UTC 零点，
// MySQL 驱动（DSN 带 loc=Local）会把它转成本地 08:00，筛选窗口整体偏移 8 小时。
func TestParseDayStartUsesLocalTimezone(t *testing.T) {
	got, ok := parseDayStart("2026-10-01")
	if !ok {
		t.Fatal("解析失败")
	}
	want := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("起始时刻 = %s, want %s", got, want)
	}
	if got.Location() != time.Local {
		t.Fatalf("时区 = %s, want %s（UTC 解析会让窗口偏移时区差）", got.Location(), time.Local)
	}
	if h, m, s := got.Clock(); h != 0 || m != 0 || s != 0 {
		t.Fatalf("起始时刻应为当天零点，实际 %02d:%02d:%02d", h, m, s)
	}
}

// 结束日要取次日零点，配合 < 使用才能包含结束日全天。
func TestParseDayEndCoversWholeDay(t *testing.T) {
	got, ok := parseDayEnd("2026-10-01")
	if !ok {
		t.Fatal("解析失败")
	}
	want := time.Date(2026, 10, 2, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("结束边界 = %s, want %s（应为次日零点）", got, want)
	}

	// 结束日当天最后一刻必须落在区间内
	lastMoment := time.Date(2026, 10, 1, 23, 59, 59, 0, time.Local)
	if !lastMoment.Before(got) {
		t.Fatalf("%s 应当落在筛选区间内", lastMoment)
	}
	// 次日零点不能落在区间内
	nextDay := time.Date(2026, 10, 2, 0, 0, 0, 0, time.Local)
	if nextDay.Before(got) {
		t.Fatalf("%s 不应落在筛选区间内", nextDay)
	}
}

func TestParseDayRejectsBlankAndInvalid(t *testing.T) {
	for _, v := range []string{"", "   ", "not-a-date", "2026/10/01", "20261001"} {
		if _, ok := parseDayStart(v); ok {
			t.Fatalf("parseDayStart(%q) 应判定为无效", v)
		}
		if _, ok := parseDayEnd(v); ok {
			t.Fatalf("parseDayEnd(%q) 应判定为无效", v)
		}
	}
	// 两端留白应被容忍
	if _, ok := parseDayStart(" 2026-10-01 "); !ok {
		t.Fatal("带空格的日期应能解析")
	}
}

// 同一天的起止组合要能覆盖当天从零点到 23:59:59 的全部时刻。
func TestSingleDayRangeCoversFullDay(t *testing.T) {
	from, ok1 := parseDayStart("2026-10-01")
	to, ok2 := parseDayEnd("2026-10-01")
	if !ok1 || !ok2 {
		t.Fatal("解析失败")
	}

	for _, moment := range []time.Time{
		time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local),
		time.Date(2026, 10, 1, 0, 30, 0, 0, time.Local),
		time.Date(2026, 10, 1, 7, 59, 59, 0, time.Local),
		time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local),
		time.Date(2026, 10, 1, 23, 59, 59, 0, time.Local),
	} {
		if moment.Before(from) || !moment.Before(to) {
			t.Fatalf("%s 应落在 2026-10-01 的筛选区间内", moment)
		}
	}

	// 相邻日期的边界时刻必须被排除
	for _, moment := range []time.Time{
		time.Date(2026, 9, 30, 23, 59, 59, 0, time.Local),
		time.Date(2026, 10, 2, 0, 0, 0, 0, time.Local),
	} {
		if !moment.Before(from) && moment.Before(to) {
			t.Fatalf("%s 不应落在 2026-10-01 的筛选区间内", moment)
		}
	}
}
