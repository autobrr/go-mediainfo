package mediainfo

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	referenceUnixDatePattern  = regexp.MustCompile(`^([A-Za-z]+) +([A-Za-z]+) +(\d{1,2}) +(\d{1,2}):(\d{1,2}):(\d{1,2}) +(\d{4})$`)
	referenceCommaDatePattern = regexp.MustCompile(`^([A-Za-z]+), +([A-Za-z]+) +(\d{1,2}), +(\d{4})(?: +(\d{1,2}):(\d{1,2}):(\d{1,2}) +([AaPp][Mm]))?$`)
	referenceSlashDatePattern = regexp.MustCompile(`^(\d*)/(\d*)/(\d*)$`)
)

// adaptReferenceDate applies MediaInfo's General date adapter. It returns ""
// for "unknown" and keeps every value the adapter does not recognize. v26
// adds the v26.10 colon-date and +hhmm offset forms.
func adaptReferenceDate(value string, v26 bool) string {
	if len(value) < 4 {
		return value
	}
	date, utc := strings.CutPrefix(value, "UTC ")
	if len(date) < 5 {
		return value
	}
	if date[4] != '-' {
		if date == "unknown" {
			return ""
		}
		if adapted, ok := adaptReferenceTextDate(strings.TrimRight(date, " \t\r\n\v\f")); ok {
			return adapted
		}
	}
	if adapted, ok := adaptReferenceISODate([]byte(date), utc, v26); ok {
		return adapted
	}
	return value
}

// adaptReferenceTextDate reads the C-locale "%a %b %d %T %Y",
// "%a, %b %d, %Y %r", "%a, %b %d, %Y" and day/month/year forms.
func adaptReferenceTextDate(value string) (string, bool) {
	if parts := referenceUnixDatePattern.FindStringSubmatch(value); parts != nil {
		return formatReferenceTextDate(parts[1], parts[2], parts[3], parts[7], parts[4], parts[5], parts[6], "")
	}
	if parts := referenceCommaDatePattern.FindStringSubmatch(value); parts != nil {
		return formatReferenceTextDate(parts[1], parts[2], parts[3], parts[4], parts[5], parts[6], parts[7], parts[8])
	}
	parts := referenceSlashDatePattern.FindStringSubmatch(value)
	if parts == nil {
		return "", false
	}
	day, _ := strconv.Atoi(parts[1])
	month, _ := strconv.Atoi(parts[2])
	year, _ := strconv.Atoi(parts[3])
	// Deliberate difference: keep the text where the reference reads a short
	// year as 19xx or a month part of 12 as the day (AGENTS.md).
	if len(parts[3]) != 4 || year <= 1900 {
		return "", false
	}
	if month > 12 && day <= 31 {
		day, month = month, day
	}
	if day < 1 || day > 31 || month < 1 || month > 11 {
		return "", false
	}
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day), true
}

func formatReferenceTextDate(weekday, monthName, dayText, yearText, hourText, minuteText, secondText, meridiem string) (string, bool) {
	month := referenceMonthNumber(monthName)
	day, _ := strconv.Atoi(dayText)
	year, _ := strconv.Atoi(yearText)
	if !referenceWeekdayName(weekday) || month == 0 || day < 1 || day > 31 || year == 1900 {
		return "", false
	}
	date := fmt.Sprintf("%04d-%02d-%02d", year, month, day)
	if hourText == "" {
		return date, true
	}
	hour, _ := strconv.Atoi(hourText)
	minute, _ := strconv.Atoi(minuteText)
	second, _ := strconv.Atoi(secondText)
	if meridiem != "" {
		if hour < 1 || hour > 12 {
			return "", false
		}
		hour %= 12
		if strings.EqualFold(meridiem, "PM") {
			hour += 12
		}
	}
	if hour > 23 || minute > 59 || second > 60 {
		return "", false
	}
	return fmt.Sprintf("%s %02d:%02d:%02d", date, hour, minute, second), true
}

func referenceWeekdayName(name string) bool {
	return slices.ContainsFunc([]string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}, func(day string) bool {
		return strings.EqualFold(name, day) || strings.EqualFold(name, day[:3])
	})
}

func referenceMonthNumber(name string) int {
	return 1 + slices.IndexFunc([]string{"january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december"}, func(month string) bool {
		return strings.EqualFold(name, month) || strings.EqualFold(name, month[:3])
	})
}

// adaptReferenceISODate ports the reference year[-month[-day[ time[.frac][zone]]]]
// walk: it keeps the separator at index 10 unless it is "T" or ", ".
func adaptReferenceISODate(value []byte, utc, v26 bool) (string, bool) {
	digits := func(indexes ...int) bool {
		for _, index := range indexes {
			if index >= len(value) || value[index] < '0' || value[index] > '9' {
				return false
			}
		}
		return true
	}
	finalize := func(utc bool) (string, bool) {
		out := string(value)
		if utc {
			out += " UTC"
		}
		return out, true
	}
	if !digits(0, 1, 2, 3) {
		return "", false
	}
	if len(value) == 4 {
		return finalize(utc)
	}
	for _, separator := range []int{4, 7} {
		if len(value) <= separator+2 || (value[separator] != '-' && (!v26 || value[separator] != ':')) || !digits(separator+1, separator+2) {
			return "", false
		}
		value[separator] = '-'
		if len(value) == separator+3 {
			return finalize(utc)
		}
	}
	if len(value) <= 11 {
		return "", false
	}
	if value[10] == 'T' {
		value[10] = ' '
	}
	if value[10] == ',' && value[11] == ' ' {
		value = slices.Delete(value, 10, 11)
	}
	if len(value) <= 18 || !digits(11, 12, 14, 15, 17, 18) || value[13] != ':' || value[16] != ':' {
		return "", false
	}
	i := 19
	if i < len(value) && value[i] == '.' {
		i++
		for i < len(value) && value[i] >= '0' && value[i] <= '9' {
			i++
		}
	}
	if len(value) == i {
		return finalize(utc)
	}
	if value[i] == 'Z' {
		if len(value) != i+1 {
			return "", false
		}
		value = value[:i]
		return finalize(true)
	}
	if len(value) <= i+2 || (value[i] != '+' && value[i] != '-') || !digits(i+1, i+2) {
		return "", false
	}
	if len(value) == i+3 {
		return finalize(utc)
	}
	if v26 && len(value) == i+5 && digits(i+3, i+4) {
		value = slices.Insert(value, i+3, ':')
	}
	if len(value) != i+6 || value[i+3] != ':' || !digits(i+4, i+5) {
		return "", false
	}
	if string(value[i+1:i+3]) == "00" && string(value[i+4:i+6]) == "00" {
		value = value[:i]
		utc = true
	}
	return finalize(utc)
}
