package util

import (
	"time"
)

// FormatTime 格式化时间
func FormatTime(t time.Time, layout string) string {
	return t.Format(layout)
}

// FormatDateTime 格式化为日期时间
func FormatDateTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// FormatDate 格式化为日期
func FormatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// ParseDateTime 解析日期时间
func ParseDateTime(s string) (time.Time, error) {
	return time.Parse("2006-01-02 15:04:05", s)
}

// ParseDate 解析日期
func ParseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// NowUnix 当前时间戳(秒)
func NowUnix() int64 {
	return time.Now().Unix()
}

// NowUnixMilli 当前时间戳(毫秒)
func NowUnixMilli() int64 {
	return time.Now().UnixMilli()
}

// UnixToTime 时间戳转时间
func UnixToTime(unix int64) time.Time {
	return time.Unix(unix, 0)
}

// UnixMilliToTime 毫秒时间戳转时间
func UnixMilliToTime(unixMilli int64) time.Time {
	return time.UnixMilli(unixMilli)
}

// IsToday 判断是否今天
func IsToday(t time.Time) bool {
	now := time.Now()
	return t.Year() == now.Year() && t.YearDay() == now.YearDay()
}

// IsYesterday 判断是否昨天
func IsYesterday(t time.Time) bool {
	yesterday := time.Now().AddDate(0, 0, -1)
	return t.Year() == yesterday.Year() && t.YearDay() == yesterday.YearDay()
}

// StartOfDay 获取一天的开始时间
func StartOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// EndOfDay 获取一天的结束时间
func EndOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
}
