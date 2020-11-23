// Copyright (c) 2019 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package graphite

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/m3db/m3/src/query/graphite/errors"
)

var reRelativeTime = regexp.MustCompile(`(?i)^\-([0-9]+)(s|min|h|d|w|mon|y)$`)    // allows -3min, -4d, etc.
var reTimeOffset = regexp.MustCompile(`(?i)^(\-|\+)([0-9]+)(s|min|h|d|w|mon|y)$`) // -3min, +3min, -4d, +4d
var reMonthAndDay = regexp.MustCompile(`(?i)^(january|february|march|april|may|june|july|august|september|october|november|december)([0-9]{1,2}?)$`)
var reDayOfWeek = regexp.MustCompile(`(?i)^(sunday|monday|tuesday|wednesday|thursday|friday|saturday)$`)
var reDayOfWeekOffset = regexp.MustCompile(`(?i)^(\-|\+)(sunday|monday|tuesday|wednesday|thursday|friday|saturday)$`) // +monday, +thursday, etc
var rePM = regexp.MustCompile(`(?i)^(([0-1]?)([0-9])pm)([[:ascii:]])*$`)                                              // 8pm, 12pm, 8pm monday
var reAM = regexp.MustCompile(`(?i)^(([0-1]?)([0-9])am)([[:ascii:]])*$`)                                              // 2am, 11am, 7am yesterday
var reTimeOfDayWithColon = regexp.MustCompile(`(?i)^(([0-1]?)([0-9]):([0-5])([0-9])((am|pm)?))([[:ascii:]])*$`)       // 8:12pm, 11:20am, 2:00am

var periods = map[string]time.Duration{
	"s":   time.Second,
	"min": time.Minute,
	"h":   time.Hour,
	"d":   time.Hour * 24,
	"w":   time.Hour * 24 * 7,
	"mon": time.Hour * 24 * 30,
	"y":   time.Hour * 24 * 365,
}

var weekdays = map[string]int{
	"sunday":    0,
	"monday":    1,
	"tuesday":   2,
	"wednesday": 3,
	"thursday":  4,
	"friday":    5,
	"saturday":  6,
}

var months = map[string]int{
	"january":   1,
	"february":  2,
	"march":     3,
	"april":     4,
	"may":       5,
	"june":      6,
	"july":      7,
	"august":    8,
	"september": 9,
	"october":   10,
	"november":  11,
	"december":  12,
}

// on Jan 2 15:04:05 -0700 MST 2006
var formats = []string{
	"15:04_060102",
	"15:04_20060102",
	"15:04_01/02/06",
	"15:04_02.01.06",
	"02.01.06",
	"01/02/06",
	"01/02/2006",
	"060102",
	"20060102",
}

// use init to rewrite formats to bypass bug in time.Parse
func init() {
	for i := range formats {
		formats[i] = bypassTimeParseBug(formats[i])
	}
}

func bypassTimeParseBug(s string) string {
	// NB(jayp): Looks like there is a bug in Golang's time.Parse when handing format strings
	// with _2 in the format string. Here is a snippet that exhibits this issue:
	//     t, e := time.Parse("15:04_20060102", "14:38_20150618")
	// We replace underscores with space to bypass this bug.
	return strings.Replace(s, "_", " ", -1)
}

// FormatTime translates a time.Time until a Graphite from/until string
func FormatTime(t time.Time) string {
	return t.Format(formats[0])
}

func getWeekDayOffset(weekday string, now time.Time) time.Duration {
	expectedDay := weekdays[weekday]
	today := int(now.Weekday())
	dayOffset := today - expectedDay
	if dayOffset < 0 {
		dayOffset += 7
	}

	return time.Duration(dayOffset) * time.Hour * -24
}

// ParseTime translates a Graphite from/until string into a timestamp relative to the provide time
func ParseTime(s string, now time.Time, absoluteOffset time.Duration) (time.Time, error) {
	if len(s) == 0 {
		return now, errors.NewInvalidParamsError(fmt.Errorf("time cannot be empty"))
	}

	if s == "now" {
		return now, nil
	}

	if m := reRelativeTime.FindStringSubmatch(s); len(m) != 0 {
		timePast, err := strconv.ParseInt(m[1], 10, 32)
		if err != nil {
			return now, errors.NewInvalidParamsError(fmt.Errorf("invalid relative time %v", err))
		}

		period := periods[strings.ToLower(m[2])]
		return now.Add(-1 * time.Duration(timePast) * period), nil
	}

	newS := bypassTimeParseBug(s)
	for _, format := range formats {
		t, err := time.Parse(format, newS)
		if err == nil {
			// Absolute time passed in, applying offset
			return t.Add(absoluteOffset), nil
		}
	}

	n, err := strconv.ParseInt(s, 10, 64)
	if err == nil {
		return time.Unix(n, 0).UTC(), nil
	}

	s = strings.Replace(strings.Replace(strings.ToLower(s), ",", "", -1), " ", "", -1)
	ref, offset := s, ""
	if strings.Contains(s, "+") {
		stringSlice := strings.Split(s, "+")
		if len(stringSlice) == 2 {
			ref, offset = stringSlice[0], stringSlice[1]
			offset = "+" + offset
		} else {
			return now, errors.NewInvalidParamsError(fmt.Errorf("unknown time string %s", s))
		}
	} else if strings.Contains(s, "-") {
		stringSlice := strings.Split(s, "-")
		if len(stringSlice) == 2 {
			ref, offset = stringSlice[0], stringSlice[1]
			offset = "-" + offset
		} else {
			return now, errors.NewInvalidParamsError(fmt.Errorf("unknown time string %s", s))
		}
	}
	parsedReference, err := ParseTimeReference(ref, now)
	if err == nil {
		parsedOffset, err := ParseOffset(offset, now)
		if err == nil {
			return parsedReference.Add(parsedOffset), nil
		}
	}

	return now, err
}

