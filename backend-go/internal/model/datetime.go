package model

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// DateTime 兼容无时区(如 2026-08-20 18:00:00)与 RFC3339 格式的时间类型。
// 内嵌 time.Time 并实现 driver.Valuer / sql.Scanner，便于 GORM 存取 SQLite。
type DateTime struct {
	time.Time
}

func NewDateTime(t time.Time) *DateTime {
	return &DateTime{Time: t}
}

// UnmarshalJSON 支持 "2006-01-02 15:04"、"2006-01-02" 与 RFC3339 等格式
func (d *DateTime) UnmarshalJSON(data []byte) error {
	s := string(data)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if s == "" || s == "null" {
		return nil
	}
	layouts := []string{
		"2006-01-02 15:04",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			d.Time = t
			return nil
		}
	}
	return fmt.Errorf("无法解析时间: %s", s)
}

// MarshalJSON 输出无时区格式，与前端展示约定一致
func (d DateTime) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.Time.Format("2006-01-02 15:04") + `"`), nil
}

// Value 实现 driver.Valuer
func (d DateTime) Value() (driver.Value, error) {
	return d.Time, nil
}

// Scan 实现 sql.Scanner
func (d *DateTime) Scan(value any) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case time.Time:
		d.Time = v
	case string:
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", v, time.Local); err == nil {
			d.Time = t
			return nil
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			d.Time = t
			return nil
		}
		return fmt.Errorf("无法扫描时间: %v", value)
	case []byte:
		return d.Scan(string(v))
	default:
		return fmt.Errorf("无法扫描时间: %v", value)
	}
	return nil
}
