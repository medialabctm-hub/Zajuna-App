package zajuna

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/zajuna-app/core/internal/coursemaps"
)

var coursePhasePattern = regexp.MustCompile(`(?is)<li\b[^>]*\bid\s*=\s*["']section-(\d+)["'][^>]*>.*?<h3\b[^>]*class\s*=\s*["'][^"']*sectionname[^"']*["'][^>]*>(.*?)</h3\s*>`)
var coursePhaseDataPattern = regexp.MustCompile(`(?is)<div\b[^>]*\bdata-id\s*=\s*["']?(\d+)["']?[^>]*\bdata-number\s*=\s*["']?(\d+)["']?[^>]*>.*?<h3\b[^>]*class\s*=\s*["'][^"']*sectionname[^"']*["'][^>]*>(.*?)</h3\s*>`)
var sectionStartPattern = regexp.MustCompile(`(?is)<li\b[^>]*\bid\s*=\s*["']section-(\d+)["']`)
var activityBlockPattern = regexp.MustCompile(`(?is)<div\b[^>]*\bclass\s*=\s*["'][^"']*activityname[^"']*["'][^>]*>(.*?)</div\s*>`)
var assignHrefPattern = regexp.MustCompile(`(?is)href\s*=\s*["']([^"']*mod/assign/view\.php\?id=(\d+)[^"']*)["']`)
var instanceNamePattern = regexp.MustCompile(`(?is)<span\b[^>]*\bclass\s*=\s*["'][^"']*instancename[^"']*["'][^>]*>(.*?)</span\s*>`)
var subsectionPattern = regexp.MustCompile(`(?is)<li\b[^>]*\bclass\s*=\s*["'][^"']*activity[^"']*["'][^>]*\bdata-modulename\s*=\s*["']label["'][^>]*>.*?<span\b[^>]*\bclass\s*=\s*["'][^"']*instancename[^"']*["'][^>]*>(.*?)</span\s*>`)
var gradingHrefPattern = regexp.MustCompile(`(?is)href\s*=\s*["']([^"']*(?:action=grader|action=grading)[^"']*)["']`)

type detectedPhase struct {
	Section int
	Name    string
}

// extractCourseStructure ports the useful structural part of the former
// workflow: phases are detected from section markup, assignments are tied to
// their phase and subsection, and every assignment gets a grading fallback.
// The output remains generic enough to support future checklist item codes.
func extractCourseStructure(body, courseID, baseURL, sourceURL string) []coursemaps.Route {
	phases := detectCoursePhases(body)
	phaseBySection := make(map[int]string, len(phases))
	result := make([]coursemaps.Route, 0, len(phases))
	courseURL := baseURL + "/zajuna/course/view.php?id=" + url.QueryEscape(courseID)
	for _, phase := range phases {
		phaseBySection[phase.Section] = phase.Name
		phaseURL := courseURL + "&section=" + strconv.Itoa(phase.Section)
		result = append(result, coursemaps.Route{
			URL:          phaseURL,
			Kind:         "phase",
			Title:        phase.Name,
			Depth:        1,
			SourceURL:    sourceURL,
			PhaseName:    phase.Name,
			PhaseSection: phase.Section,
		})
	}

	subsections := detectSubsections(body)
	for _, match := range activityBlockPattern.FindAllStringSubmatchIndex(body, -1) {
		blockStart, blockEnd := match[0], match[1]
		block := body[match[2]:match[3]]
		hrefMatch := assignHrefPattern.FindStringSubmatch(block)
		if len(hrefMatch) != 3 {
			continue
		}
		activityURL, ok := normalizeInternalURL(sourceURL, hrefMatch[1], baseURL)
		if !ok {
			continue
		}
		activityID := hrefMatch[2]
		activityName := "Actividad sin nombre"
		if nameMatch := instanceNamePattern.FindStringSubmatch(block); len(nameMatch) == 2 {
			if cleaned := cleanText(nameMatch[1]); cleaned != "" {
				activityName = cleaned
			}
		}
		section := sectionBefore(body, blockStart)
		phaseName := phaseForSection(phases, phaseBySection, section)
		subsection := subsectionBefore(subsections, blockStart)
		technical := isTechnicalActivity(activityName)
		result = append(result, coursemaps.Route{
			URL:          activityURL,
			Kind:         "assign",
			Title:        activityName,
			Depth:        1,
			SourceURL:    sourceURL,
			PhaseName:    phaseName,
			PhaseSection: section,
			ActivityID:   activityID,
			Subsection:   subsection,
			Technical:    technical,
		})

		gradingURL := activityURL
		if match := gradingHrefPattern.FindStringSubmatch(block); len(match) == 2 {
			if normalized, valid := normalizeInternalURL(activityURL, match[1], baseURL); valid {
				gradingURL = normalized
			}
		}
		separator := "?"
		if strings.Contains(gradingURL, "?") {
			separator = "&"
		}
		if !strings.Contains(strings.ToLower(gradingURL), "action=grader") && !strings.Contains(strings.ToLower(gradingURL), "action=grading") {
			gradingURL += separator + "action=grading"
		}
		if normalized, valid := normalizeInternalURL(activityURL, gradingURL, baseURL); valid {
			result = append(result, coursemaps.Route{
				URL:          normalized,
				Kind:         "grading",
				Title:        "Calificación: " + activityName,
				Depth:        2,
				SourceURL:    activityURL,
				PhaseName:    phaseName,
				PhaseSection: section,
				ActivityID:   activityID,
				Subsection:   subsection,
				Technical:    technical,
			})
		}
		_ = blockEnd
	}
	return result
}

