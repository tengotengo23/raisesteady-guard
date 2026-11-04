package core

import (
	"strconv"
	"time"
)

// Schedule rules of one account (P2-160). Every time passed in is already in
// the agent's fixed zone (Agent.Local), so the weekday and the hour follow the
// same clock as the day and bedtime.

// IsWeekendDay reports whether the day's minutes are the weekend ones (Saturday, Sunday).
func IsWeekendDay(d time.Weekday) bool { return d == time.Saturday || d == time.Sunday }

// IsWeekendNight reports whether the night that starts on this day uses the
// weekend bedtime (Friday and Saturday nights).
func IsWeekendNight(d time.Weekday) bool { return d == time.Friday || d == time.Saturday }

// IsSchoolDay is Monday to Friday.
func IsSchoolDay(d time.Weekday) bool { return !IsWeekendDay(d) }

// MinuteOf turns "HH:MM" into minutes after midnight (-1 when malformed).
func MinuteOf(hhmm string) int {
	if !ValidHHMM(hhmm) {
		return -1
	}
	h, _ := strconv.Atoi(hhmm[:2])
	m, _ := strconv.Atoi(hhmm[3:])
	return h*60 + m
}

// LimitsOn returns the browsing and video minutes for the day of t.
func (s *Settings) LimitsOn(t time.Time) (browse, video int) {
	if s.Weekend.On && IsWeekendDay(t.Weekday()) {
		return s.Weekend.BrowseMin, s.Weekend.VideoMin
	}
	return s.BrowseMin, s.VideoMin
}

// NightBedtime is the bedtime of the night that starts on day d.
func (s *Settings) NightBedtime(d time.Weekday) Bedtime {
	if s.Weekend.On && IsWeekendNight(d) {
		return s.Weekend.Bedtime
	}
	return s.Bedtime
}

func crosses(b Bedtime) bool {
	st, en := MinuteOf(b.Start), MinuteOf(b.End)
	return st >= 0 && en >= 0 && st > en
}

// BedtimeNow is the bedtime that matters at t: last night's while its morning
// part is still running, else tonight's.
func (s *Settings) BedtimeNow(t time.Time) Bedtime {
	m := t.Hour()*60 + t.Minute()
	last := s.NightBedtime(t.AddDate(0, 0, -1).Weekday())
	if crosses(last) && m < MinuteOf(last.End) {
		return last
	}
	return s.NightBedtime(t.Weekday())
}

// BedtimeActiveAt reports whether t is inside bedtime: last night's morning
// part, or tonight's window.
func (s *Settings) BedtimeActiveAt(t time.Time) bool {
	m := t.Hour()*60 + t.Minute()
	last := s.NightBedtime(t.AddDate(0, 0, -1).Weekday())
	if crosses(last) && m < MinuteOf(last.End) {
		return true
	}
	b := s.NightBedtime(t.Weekday())
	st, en := MinuteOf(b.Start), MinuteOf(b.End)
	switch {
	case st < 0 || en < 0 || st == en:
		return false
	case st < en:
		return m >= st && m < en
	default:
		return m >= st
	}
}

// HomeworkActive reports whether t is inside the homework window on a school day.
func (s *Settings) HomeworkActive(t time.Time) bool {
	h := s.Homework
	if !h.On || !IsSchoolDay(t.Weekday()) {
		return false
	}
	m := t.Hour()*60 + t.Minute()
	st, en := MinuteOf(h.Start), MinuteOf(h.End)
	return st >= 0 && en > st && m >= st && m < en
}

// PausedAt returns whether the internet is paused at t, and until when.
func (s *Settings) PausedAt(t time.Time) (bool, time.Time) {
	if s.PauseUntil == "" {
		return false, time.Time{}
	}
	end, err := time.Parse(time.RFC3339, s.PauseUntil)
	if err != nil || !t.Before(end) {
		return false, time.Time{}
	}
	return true, end.In(t.Location())
}

// NextMorning is the next 06:00 after t in t's zone ("until tomorrow"): the
// same day's 06:00 when t is earlier in the night.
func NextMorning(t time.Time) time.Time {
	m := time.Date(t.Year(), t.Month(), t.Day(), 6, 0, 0, 0, t.Location())
	if !m.After(t) {
		m = time.Date(t.Year(), t.Month(), t.Day()+1, 6, 0, 0, 0, t.Location())
	}
	return m
}

// GameMinutes is the daily game time: the parent's choice, else the band's video minutes.
func (s *Settings) GameMinutes(bands *Bands) int {
	if s.GameMin != nil {
		return *s.GameMin
	}
	if b, ok := bands.Get(s.Band); ok {
		return b.VideoMin
	}
	return 0
}

// AppMode is what an app may do in this account: the parent's choice, else
// game time for games, and for chat apps blocked under their minimum age.
func (s *Settings) AppMode(app PCApp, bands *Bands) string {
	if m, ok := s.Apps[app.ID]; ok {
		return m
	}
	return DefaultAppMode(app, s.Band, bands)
}

// DefaultAppMode is an app's mode before the parent changes it.
func DefaultAppMode(app PCApp, band string, bands *Bands) string {
	if app.Kind == "game" {
		return AppGame
	}
	b, ok := bands.Get(band)
	if !ok || b.MinAge < app.MinAge {
		return AppBlocked
	}
	return AppAllowed
}
