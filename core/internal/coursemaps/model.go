package coursemaps

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Route is a normalized, same-origin link discovered while crawling a course.
type Route struct {
	URL          string `json:"url"`
	Kind         string `json:"kind"`
	Title        string `json:"title,omitempty"`
	Depth        int    `json:"depth"`
	SourceURL    string `json:"sourceUrl,omitempty"`
	PhaseName    string `json:"phaseName,omitempty"`
	PhaseSection int    `json:"phaseSection,omitempty"`
	ActivityID   string `json:"activityId,omitempty"`
	Subsection   string `json:"subsection,omitempty"`
	Technical    bool   `json:"technical,omitempty"`
	// Restricted marks a route the authenticated session could not open
	// (e.g. a forum that redirects with "No dispone de permiso para ver los
	// debates de este foro"). It stays in the map for diagnostics but is
	// never chosen as evidence.
	Restricted bool `json:"restricted,omitempty"`
}

// Activity is an instructor-selectable assignment discovered in a course.
// It is deliberately derived from the route map so the user chooses from
// Zajuna's current titles and URLs instead of typing arbitrary links.
type Activity struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	PhaseName    string `json:"phaseName,omitempty"`
	PhaseSection int    `json:"phaseSection,omitempty"`
	Subsection   string `json:"subsection,omitempty"`
	Technical    bool   `json:"technical"`
}

// Stats keeps the useful high-level classification counts for a route map.
type Stats struct {
	Total     int `json:"total"`
	Phases    int `json:"phases"`
	Forums    int `json:"forums"`
	Pages     int `json:"pages"`
	Assigns   int `json:"assigns"`
	Gradings  int `json:"gradings"`
	URLs      int `json:"urls"`
	Resources int `json:"resources"`
}

// Record is the local, versionable route map for one Zajuna course.
// ByItemCode contains both generic route.* groups and the deterministic
// checklist projection used by directed capture. The projection is intentionally
// versionable so later title/phase rules can improve it without changing the
// persistence or worker contract.
type Record struct {
	CourseID      string                     `json:"courseId"`
	CourseURL     string                     `json:"courseUrl"`
	ProfileURL    string                     `json:"profileUrl"`
	ByItemCode    map[string]json.RawMessage `json:"byItemCode"`
	Routes        []Route                    `json:"routes"`
	LinkCount     int                        `json:"linkCount"`
	ItemCodeCount int                        `json:"itemCodeCount"`
	ScrapeStats   Stats                      `json:"scrapeStats"`
	Warning       string                     `json:"warning,omitempty"`
	Source        string                     `json:"source"`
	DiscoveredAt  time.Time                  `json:"discoveredAt"`
	UpdatedAt     time.Time                  `json:"updatedAt"`
}

// Store is the persistence contract used by the discovery worker and API.
type Store interface {
	CreateOrReplaceCourseMap(context.Context, Record) error
	GetCourseMap(context.Context, string) (Record, error)
	ListCourseMaps(context.Context, int) ([]Record, error)
}

// Activities returns one normalized entry per assignment activity. Grading
// and navigation routes are intentionally excluded because they are views of
// an activity, not activities the instructor should select.
func Activities(record Record) []Activity {
	byID := make(map[string]Activity)
	for _, route := range record.Routes {
		if route.Kind != "assign" {
			continue
		}
		activityID := strings.TrimSpace(route.ActivityID)
		if activityID == "" {
			parsed, err := url.Parse(route.URL)
			if err == nil {
				activityID = strings.TrimSpace(parsed.Query().Get("id"))
			}
		}
		if activityID == "" || strings.TrimSpace(route.URL) == "" {
			continue
		}
		candidate := Activity{
			ID: activityID, Title: strings.TrimSpace(route.Title), URL: route.URL,
			PhaseName: strings.TrimSpace(route.PhaseName), PhaseSection: route.PhaseSection,
			Subsection: strings.TrimSpace(route.Subsection), Technical: route.Technical,
		}
		current, exists := byID[activityID]
		if !exists || (current.Title == "" && candidate.Title != "") ||
			(!current.Technical && candidate.Technical) {
			byID[activityID] = candidate
		}
	}
	result := make([]Activity, 0, len(byID))
	for _, activity := range byID {
		result = append(result, activity)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].PhaseSection != result[j].PhaseSection {
			return result[i].PhaseSection < result[j].PhaseSection
		}
		return strings.ToLower(result[i].Title) < strings.ToLower(result[j].Title)
	})
	return result
}
