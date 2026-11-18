package gateway

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yetone/magpie/internal/appdir"
)

// A provider's DayCalls is how many requests it may take in one calendar
// day on this computer's clock, all of its keys and accounts together
// (provider.DayCallLimit). Every request that is about to enter the
// provider counts — each try of the forward loop, a retry or the next key
// included, but not one held before anything was sent. The count is kept in
// daycalls.json in magpie's config folder, so a restart keeps it, and a day
// that has turned is dropped as it is read. One more request than the limit
// is turned away at once with a 429 whose Retry-After is the next midnight;
// nothing rests for it. A provider with no limit reads and writes nothing.
//
// The count is kept while the gateway runs under days.mu, and the file is
// written whole on each count: a second magpie using the same folder sees
// the file as it was when its own count started, as providers.json is. The
// day is read on dayNow, which tests hold.

// dayLayout is the calendar day's key in daycalls.json.
const dayLayout = "2006-01-02"

// dayNow is the clock a day is read on; tests hold it.
var dayNow = time.Now

// dayCallsPath is where the counts are kept: magpie's config folder, beside
// providers.json and settings.json.
func dayCallsPath() string { return filepath.Join(appdir.Config(), "daycalls.json") }

// dayCalls are how many requests each provider has taken on the days the
// file keeps. Only the day read is kept: one older is dropped as it turns.
type dayCalls struct {
	mu     sync.Mutex
	loaded bool
	days   map[string]map[string]int // day -> provider id -> requests
}

// dayCallsFile is daycalls.json's shape.
type dayCallsFile struct {
	Days map[string]map[string]int `json:"days"`
}

// loadLocked reads the file once, and drops every day but day.
func (d *dayCalls) loadLocked(day string) {
	if !d.loaded {
		d.loaded = true
		d.days = map[string]map[string]int{}
		b, err := os.ReadFile(dayCallsPath())
		switch {
		case err == nil:
			var f dayCallsFile
			if err := json.Unmarshal(b, &f); err != nil {
				log.Printf("daycalls: %v", err)
			} else if f.Days != nil {
				d.days = f.Days
			}
		case !os.IsNotExist(err):
			log.Printf("daycalls: %v", err)
		}
	}
	if d.days == nil {
		d.days = map[string]map[string]int{}
	}
	for k := range d.days {
		if k != day {
			delete(d.days, k)
		}
	}
}

// saveLocked writes the counts whole, a temp file renamed over the old one.
func (d *dayCalls) saveLocked() {
	b, err := json.Marshal(dayCallsFile{Days: d.days})
	if err != nil {
		return
	}
	path := dayCallsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("daycalls: %v", err)
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		log.Printf("daycalls: %v", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("daycalls: %v", err)
		os.Remove(tmp)
	}
}

// free says whether a request for id would enter the provider now: no
// limit, or fewer than limit requests counted today.
func (d *dayCalls) free(id string, limit int, now time.Time) bool {
	if limit <= 0 {
		return true
	}
	day := now.Format(dayLayout)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.loadLocked(day)
	return d.days[day][id] < limit
}

// take counts one request for id and answers ok; over the limit it counts
// nothing and answers how long until the next midnight.
func (d *dayCalls) take(id string, limit int, now time.Time) (ok bool, after time.Duration) {
	if limit <= 0 {
		return true, 0
	}
	day := now.Format(dayLayout)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.loadLocked(day)
	counts := d.days[day]
	if counts == nil {
		counts = map[string]int{}
		d.days[day] = counts
	}
	if counts[id] >= limit {
		return false, dayLeft(now)
	}
	counts[id]++
	d.saveLocked()
	return true, 0
}

// dayLeft is how long from now until the next midnight on this computer's
// clock.
func dayLeft(now time.Time) time.Duration {
	n := now.In(time.Local)
	return time.Date(n.Year(), n.Month(), n.Day()+1, 0, 0, 0, 0, time.Local).Sub(now)
}

// errDayCalls turns a request away that found its provider's day full:
// after is how long until the next midnight.
type errDayCalls struct {
	limit int
	after time.Duration
}

func (e *errDayCalls) Error() string {
	return fmt.Sprintf("%d requests a day is its limit, and the next can go in %.0fh, at the next midnight", e.limit, math.Ceil(e.after.Hours()))
}

// retryAfter is e's Retry-After, in whole seconds, at least 1.
func (e *errDayCalls) retryAfter() string {
	return fmt.Sprint(max(int(math.Ceil(e.after.Seconds())), 1))
}
