package util

import (
	"fmt"
	"time"
)

func ParseTimestamp(ts string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		switch err := err.(type) {
		case *time.ParseError:
			// If err is a time.ParseError then its string representation is not
			// appropriate since it relies on details of Go's strange date format
			// representation, which a caller of our functions is not expected
			// to be familiar with.
			//
			// Therefore we do some light transformation to get a more suitable
			// error that should make more sense to our callers. These are
			// still not awesome error messages, but at least they refer to
			// the timestamp portions by name rather than by Go's example
			// values.
			if err.LayoutElem == "" && err.ValueElem == "" && err.Message != "" {
				// For some reason err.Message is populated with a ": " prefix
				// by the time package.
				return time.Time{}, fmt.Errorf("not a valid RFC3339 timestamp%s", err.Message)
			}
			var what string
			switch err.LayoutElem {
			case "2006":
				what = "year"
			case "01":
				what = "month"
			case "02":
				what = "day of month"
			case "15":
				what = "hour"
			case "04":
				what = "minute"
			case "05":
				what = "second"
			case "Z07:00":
				what = "UTC offset"
			case "T":
				return time.Time{}, fmt.Errorf("not a valid RFC3339 timestamp: missing required time introducer 'T'")
			case ":", "-":
				if err.ValueElem == "" {
					return time.Time{}, fmt.Errorf("not a valid RFC3339 timestamp: end of string where %q is expected", err.LayoutElem)
				} else {
					return time.Time{}, fmt.Errorf("not a valid RFC3339 timestamp: found %q where %q is expected", err.ValueElem, err.LayoutElem)
				}
			default:
				// Should never get here, because time.RFC3339 includes only the
				// above portions, but since that might change in future we'll
				// be robust here.
				what = "timestamp segment"
			}
			if err.ValueElem == "" {
				return time.Time{}, fmt.Errorf("not a valid RFC3339 timestamp: end of string before %s", what)
			} else {
				return time.Time{}, fmt.Errorf("not a valid RFC3339 timestamp: cannot use %q as %s", err.ValueElem, what)
			}
		}
		return time.Time{}, err
	}
	return t, nil
}