func detectCoursePhases(body string) []detectedPhase {
	result := make([]detectedPhase, 0, 4)
	seen := map[int]bool{}
	for _, match := range coursePhasePattern.FindAllStringSubmatch(body, -1) {
		section, _ := strconv.Atoi(match[1])
		name := strings.ToUpper(cleanText(match[2]))
		if !isAllowedPhase(name) || seen[section] {
			continue
		}
		seen[section] = true
		result = append(result, detectedPhase{Section: section, Name: name})
	}
	for _, match := range coursePhaseDataPattern.FindAllStringSubmatch(body, -1) {
		section, _ := strconv.Atoi(match[2])
		name := strings.ToUpper(cleanText(match[3]))
		if !isAllowedPhase(name) || seen[section] {
			continue
		}
		seen[section] = true
		result = append(result, detectedPhase{Section: section, Name: name})
	}
	return result
}

func isAllowedPhase(name string) bool {
	switch name {
	case "FASE 1 PLANEAR", "FASE 2 HACER", "FASE 3 VERIFICAR", "FASE 4 ACTUAR":
		return true
	default:
		return false
	}
}

type detectedSubsection struct {
	Index int
	Name  string
}

func detectSubsections(body string) []detectedSubsection {
	result := make([]detectedSubsection, 0)
	for _, match := range subsectionPattern.FindAllStringSubmatchIndex(body, -1) {
		name := cleanText(body[match[2]:match[3]])
		upper := strings.ToUpper(name)
		if name != "" && (strings.Contains(upper, "ACTIVIDAD") || strings.Contains(upper, "GUIA") || strings.Contains(upper, "GUÍA")) {
			result = append(result, detectedSubsection{Index: match[0], Name: name})
		}
	}
	return result
}

func sectionBefore(body string, index int) int {
	section := 0
	for _, match := range sectionStartPattern.FindAllStringSubmatch(body[:index], -1) {
		section, _ = strconv.Atoi(match[1])
	}
	return section
}

func subsectionBefore(subsections []detectedSubsection, index int) string {
	value := ""
	for _, subsection := range subsections {
		if subsection.Index >= index {
			break
		}
		value = subsection.Name
	}
	return value
}

func isTechnicalActivity(name string) bool {
	return coursemaps.IsTechnicalActivity(name)
}

// phaseForSection returns the phase a section belongs to. Activities of SENA
// courses live in nested subsections ("Actividad de aprendizaje GA1-…")
// numbered after their phase in document order, so a section belongs to the
// last phase whose number is not greater than its own. Sections before the
// first phase (inducción) have no phase.
func phaseForSection(phases []detectedPhase, bySection map[int]string, section int) string {
	if name, ok := bySection[section]; ok {
		return name
	}
	best := -1
	name := ""
	for _, phase := range phases {
		if phase.Section <= section && phase.Section > best {
			best, name = phase.Section, phase.Name
		}
	}
	return name
}
