package handler

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	_ "time/tzdata"
)

func webhookFilterDates(values url.Values) (*time.Time, *time.Time, error) {
	zone := strings.TrimSpace(values.Get("tz"))
	if zone == "" {
		zone = "UTC"
	}
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "Local" {
		return nil, nil, fmt.Errorf("unknown timezone %q; select a named timezone or UTC", zone)
	}
	from, err := parseWebhookDateTime(values.Get("from"), false, loc)
	if err != nil {
		return nil, nil, fmt.Errorf("From: %w", err)
	}
	to, err := parseWebhookDateTime(values.Get("to"), true, loc)
	if err != nil {
		return nil, nil, fmt.Errorf("To: %w", err)
	}
	return from, to, nil
}

// Look at the surrounding zone intervals rather than letting time.Date choose
// silently between the two occurrences of a clock time during a DST overlap.
func resolveLocalTime(wall time.Time, loc *time.Location) (*time.Time, error) {
	const layout = "2006-01-02T15:04:05.999999999"
	var result *time.Time
	end := wall.Add(48 * time.Hour)
	for cursor := wall.Add(-48 * time.Hour); !cursor.After(end); {
		local := cursor.In(loc)
		_, offset := local.Zone()
		candidate := wall.Add(-time.Duration(offset) * time.Second)
		if candidate.In(loc).Format(layout) == wall.Format(layout) {
			if result != nil && !result.Equal(candidate) {
				return nil, fmt.Errorf("%s occurs twice in %s; choose a time outside the clock change or use an RFC3339 value with an explicit offset", wall.Format(layout), loc)
			}
			result = &candidate
		}
		_, next := local.ZoneBounds()
		if next.IsZero() || next.After(end) {
			break
		}
		// Extrapolated year-end bounds can precede the cursor in leap years.
		if !next.After(cursor) {
			next = time.Date(cursor.Year()+1, time.January, 1, 0, 0, 0, 0, time.UTC)
		}
		cursor = next
	}
	if result == nil {
		return nil, fmt.Errorf("%s does not exist in %s; choose a time outside the clock change", wall.Format(layout), loc)
	}
	return result, nil
}

func canonicalDateQuery(values url.Values, from, to *time.Time) url.Values {
	values.Del("tz")
	for name, value := range map[string]*time.Time{"from": from, "to": to} {
		values.Del(name)
		if value != nil {
			values.Set(name, value.UTC().Format(time.RFC3339Nano))
		}
	}
	return values
}

func dateTimeInstant(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
