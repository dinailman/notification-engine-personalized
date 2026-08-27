package rules

import "time"

// ValidQuietHours reports whether start and end form a usable quiet window. Both must
// parse as "15:04" and they must differ, since an empty window and a full day are the
// same input and the API should not have to guess which was meant.
func ValidQuietHours(start, end string) bool {
	if _, err := time.Parse("15:04", start); err != nil {
		return false
	}
	if _, err := time.Parse("15:04", end); err != nil {
		return false
	}
	return start != end
}

// NextAllowed reports whether at falls inside the user's quiet window and, if so, the
// instant the window closes. start and end are "15:04" wall clock in loc; an end at or
// before start wraps past midnight, so 22:00-07:00 is the overnight window it reads as.
// A window that is empty or does not parse is never quiet.
//
// The returned instant is built with time.Date in loc, so it accounts for a DST shift
// inside the window. If the closing wall-clock time does not exist on that local day --
// the spring-forward hour -- time.Date normalises forward, which releases the
// notification at the first instant that does exist rather than never.
func NextAllowed(at time.Time, loc *time.Location, start, end string) (time.Time, bool) {
	if loc == nil || !ValidQuietHours(start, end) {
		return at, false
	}
	from, _ := time.Parse("15:04", start)
	until, _ := time.Parse("15:04", end)

	local := at.In(loc)
	year, month, day := local.Date()
	opensAt := time.Date(year, month, day, from.Hour(), from.Minute(), 0, 0, loc)
	closesAt := time.Date(year, month, day, until.Hour(), until.Minute(), 0, 0, loc)

	if start < end {
		// A window inside a single local day, such as 13:00-14:00.
		if !local.Before(opensAt) && local.Before(closesAt) {
			return closesAt.UTC(), true
		}
		return at, false
	}
	// A window wrapping local midnight, such as 22:00-07:00.
	if !local.Before(opensAt) {
		// The evening leg: the window closes on the following local day.
		return closesAt.AddDate(0, 0, 1).UTC(), true
	}
	if local.Before(closesAt) {
		// The morning leg, still inside the window that opened yesterday.
		return closesAt.UTC(), true
	}
	return at, false
}