// ParseTimeReference takes a Graphite time reference ("8am", "today", "monday") and returns a time.Time
func ParseTimeReference(ref string, now time.Time) (time.Time, error) {
	var (
		hour    = now.Hour()
		minute  = now.Minute()
		refDate time.Time
	)

	if ref == "" || ref == "now" {
		return now, nil
	}

	// check if the time ref fits an absolute time format
	for _, format := range formats {
		t, err := time.Parse(format, ref)
		if err == nil {
			return t, nil
		}
	}

	rawRef := ref

	// Time of Day Reference (8:12pm, 11:20am, 2:00am, etc.)
	if reTimeOfDayWithColon.MatchString(rawRef) {
		i := strings.Index(ref, ":")
		newHour, err := strconv.ParseInt(ref[:i], 10, 0)
		if err != nil {
			return time.Time{}, err
		}
		hour = int(newHour)
		if len(ref) >= i+3 {
			newMinute, err := strconv.ParseInt(ref[i+1:i+3], 10, 32)
			if err != nil {
				return time.Time{}, err
			}
			minute = int(newMinute)
			if minute > 59 {
				return time.Time{}, errors.NewInvalidParamsError(fmt.Errorf("unknown time reference %s", rawRef))
			}
			ref = ref[i+3:]
		}

		if len(ref) >= 2 {
			if ref[:2] == "am" {
				ref = ref[2:]
			} else if ref[:2] == "pm" {
				hour = (hour + 12) % 24
				ref = ref[2:]
			}
		}
	}

	// Xam or XXam
	if reAM.MatchString(rawRef) {
		i := strings.Index(ref, "am")
		newHour, err := strconv.ParseInt(ref[:i], 10, 32)
		if err != nil {
			return time.Time{}, err
		}
		hour = int(newHour)
		minute = 0
		ref = ref[i+2:]
	}

	// Xpm or XXpm
	if rePM.MatchString(rawRef) {
		i := strings.Index(ref, "pm")
		newHour, err := strconv.ParseInt(ref[:i], 10, 32)
		if err != nil {
			return time.Time{}, err
		}
		if newHour > 24 {
			return time.Time{}, errors.NewInvalidParamsError(fmt.Errorf("unknown time reference %s", rawRef))
		}
		hour = int((newHour + 12) % 24)
		minute = 0
		ref = ref[i+2:]
	}

	if strings.HasPrefix(ref, "noon") {
		hour, minute = 12, 0
		ref = ref[4:]
	} else if strings.HasPrefix(ref, "midnight") {
		hour, minute = 0, 0
		ref = ref[8:]
	} else if strings.HasPrefix(ref, "teatime") {
		hour, minute = 16, 0
		ref = ref[7:]
	}

	refDate = time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())

	// Day reference
	if ref == "yesterday" {
		refDate = refDate.Add(time.Hour * -24)
	} else if ref == "tomorrow" {
		refDate = refDate.Add(time.Hour * 24)
	} else if ref == "today" {
		return refDate, nil
	} else if reMonthAndDay.MatchString(ref) { // monthDay (january10, may6, may06 etc.)
		day := 0
		monthString := ""
		if val, err := strconv.ParseInt(ref[len(ref)-2:], 10, 64); err == nil {
			day = int(val)
			monthString = ref[:len(ref)-2]
		} else if val, err := strconv.ParseInt(ref[len(ref)-1:], 10, 64); err == nil {
			day = int(val)
			monthString = ref[:len(ref)-1]
		} else {
			return refDate, errors.NewInvalidParamsError(fmt.Errorf("day of month required after month name"))
		}
		refDate = time.Date(refDate.Year(), time.Month(months[monthString]), day, hour, minute, 0, 0, refDate.Location())
	} else if reDayOfWeek.MatchString(ref) { // DayOfWeek (Monday, etc)
		refDate = refDate.Add(getWeekDayOffset(ref, refDate))
	} else if ref != "" {
		return time.Time{}, errors.NewInvalidParamsError(fmt.Errorf("unknown time reference %s", rawRef))
	}

	return refDate, nil
}

// ParseDuration parses a duration
func ParseDuration(s string) (time.Duration, error) {
	if m := reRelativeTime.FindStringSubmatch(s); len(m) != 0 {
		timePast, err := strconv.ParseInt(m[1], 10, 32)
		if err != nil {
			return 0, errors.NewInvalidParamsError(fmt.Errorf("invalid relative time %v", err))
		}

		period := periods[strings.ToLower(m[2])]
		return -1 * time.Duration(timePast) * period, nil
	}

	return 0, errors.NewInvalidParamsError(fmt.Errorf("invalid relative time %s", s))
}

// ParseOffset parses a time offset (like a duration, but can be 0 or positive)
func ParseOffset(s string, now time.Time) (time.Duration, error) {
	if s == "" {
		return time.Duration(0), nil
	}

	if m := reTimeOffset.FindStringSubmatch(s); len(m) != 0 {
		parity := 1
		if m[1] == "-" {
			parity = -1
		}
		timeInteger, err := strconv.ParseInt(m[2], 10, 32)
		if err != nil {
			return 0, errors.NewInvalidParamsError(fmt.Errorf("invalid time offset %v", err))
		}
		period := periods[strings.ToLower(m[3])]
		return period * time.Duration(timeInteger) * time.Duration(parity), nil
	}

	return 0, errors.NewInvalidParamsError(fmt.Errorf("invalid time offset %s", s))
}
