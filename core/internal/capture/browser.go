package capture

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mxschmitt/playwright-go"
	"github.com/zajuna-app/core/internal/security"
)

type Runtime struct {
	Root        string
	DriverDir   string
	BrowsersDir string
}

type CaptureResult struct {
	Title           string
	FinalURL        string
	Selector        string
	SelectorMatched bool
	// RowsTotal/RowStart describe a row-batched capture: RowsTotal is the
	// number of counted rows in the container and RowStart the 1-based index
	// of the first row in the shot. Both are 0 when the capture was not batched.
	RowsTotal int
	RowStart  int
	// ContentItems counts visible activities/resources inside a captured
	// course section (-1 when the capture was not a section). The reviewer
	// uses it to flag sections that show only their title.
	ContentItems int
	// ColumnWindows is how many width-bounded windows the element needed
	// (see CaptureOptions.MaxWidth); 0 when the width was not bounded.
	ColumnWindows int
}

var ErrBlockedPage = errors.New("la página destino fue bloqueada por el sitio remoto")
var ErrLoginPage = errors.New("la página destino es la pantalla de autenticación de Zajuna")
var ErrChallengePage = errors.New("la página destino pide CAPTCHA o MFA")
var ErrSelectorNotFound = errors.New("el selector requerido no apareció en la página destino")

// ErrNoRowsInBatch means the requested row batch starts after the last row:
// the list is already fully covered by earlier slots, so there is nothing to
// capture. Callers must treat it as "slot not needed", not as a failure.
var ErrNoRowsInBatch = errors.New("no quedan filas para este lote")

// ErrContentAbsent means the expected page loaded (the list or page that
// holds the evidence is there) but nothing on it proves the item: no
// instructor reply, no conclusion, a forum without dates. It is always
// wrapped together with ErrSelectorNotFound. Only this error lets a capture
// retire the evidence a previous run left in the slot; an error page, a
// renamed activity or a navigation problem stay ordinary failures.
var ErrContentAbsent = errors.New("sin contenido en Zajuna")

// Default capture viewport, used when a target does not set its own. It is
// the size the cronogramas use, which gives course sections a readable width.
const (
	defaultViewportWidth  = 2560
	defaultViewportHeight = 1200
)

// ErrForumAccessDenied means Moodle refused the forum to this account and
// redirected away from it: the route map points to a forum the instructor
// cannot see, so no row of it can ever be evidence.
var ErrForumAccessDenied = errors.New("el foro no está disponible para esta cuenta de Zajuna")

const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

type CaptureOptions struct {
	Selector        string
	Selectors       []string
	RevealSelectors []string
	HideSelectors   []string
	ViewportWidth   int
	ViewportHeight  int
	FullPage        bool
	LabelHint       string
	OwnerName       string
	Timeout         time.Duration
	RequireSelector bool
	OwnerOnly       bool
	// RowSelector/RowsPerShot/RowBatch crop list and table evidence to one
	// batch of rows (see checklist.CaptureTarget). RowBatch is zero-based.
	RowSelector string
	RowsPerShot int
	RowBatch    int
	// OptionalSlot marks a slot that may legitimately not exist (e.g. the
	// 5th "Grabaciones" section of a course with 4 phases): when no selector
	// matches, the capture returns ErrNoRowsInBatch ("not needed") instead of
	// a failure.
	OptionalSlot bool
	// RowMatch keeps only rows whose text contains one of these terms.
	RowMatch []string
	// RowRequireReply keeps only discussions with one or more replies whose
	// last message is by OwnerName (see checklist.CaptureTarget).
	RowRequireReply bool
	// AbsenceSelector identifies the right page when no selector matched:
	// if it is present, the failure is ErrContentAbsent instead of a plain
	// ErrSelectorNotFound (e.g. a forum page that shows no dates).
	AbsenceSelector string
	// CourseLayout puts the course sections in a known state before the
	// capture (CourseLayoutMenu, CourseLayoutFirstSection or "" to keep them).
	CourseLayout string
	// MaxWidth bounds element shots: a wider element is split into windows
	// of whole columns and ColumnBatch (zero-based) picks one. A window past
	// the last column returns ErrNoRowsInBatch.
	MaxWidth    int
	ColumnBatch int
	// ExpandEmbeddedSheets grows Google Sheets iframes to their content so
	// the shot shows the whole sheet instead of its scroll viewport.
	ExpandEmbeddedSheets bool
}

// BrowserCookie is the cookie shape needed to bridge an authenticated HTTP
// session into an isolated Chromium context. It is never persisted.
type BrowserCookie struct {
	Name     string
	Value    string
	URL      string
	Domain   string
	Path     string
	Secure   bool
	HttpOnly bool
	SameSite string
	Expires  *time.Time
}

func Resolve(executablePath string) Runtime {
	if configured := os.Getenv("ZAJUNA_PLAYWRIGHT_DIR"); configured != "" {
		return newRuntime(configured)
	}
	if executablePath == "" {
		executablePath, _ = os.Executable()
	}
	return newRuntime(filepath.Join(filepath.Dir(executablePath), "playwright"))
}

func newRuntime(root string) Runtime {
	return Runtime{
		Root:        root,
		DriverDir:   filepath.Join(root, "driver"),
		BrowsersDir: filepath.Join(root, "browsers"),
	}
}

func (r Runtime) Installed() bool {
	if r.Root == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(r.DriverDir, "package", "cli.js")); err != nil {
		return false
	}
	entries, err := os.ReadDir(r.BrowsersDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return true
		}
	}
	return false
}

func (r Runtime) Start() (*playwright.Playwright, error) {
	if !r.Installed() {
		return nil, fmt.Errorf("runtime Chromium no instalado en %s; ejecuta npm run browser:install", r.Root)
	}
	var instance *playwright.Playwright
	err := withPlaywrightEnv(r, func() error {
		var runErr error
		instance, runErr = playwright.Run(&playwright.RunOptions{
			DriverDirectory:     r.DriverDir,
			Browsers:            []string{"chromium"},
			SkipInstallBrowsers: true,
			Verbose:             false,
		})
		if runErr != nil {
			return fmt.Errorf("iniciar Playwright local: %w", runErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return instance, nil
}

// playwrightEnvMu serializes the process-wide PLAYWRIGHT_* variables with
// the driver launch that inherits them. Parallel checklist sessions start
// drivers concurrently; without it one launch could inherit another
// runtime's paths between Setenv and exec.
var playwrightEnvMu sync.Mutex

func withPlaywrightEnv(r Runtime, run func() error) error {
	playwrightEnvMu.Lock()
	defer playwrightEnvMu.Unlock()
	for name, value := range map[string]string{"PLAYWRIGHT_DRIVER_PATH": r.DriverDir, "PLAYWRIGHT_BROWSERS_PATH": r.BrowsersDir} {
		if os.Getenv(name) == value {
			continue
		}
		if err := os.Setenv(name, value); err != nil {
			return fmt.Errorf("configurar %s de Playwright: %w", name, err)
		}
	}
	return run()
}

func (r Runtime) CaptureURL(ctx context.Context, targetURL, outputPath string) error {
	_, err := r.CaptureURLWithMetadata(ctx, targetURL, outputPath)
	return err
}

func (r Runtime) CaptureURLWithMetadata(ctx context.Context, targetURL, outputPath string) (CaptureResult, error) {
	return r.CaptureURLWithMetadataAndCookies(ctx, targetURL, outputPath, nil)
}

func (r Runtime) CaptureURLWithMetadataAndCookies(ctx context.Context, targetURL, outputPath string, cookies []BrowserCookie) (CaptureResult, error) {
	return r.CaptureURLWithMetadataAndCookiesAndOptions(ctx, targetURL, outputPath, cookies, CaptureOptions{})
}

func (r Runtime) CaptureURLWithMetadataAndCookiesAndOptions(ctx context.Context, targetURL, outputPath string, cookies []BrowserCookie, options CaptureOptions) (CaptureResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	parsed, err := url.Parse(targetURL)
	if err != nil || parsed.Host == "" {
		return CaptureResult{}, errors.New("la URL de captura debe ser http o https")
	}
	if err := ValidateCaptureNavigationURL(targetURL, parsed); err != nil {
		return CaptureResult{}, err
	}
	if outputPath == "" {
		return CaptureResult{}, errors.New("la ruta de salida de captura es obligatoria")
	}
	if err := ctx.Err(); err != nil {
		return CaptureResult{}, err
	}
	absoluteOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return CaptureResult{}, fmt.Errorf("resolver salida de captura: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absoluteOutput), 0o700); err != nil {
		return CaptureResult{}, fmt.Errorf("crear carpeta de captura: %w", err)
	}
	pw, err := r.Start()
	if err != nil {
		return CaptureResult{}, err
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(true)})
	if err != nil {
		return CaptureResult{}, fmt.Errorf("lanzar Chromium local: %w", err)
	}
	defer browser.Close()
	browserContext, err := browser.NewContext(playwright.BrowserNewContextOptions{
		UserAgent: playwright.String(browserUserAgent),
		Locale:    playwright.String("es-CO"),
	})
	if err != nil {
		return CaptureResult{}, fmt.Errorf("crear contexto Chromium: %w", err)
	}
	defer browserContext.Close()
	if err := installNetworkPolicy(browserContext, parsed); err != nil {
		return CaptureResult{}, fmt.Errorf("configurar política de red de Chromium: %w", err)
	}
	if len(cookies) > 0 {
		browserCookies := make([]playwright.OptionalCookie, 0, len(cookies))
		for _, cookie := range cookies {
			item, ok := cookie.playwrightCookie()
			if !ok {
				continue
			}
			browserCookies = append(browserCookies, item)
		}
		if len(browserCookies) > 0 {
			if err := browserContext.AddCookies(browserCookies); err != nil {
				return CaptureResult{}, fmt.Errorf("cargar sesión autenticada en Chromium: %w", err)
			}
		}
	}
	page, err := browserContext.NewPage()
	if err != nil {
		return CaptureResult{}, fmt.Errorf("crear página Chromium: %w", err)
	}
	return capturePage(ctx, page, targetURL, absoluteOutput, options)
}

func capturePage(ctx context.Context, page playwright.Page, targetURL, absoluteOutput string, options CaptureOptions) (CaptureResult, error) {
	parsedTarget, err := url.Parse(targetURL)
	if err != nil || parsedTarget.Host == "" {
		return CaptureResult{}, errors.New("la URL de captura debe ser http o https")
	}
	if err := ValidateCaptureNavigationURL(targetURL, parsedTarget); err != nil {
		return CaptureResult{}, err
	}
	// Always set the viewport: pooled sessions reuse the page, so a target
	// without its own size inherited whatever the previous capture set (the
	// same course section came out 2000 or 976 px wide between runs).
	width, height := options.ViewportWidth, options.ViewportHeight
	if width <= 0 || height <= 0 {
		width, height = defaultViewportWidth, defaultViewportHeight
	}
	if err := page.SetViewportSize(width, height); err != nil {
		return CaptureResult{}, fmt.Errorf("configurar viewport de captura: %w", err)
	}
	if _, err := page.Goto(targetURL); err != nil {
		return CaptureResult{}, fmt.Errorf("navegar para captura: %w", err)
	}
	_ = page.WaitForLoadState()
	title, _ := page.Title()
	finalURL := page.URL()
	if err := ValidateCaptureNavigationURL(finalURL, parsedTarget); err != nil {
		return CaptureResult{}, fmt.Errorf("captura bloqueada: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return CaptureResult{}, err
	}
	body, _ := page.Locator("body").InnerText()
	if isZajunaLoginPage(finalURL, title, body) {
		return CaptureResult{}, fmt.Errorf("%w: URL final %s", ErrLoginPage, finalURL)
	}
	if reason := pageChallengeReason(page, finalURL, title, body); reason != "" {
		return CaptureResult{}, fmt.Errorf("%w: %s en URL final %s", ErrChallengePage, reason, finalURL)
	}
	if blocked, blockedErr := isBlockedPage(page, title); blocked {
		return CaptureResult{}, blockedErr
	}
	// Checked before the notifications are hidden: the permission notice is
	// the proof that the forum is unavailable.
	if forumAccessDenied(parsedTarget, finalURL, body) {
		return CaptureResult{}, fmt.Errorf("%w: Zajuna redirigió a %s", ErrForumAccessDenied, security.RedactURL(finalURL))
	}
	// Order matters: expandEmbeddedSheets grows the frame to the sheet
	// (mostly its width); prepareEmbeddedSheets then opens the phase tab and
	// sets the final height measured inside the nested widget frame, capped so
	// the evidence review does not flag it as too tall.
	if options.ExpandEmbeddedSheets {
		expandEmbeddedSheets(page)
	}
	prepareCourseMenu(page, options)
	if err := ensureStillOn(page, finalURL); err != nil {
		return CaptureResult{}, err
	}
	prepareEmbeddedSheets(page, title)
	prepareHiddenEvidenceRegions(page, options.HideSelectors)
	// Moodle flash notifications (e.g. "No dispone de permiso para ver los
	// debates de este foro") must never leak into evidence.
	prepareHiddenEvidenceRegions(page, moodleNotificationSelectors)
	result := CaptureResult{Title: title, FinalURL: finalURL, ContentItems: -1}
	batching := strings.TrimSpace(options.RowSelector) != "" && options.RowsPerShot > 0
	selectors := make([]string, 0, len(options.Selectors)+1)
	addSelector := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range selectors {
			if existing == value {
				return
			}
		}
		selectors = append(selectors, value)
	}
	addSelector(options.Selector)
	for _, selector := range options.Selectors {
		addSelector(selector)
	}
	matchedCandidates := 0
	emptyPrimaryContainer := false
	lastScreenshotError := ""
	selectorDiagnostics := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		locator := page.Locator(selector)
		rawCount, _ := locator.Count()
		if hint := strings.TrimSpace(options.LabelHint); hint != "" {
			filtered := locator.Filter(playwright.LocatorFilterOptions{HasText: hint})
			count, countErr := filtered.Count()
			if countErr != nil || count == 0 {
				selectorDiagnostics = append(selectorDiagnostics, fmt.Sprintf("%s raw=%d hint=0", selector, rawCount))
				// Never use the first generic node when a semantic hint does not
				// match; that would produce unrelated evidence.
				continue
			}
			locator = filtered
		}
		if options.OwnerOnly {
			ownerName := strings.TrimSpace(options.OwnerName)
			if ownerName == "" {
				continue
			}
			// When batching rows, the owner filter applies to rows inside the
			// container (see captureRowBatch), not to the container itself.
			if !batching {
				filtered := locator.Filter(playwright.LocatorFilterOptions{HasText: ownerName})
				count, countErr := filtered.Count()
				if countErr != nil || count == 0 {
					selectorDiagnostics = append(selectorDiagnostics, fmt.Sprintf("%s raw=%d owner=0", selector, rawCount))
					continue
				}
				locator = filtered
			}
		}
		if options.OwnerOnly && !batching {
			selectorDiagnostics = append(selectorDiagnostics, fmt.Sprintf("%s raw=%d owner=%d", selector, rawCount, func() int { value, _ := locator.Count(); return value }()))
		}
		if count, countErr := locator.Count(); countErr == nil && count > 0 {
			matchedCandidates += count
			timeout := 5000.0
			if options.Timeout > 0 {
				timeout = float64(options.Timeout.Milliseconds())
			}
			if batching {
				batch, batchErr := captureRowBatch(page, locator.First(), absoluteOutput, timeout, options)
				if errors.Is(batchErr, ErrNoRowsInBatch) && batch.total == 0 {
					// This candidate has no rows (e.g. a per-row fallback such as
					// `.discussion`). Only a container that actually counted rows
					// may declare the batch empty; keep looking.
					selectorDiagnostics = append(selectorDiagnostics, fmt.Sprintf("%s raw=%d rows=0", selector, rawCount))
					if selector == strings.TrimSpace(options.Selector) {
						emptyPrimaryContainer = true
					}
					continue
				}
				if batch.handled {
					selectorDiagnostics = append(selectorDiagnostics, fmt.Sprintf("%s raw=%d rows=%d", selector, rawCount, batch.total))
					if batchErr == nil {
						result.Selector = selector
						result.SelectorMatched = true
						result.RowsTotal = batch.total
						result.RowStart = batch.start + 1
						result.ColumnWindows = batch.columnWindows
						return result, nil
					}
					// Rows were counted here, so this container is the list:
					// "no more rows" is definitive, and a screenshot error must
					// surface as a failure instead of becoming an empty batch on
					// a fallback selector (which would prune good evidence).
					return CaptureResult{}, batchErr
				}
				if batchErr != nil {
					lastScreenshotError = batchErr.Error()
					continue
				}
				// No rows at all on the first batch: fall back to the legacy
				// container capture, including the container-level owner filter.
				selectorDiagnostics = append(selectorDiagnostics, fmt.Sprintf("%s raw=%d rows=0", selector, rawCount))
				if selector == strings.TrimSpace(options.Selector) {
					emptyPrimaryContainer = true
				}
				if len(options.RowMatch) > 0 || options.RowRequireReply {
					// A topic or reply filter found no rows: the legacy
					// whole-container capture would show rows that do not
					// prove this item.
					continue
				}
				if options.OwnerOnly {
					filtered := locator.Filter(playwright.LocatorFilterOptions{HasText: strings.TrimSpace(options.OwnerName)})
					if ownerCount, ownerErr := filtered.Count(); ownerErr != nil || ownerCount == 0 {
						selectorDiagnostics = append(selectorDiagnostics, fmt.Sprintf("%s raw=%d owner=0", selector, rawCount))
						continue
					}
					locator = filtered
				}
			}
			var captureErr error
			if options.FullPage {
				neutralizeFloatingElements(page)
				_, captureErr = page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(absoluteOutput), FullPage: playwright.Bool(true), Timeout: playwright.Float(timeout)})
			} else {
				expandCourseSection(locator.First(), timeout)
				if emptyCourseSection(locator.First(), timeout) {
					// Only the section title: the section exists but Zajuna has
					// nothing in it. A header-only shot is not evidence.
					return CaptureResult{}, fmt.Errorf("%w (%w): la sección no tiene actividades ni archivos", ErrSelectorNotFound, ErrContentAbsent)
				}
				neutralizeFloatingElements(page)
				result.ColumnWindows, captureErr = screenshotElement(page, locator.First(), absoluteOutput, timeout, options)
				if errors.Is(captureErr, ErrNoRowsInBatch) {
					return CaptureResult{}, captureErr
				}
			}
			if captureErr == nil {
				result.Selector = selector
				result.SelectorMatched = true
				result.ContentItems = sectionContentItems(locator.First(), timeout)
				return result, nil
			} else {
				lastScreenshotError = captureErr.Error()
			}
		}
	}
	if batching && options.RowBatch == 0 && !options.OwnerOnly && len(options.RowMatch) > 0 && emptyPrimaryContainer {
		// The table exists but no row shows the required state (e.g. no
		// graded submission): a real absence, not a capture failure.
		return CaptureResult{}, fmt.Errorf("%w (%w): la tabla no tiene filas «%s»", ErrSelectorNotFound, ErrContentAbsent, strings.Join(options.RowMatch, "» o «"))
	}
	if batching && options.RowBatch == 0 && options.OwnerOnly && emptyPrimaryContainer {
		// The list exists but none of its rows is by the instructor: a real
		// absence of evidence, reported in plain words.
		if options.RowRequireReply {
			return CaptureResult{}, fmt.Errorf("%w (%w): la lista no tiene respuestas del instructor autenticado (debates con réplicas cuyo último mensaje sea suyo)", ErrSelectorNotFound, ErrContentAbsent)
		}
		if len(options.RowMatch) > 0 {
			return CaptureResult{}, fmt.Errorf("%w (%w): la lista no tiene publicaciones del instructor autenticado sobre «%s»", ErrSelectorNotFound, ErrContentAbsent, strings.Join(options.RowMatch, "» o «"))
		}
		return CaptureResult{}, fmt.Errorf("%w (%w): la lista no tiene publicaciones del instructor autenticado", ErrSelectorNotFound, ErrContentAbsent)
	}
	if batching && options.RowBatch > 0 && emptyPrimaryContainer {
		// The real list container rendered but holds no (owner) rows: later
		// batches are simply not needed. A missing container stays a failure.
		return CaptureResult{}, fmt.Errorf("%w: la lista no tiene filas", ErrNoRowsInBatch)
	}
	if options.OptionalSlot && matchedCandidates == 0 {
		return CaptureResult{}, fmt.Errorf("%w: el espacio opcional no existe en esta página", ErrNoRowsInBatch)
	}
	if options.RequireSelector && matchedCandidates == 0 && strings.TrimSpace(options.AbsenceSelector) != "" {
		if count, countErr := page.Locator(strings.TrimSpace(options.AbsenceSelector)).Count(); countErr == nil && count > 0 {
			return CaptureResult{}, fmt.Errorf("%w (%w): la página cargó pero no muestra %s", ErrSelectorNotFound, ErrContentAbsent, strings.TrimSpace(options.Selector))
		}
	}
	if options.RequireSelector {
		diagnostics := fmt.Sprintf("candidatos=%d", matchedCandidates)
		if len(selectorDiagnostics) > 0 {
			diagnostics += ", selectores=" + strings.Join(selectorDiagnostics, " | ")
		}
		if options.OwnerOnly {
			counts := make([]string, 0, 8)
			for _, diagnosticSelector := range []string{"#region-main", "#page-mod-forum-view", "#region-main table", ".forumheaderlist", ".discussion", ".forumpost", ".forum-post", "article"} {
				count, _ := page.Locator(diagnosticSelector).Count()
				counts = append(counts, fmt.Sprintf("%s=%d", diagnosticSelector, count))
			}
			diagnostics += ", DOM=" + strings.Join(counts, ",")
		}
		if lastScreenshotError != "" {
			diagnostics += fmt.Sprintf(", error de captura: %s", lastScreenshotError)
		}
		return CaptureResult{}, fmt.Errorf("%w: %s (%s)", ErrSelectorNotFound, strings.TrimSpace(options.Selector), diagnostics)
	}
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(absoluteOutput), FullPage: playwright.Bool(true)}); err != nil {
		return CaptureResult{}, fmt.Errorf("guardar captura: %w", err)
	}
	if len(selectors) > 0 {
		result.Selector = selectors[0]
	}
	return result, nil
}

func prepareHiddenEvidenceRegions(page playwright.Page, selectors []string) {
	for _, selector := range selectors {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			continue
		}
		_, _ = page.Evaluate(`(selector) => {
			for (const node of document.querySelectorAll(selector)) {
				node.setAttribute('data-zajuna-hidden-evidence', 'true');
				node.style.display = 'none';
			}
			return true;
		}`, selector)
	}
}

// expandCourseSectionScript opens a collapsed Moodle 4 course section (and
// its nested subsections) in place. Section content is already in the DOM as
// `.content.collapse` with display:none, so a capture of a collapsed section
// only showed its title bar. Nothing is clicked and nothing navigates.
const expandCourseSectionScript = `(section) => {
	if (!section.matches('li.section, [data-for="section"]')) {
		return false;
	}
	// Only the section's own panel: nested subsections stay collapsed so the
	// evidence shows the organization (their titles) instead of a page tens
	// of thousands of pixels tall. Items that need a subsection target it.
	const own = [...section.querySelectorAll(':scope > .content.collapse, :scope > .course-content-item-content.collapse')];
	// One level of child subsections is opened too, so a section shows its
	// actas, recordings or months instead of only their collapsed titles;
	// deeper levels stay closed to keep the image readable.
	const children = own.flatMap((panel) => [...panel.querySelectorAll('li.section')]
		.filter((child) => child.parentElement.closest('li.section') === section)
		.flatMap((child) => [...child.querySelectorAll(':scope > .content.collapse, :scope > .course-content-item-content.collapse')]));
	const panels = [...own, ...children];
	// A subsection stays invisible (and its screenshot times out) while any
	// ancestor section is collapsed: open the whole chain up to the course.
	for (let node = section.parentElement; node; node = node.parentElement) {
		if (node.classList && node.classList.contains('collapse')) {
			panels.push(node);
		}
	}
	for (const panel of panels) {
		panel.classList.add('show');
		panel.style.display = 'block';
		panel.style.height = 'auto';
	}
	for (const toggle of [section, ...children.map((panel) => panel.parentElement)].flatMap((node) => [...node.querySelectorAll(':scope > .course-section-header [data-toggle="collapse"][aria-expanded="false"]')])) {
		toggle.classList.remove('collapsed');
		toggle.setAttribute('aria-expanded', 'true');
	}
	return panels.length > 0;
}`

func expandCourseSection(section playwright.Locator, timeout float64) {
	_, _ = section.Evaluate(expandCourseSectionScript, nil, playwright.LocatorEvaluateOptions{Timeout: playwright.Float(timeout)})
}

// moodleNotificationSelectors are flash/alert regions Moodle injects after a
// redirect (permission errors, session notices). They are never evidence.
var moodleNotificationSelectors = []string{
	"#user-notifications",
	"#region-main > .alert-dismissible",
	"#page-content .alert-block.alert-dismissible",
}

// rowBatchWindow returns the half-open row range [start, end) covered by the
// zero-based batch when total rows are split into shots of perShot rows. ok
// is false when the batch starts at or after the last row.
func rowBatchWindow(total, perShot, batch int) (start, end int, ok bool) {
	if total <= 0 || perShot <= 0 || batch < 0 {
		return 0, 0, false
	}
	start = batch * perShot
	if start >= total {
		return 0, 0, false
	}
	end = start + perShot
	if end > total {
		end = total
	}
	return start, end, true
}

// rowBatchHideScript counts rows inside the container and, when the batch
// window is not empty, hides every row outside it (and every non-owner row
// when ownerOnly). Counting and hiding happen in one evaluation so both use
// the same DOM snapshot. Returns {total}.
const rowBatchHideScript = `(container, args) => {
	const normalize = (value) => (value || '')
		.normalize('NFD')
		.replace(/[̀-ͯ]/g, '')
		.replace(/\s+/g, ' ')
		.trim()
		.toLowerCase();
	const owner = normalize(args.owner);
	const topics = (args.rowMatch || []).map(normalize).filter(Boolean);
	const all = Array.from(container.querySelectorAll(args.rowSelector));
	// Column positions come from the header text ("Réplicas", "Último
	// mensaje"), which is stable across Moodle themes; class names are the
	// fallback.
	const headers = Array.from(container.querySelectorAll('thead th, thead td')).map((cell) => normalize(cell.textContent));
	const columnIndex = (pattern) => headers.findIndex((text) => pattern.test(text));
	const repliesColumn = columnIndex(/^(replicas|respuestas|replies)\b/);
	const lastPostColumn = columnIndex(/^(ultimo mensaje|last post)/);
	const cellAt = (row, index, fallback) => {
		const cells = Array.from(row.querySelectorAll(':scope > td, :scope > th'));
		if (index >= 0 && index < cells.length) return cells[index];
		return row.querySelector(fallback);
	};
	const counted = [];
	const hide = [];
	for (const row of all) {
		// Prefer the author cell: a discussion started by someone else may
		// still show the instructor as its last poster.
		const author = row.querySelector('td.author, .author, [data-region="author-name"]');
		let ownerText = author ? author.textContent : row.textContent;
		let replyOK = true;
		if (args.requireReply) {
			// "Responde": the instructor wrote the last message of a
			// discussion that has replies. Without both columns the row
			// cannot prove it, so it is not evidence.
			const repliesCell = cellAt(row, repliesColumn, 'td.replies, .replies');
			const lastPostCell = cellAt(row, lastPostColumn, 'td.lastpost, .lastpost');
			const replies = repliesCell ? parseInt((repliesCell.textContent.match(/\d+/) || ['0'])[0], 10) : 0;
			replyOK = replies > 0 && Boolean(lastPostCell);
			ownerText = lastPostCell ? lastPostCell.textContent : '';
		}
		const topicOK = topics.length === 0 || topics.some((topic) => normalize(row.textContent).includes(topic));
		if (!topicOK || !replyOK || (args.ownerOnly && (owner === '' || !normalize(ownerText).includes(owner)))) {
			hide.push(row);
		} else {
			counted.push(row);
		}
	}
	const total = counted.length;
	const start = args.batch * args.perShot;
	if (total === 0 || start >= total) {
		return { total };
	}
	const end = Math.min(start + args.perShot, total);
	counted.forEach((row, index) => {
		if (index < start || index >= end) {
			hide.push(row);
		}
	});
	for (const row of hide) {
		if (row.hasAttribute('data-zajuna-row-batch-hidden')) {
			continue;
		}
		row.setAttribute('data-zajuna-row-batch-hidden', 'true');
		row.setAttribute('data-zajuna-row-batch-display', row.style.display || '');
		row.style.display = 'none';
	}
	return { total };
}`

// rowBatchRestoreScript undoes rowBatchHideScript so later work on the same
// page sees the original rows.
const rowBatchRestoreScript = `() => {
	for (const row of document.querySelectorAll('[data-zajuna-row-batch-hidden]')) {
		row.style.display = row.getAttribute('data-zajuna-row-batch-display') || '';
		row.removeAttribute('data-zajuna-row-batch-hidden');
		row.removeAttribute('data-zajuna-row-batch-display');
	}
	return true;
}`

type rowBatchOutcome struct {
	// handled is true when rows were counted and the batch window was
	// captured (or its screenshot attempted). false with a nil error means
	// there were no rows on batch 0 and the caller must fall back.
	handled       bool
	total         int
	start         int
	columnWindows int
}

// captureRowBatch screenshots only the requested batch of rows inside the
// container. It returns ErrNoRowsInBatch (without writing a file) when the
// batch starts after the last counted row.
func captureRowBatch(page playwright.Page, container playwright.Locator, absoluteOutput string, timeout float64, options CaptureOptions) (rowBatchOutcome, error) {
	raw, err := container.Evaluate(rowBatchHideScript, map[string]any{
		"rowSelector":  strings.TrimSpace(options.RowSelector),
		"owner":        strings.TrimSpace(options.OwnerName),
		"ownerOnly":    options.OwnerOnly,
		"perShot":      options.RowsPerShot,
		"batch":        options.RowBatch,
		"rowMatch":     options.RowMatch,
		"requireReply": options.RowRequireReply,
	}, playwright.LocatorEvaluateOptions{Timeout: playwright.Float(timeout)})
	restore := func() { _, _ = page.Evaluate(rowBatchRestoreScript) }
	if err != nil {
		restore()
		return rowBatchOutcome{}, fmt.Errorf("contar filas del lote: %w", err)
	}
	total := evaluatedInt(raw, "total")
	start, _, ok := rowBatchWindow(total, options.RowsPerShot, options.RowBatch)
	if total == 0 && options.RowBatch == 0 {
		restore()
		return rowBatchOutcome{}, nil
	}
	if total == 0 {
		restore()
		return rowBatchOutcome{}, fmt.Errorf("%w: el contenedor no tiene filas", ErrNoRowsInBatch)
	}
	if !ok {
		restore()
		return rowBatchOutcome{handled: true, total: total}, fmt.Errorf("%w: lote %d con %d filas por captura, %d filas en total", ErrNoRowsInBatch, options.RowBatch+1, options.RowsPerShot, total)
	}
	neutralizeFloatingElements(page)
	columnWindows, captureErr := screenshotElement(page, container, absoluteOutput, timeout, options)
	restore()
	return rowBatchOutcome{handled: true, total: total, start: start, columnWindows: columnWindows}, captureErr
}

// columnGeometryScript lifts overflow clipping on the element's ancestors
// (Moodle wraps the grader in a scrolling div, so the hidden part would not
// render) and returns the element box plus the right edge of every cell of
// its widest visible row, all in page coordinates.
const columnGeometryScript = `(element) => {
	for (let node = element.parentElement; node; node = node.parentElement) {
		const style = getComputedStyle(node);
		if (style.overflowX !== 'visible' || style.overflowY !== 'visible') {
			node.style.setProperty('overflow', 'visible', 'important');
		}
		if (style.maxWidth !== 'none') {
			node.style.setProperty('max-width', 'none', 'important');
		}
	}
	let cells = [];
	for (const row of element.querySelectorAll('tr')) {
		if (row.offsetParent === null) {
			continue;
		}
		const rowCells = row.querySelectorAll(':scope > th, :scope > td');
		if (rowCells.length > cells.length) {
			cells = Array.from(rowCells);
		}
	}
	const rect = element.getBoundingClientRect();
	return {
		left: rect.left + window.scrollX,
		top: rect.top + window.scrollY,
		width: rect.width,
		height: rect.height,
		edges: cells.map((cell) => cell.getBoundingClientRect().right + window.scrollX),
	};
}`

type columnSpan struct {
	start, end float64
}

// columnWindows splits [left, left+width) into windows no wider than
// maxWidth that end on a column edge, so no column is cut in half. A single
// column wider than the limit is cut at the limit.
func columnWindows(left, width float64, edges []float64, maxWidth float64) []columnSpan {
	if width <= 0 || maxWidth <= 0 {
		return nil
	}
	right := left + width
	if width <= maxWidth {
		return []columnSpan{{left, right}}
	}
	sorted := append([]float64(nil), edges...)
	sort.Float64s(sorted)
	windows := make([]columnSpan, 0, int(width/maxWidth)+1)
	for start := left; start < right-0.5; {
		limit := start + maxWidth
		end := 0.0
		if right <= limit {
			end = right
		} else {
			for _, edge := range sorted {
				if edge > start+0.5 && edge <= limit {
					end = edge
				}
			}
			if end == 0 {
				end = limit
			}
		}
		windows = append(windows, columnSpan{start, end})
		start = end
	}
	return windows
}

// screenshotElement writes the element shot. With MaxWidth set, a wider
// element is clipped to window ColumnBatch; the returned count is the
// number of windows the element needs (0 when the width is not bounded).
func screenshotElement(page playwright.Page, element playwright.Locator, absoluteOutput string, timeout float64, options CaptureOptions) (int, error) {
	elementShot := func() error {
		_, err := element.Screenshot(playwright.LocatorScreenshotOptions{Path: playwright.String(absoluteOutput), Timeout: playwright.Float(timeout)})
		return err
	}
	if options.MaxWidth <= 0 {
		return 0, elementShot()
	}
	raw, err := element.Evaluate(columnGeometryScript, nil, playwright.LocatorEvaluateOptions{Timeout: playwright.Float(timeout)})
	if err != nil {
		return 0, fmt.Errorf("medir columnas de la captura: %w", err)
	}
	values, _ := raw.(map[string]any)
	edges := make([]float64, 0)
	if list, ok := values["edges"].([]any); ok {
		for _, value := range list {
			if edge, ok := evaluatedFloat(value); ok {
				edges = append(edges, edge)
			}
		}
	}
	left, _ := evaluatedFloat(values["left"])
	top, _ := evaluatedFloat(values["top"])
	width, _ := evaluatedFloat(values["width"])
	height, _ := evaluatedFloat(values["height"])
	windows := columnWindows(left, width, edges, float64(options.MaxWidth))
	if options.ColumnBatch < 0 || (options.ColumnBatch > 0 && options.ColumnBatch >= len(windows)) {
		return len(windows), fmt.Errorf("%w: ventana de columnas %d, el elemento solo necesita %d", ErrNoRowsInBatch, options.ColumnBatch+1, len(windows))
	}
	if len(windows) <= 1 || height <= 0 {
		return len(windows), elementShot()
	}
	window := windows[options.ColumnBatch]
	_, err = page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String(absoluteOutput),
		FullPage: playwright.Bool(true),
		Clip:     &playwright.Rect{X: window.start, Y: top, Width: window.end - window.start, Height: height},
		Timeout:  playwright.Float(timeout),
	})
	return len(windows), err
}

func evaluatedFloat(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	}
	return 0, false
}

// forumAccessDenied tells whether a forum target was refused: Moodle
// redirects off the forum (to the course page) and shows its
// "noviewdiscussionspermission" notice.
func forumAccessDenied(target *url.URL, finalURL, body string) bool {
	if target == nil || !strings.HasSuffix(strings.ToLower(target.Path), "/mod/forum/view.php") {
		return false
	}
	if final, err := url.Parse(finalURL); err == nil && final.Path != "" && !strings.Contains(strings.ToLower(final.Path), "/mod/forum/") {
		return true
	}
	lower := strings.ToLower(body)
	for _, marker := range []string{"no dispone de permiso para ver los debates", "no tiene permiso para ver los debates", "permission to view discussions"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// Embedded Google Sheets are bounded so a huge sheet cannot produce an
// image Chromium refuses to encode.
const (
	maxEmbeddedSheets      = 4
	maxEmbeddedSheetWidth  = 2560
	maxEmbeddedSheetHeight = 16000
)

// sheetContentSizeScript runs inside a Google Sheets frame: it lifts the
// published sheet's own scroll containers and measures the full grid.
const sheetContentSizeScript = `() => {
	const style = document.createElement('style');
	style.setAttribute('data-zajuna-sheet-expand', 'true');
	style.textContent = 'html,body,#sheets-viewport,#sheets-viewport > div,.grid-container{overflow:visible !important;height:auto !important;max-height:none !important;width:auto !important;max-width:none !important}';
	(document.head || document.documentElement).appendChild(style);
	const root = document.documentElement;
	let width = Math.max(root.scrollWidth, document.body ? document.body.scrollWidth : 0);
	let height = Math.max(root.scrollHeight, document.body ? document.body.scrollHeight : 0);
	for (const table of document.querySelectorAll('table')) {
		const rect = table.getBoundingClientRect();
		width = Math.max(width, Math.ceil(rect.right + window.scrollX));
		height = Math.max(height, Math.ceil(rect.bottom + window.scrollY));
	}
	return { width, height };
}`

// resizeSheetFrameScript grows the iframe (never shrinks it) and lifts
// fixed heights/clipping on its ancestors; position:static undoes
// aspect-ratio wrappers that pin the iframe absolutely.
const resizeSheetFrameScript = `(frame, size) => {
	const rect = frame.getBoundingClientRect();
	const width = Math.max(Math.ceil(rect.width), size.width);
	const height = Math.max(Math.ceil(rect.height), size.height);
	frame.style.setProperty('position', 'static', 'important');
	frame.style.setProperty('width', width + 'px', 'important');
	frame.style.setProperty('height', height + 'px', 'important');
	frame.style.setProperty('max-width', 'none', 'important');
	frame.style.setProperty('max-height', 'none', 'important');
	for (let node = frame.parentElement; node && node !== document.body; node = node.parentElement) {
		const style = getComputedStyle(node);
		if (style.overflowX !== 'visible' || style.overflowY !== 'visible') {
			node.style.setProperty('overflow', 'visible', 'important');
		}
		if (style.maxHeight !== 'none') {
			node.style.setProperty('max-height', 'none', 'important');
		}
		if (node.style.height) {
			node.style.setProperty('height', 'auto', 'important');
		}
	}
	return { width, height };
}`

// expandEmbeddedSheets is best effort: nothing is clicked or navigated,
// only frames still on docs.google.com are touched, and any failure keeps
// the previous (viewport-only) capture.
func expandEmbeddedSheets(page playwright.Page) {
	frames := page.Locator(`iframe[src*="docs.google.com/spreadsheets"]`)
	count, err := frames.Count()
	if err != nil || count == 0 {
		return
	}
	resized := false
	for index := 0; index < count && index < maxEmbeddedSheets; index++ {
		element := frames.Nth(index)
		handle, err := element.ElementHandle(playwright.LocatorElementHandleOptions{Timeout: playwright.Float(3000)})
		if err != nil {
			continue
		}
		frame, err := handle.ContentFrame()
		if err != nil || frame == nil || !embeddedSheetFrameURL(frame.URL()) {
			continue
		}
		_ = frame.WaitForLoadState(playwright.FrameWaitForLoadStateOptions{State: playwright.LoadStateLoad, Timeout: playwright.Float(8000)})
		raw, err := frame.Evaluate(sheetContentSizeScript)
		if err != nil {
			continue
		}
		width, height := boundedSheetSize(evaluatedInt(raw, "width"), evaluatedInt(raw, "height"))
		if width == 0 || height == 0 {
			continue
		}
		if _, err := element.Evaluate(resizeSheetFrameScript, map[string]any{"width": width, "height": height}, playwright.LocatorEvaluateOptions{Timeout: playwright.Float(3000)}); err == nil {
			resized = true
		}
	}
	if resized {
		page.WaitForTimeout(500)
	}
}

func embeddedSheetFrameURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && strings.EqualFold(parsed.Hostname(), "docs.google.com") && strings.HasPrefix(parsed.Path, "/spreadsheets/")
}

func boundedSheetSize(width, height int) (int, int) {
	if width <= 0 || height <= 0 {
		return 0, 0
	}
	return min(width, maxEmbeddedSheetWidth), min(height, maxEmbeddedSheetHeight)
}

func evaluatedInt(raw any, key string) int {
	values, ok := raw.(map[string]any)
	if !ok {
		return 0
	}
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	return 0
}

// prepareCourseMenu leaves the course sections in a known state before a
// semantic selector is evaluated. It never clicks Moodle's collapse toggles:
// Moodle saves every toggle as a user preference, so clicking changed the
// instructor's own course view in Zajuna and made each capture depend on the
// ones before it (3.1 and 4.1 showed different sections on every run).
// Sections are opened or closed in the DOM only; their content is already
// rendered, hidden by the collapse classes.
func prepareCourseMenu(page playwright.Page, options CaptureOptions) {
	selectors := append([]string{options.Selector}, options.Selectors...)
	needsCourseMenu := false
	for _, selector := range selectors {
		if strings.Contains(selector, ".course-content") || strings.Contains(selector, "activity-item") {
			needsCourseMenu = true
			break
		}
	}
	if !needsCourseMenu {
		return
	}
	_ = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: playwright.Float(5000),
	})
	_, _ = page.Evaluate(courseLayoutScript, strings.TrimSpace(options.CourseLayout))
	for _, selector := range options.RevealSelectors {
		_, _ = page.Evaluate(revealCollapseTargetScript, strings.TrimSpace(selector))
	}
	page.WaitForTimeout(300)
}

// Course layouts a capture can request (see checklist.CaptureTarget).
const (
	// CourseLayoutMenu closes every section: the evidence is the menu.
	CourseLayoutMenu = "menu"
	// CourseLayoutFirstSection closes every section and opens the first
	// top-level one with its whole subtree (material and evidence links).
	CourseLayoutFirstSection = "first-section"
)

// courseLayoutScript applies a course layout in the DOM. An empty layout
// leaves the page as Zajuna rendered it; the captured section is then opened
// by expandCourseSection.
const courseLayoutScript = `(layout) => {
	const content = document.querySelector('#region-main .course-content');
	if (!content || (layout !== 'menu' && layout !== 'first-section')) return false;
	const panelsOf = (section) => [...section.querySelectorAll(':scope > .content.collapse, :scope > .course-content-item-content.collapse')];
	const setOpen = (section, open) => {
		for (const panel of panelsOf(section)) {
			panel.classList.toggle('show', open);
			panel.style.display = open ? 'block' : 'none';
			panel.style.height = open ? 'auto' : '';
		}
		for (const toggle of section.querySelectorAll(':scope > .course-section-header [data-toggle="collapse"], :scope > .course-section-header [data-bs-toggle="collapse"]')) {
			toggle.setAttribute('aria-expanded', String(open));
			toggle.classList.toggle('collapsed', !open);
		}
	};
	const sections = [...content.querySelectorAll('li.section')];
	sections.forEach((section) => setOpen(section, false));
	if (layout === 'first-section') {
		// Section 0 is the course header (banner, announcements): it stays
		// open and is not the "first section" of the course content.
		const general = document.getElementById('section-0');
		if (general) setOpen(general, true);
		const first = sections.find((section) => section !== general && !section.parentElement.closest('li.section') && panelsOf(section).length > 0);
		if (first) {
			setOpen(first, true);
			first.querySelectorAll('li.section').forEach((section) => setOpen(section, true));
		}
	}
	return true;
}`

// revealCollapseTargetScript opens the panel a collapse control points to
// (and every collapsed ancestor) without clicking the control.
const revealCollapseTargetScript = `(selector) => {
	const control = document.querySelector(selector);
	if (!control) return false;
	const targetID = control.getAttribute('aria-controls') || (control.getAttribute('href') || '').replace(/^#/, '');
	const target = targetID ? document.getElementById(targetID) : null;
	if (!target) return false;
	control.setAttribute('aria-expanded', 'true');
	control.classList.remove('collapsed');
	let node = target;
	while (node && node.id !== 'region-main') {
		node.classList.add('show');
		node.classList.remove('collapse');
		node.removeAttribute('hidden');
		if (getComputedStyle(node).display === 'none') node.style.display = 'block';
		if (getComputedStyle(node).visibility === 'hidden') node.style.visibility = 'visible';
		node = node.parentElement;
	}
	return true;
}`

func isBlockedPage(page playwright.Page, title string) (bool, error) {
	body, err := page.Locator("body").InnerText()
	if err != nil {
		return false, nil
	}
	content := strings.ToLower(strings.TrimSpace(title + "\n" + body))
	if !containsBlockedPageMarkers(content) {
		return false, nil
	}
	return true, fmt.Errorf("%w: respuesta WAF de Zajuna", ErrBlockedPage)
}

func containsBlockedPageMarkers(content string) bool {
	content = strings.ToLower(content)
	return strings.Contains(content, "web page blocked") ||
		(strings.Contains(content, "attack id:") && strings.Contains(content, "message id:"))
}

func (r Runtime) RenderHTMLToPDF(ctx context.Context, htmlContent, outputPath string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if htmlContent == "" || outputPath == "" {
		return errors.New("HTML y ruta de PDF son obligatorios")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	absoluteOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("resolver salida PDF: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absoluteOutput), 0o700); err != nil {
		return fmt.Errorf("crear carpeta PDF: %w", err)
	}
	pw, err := r.Start()
	if err != nil {
		return err
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(true)})
	if err != nil {
		return fmt.Errorf("lanzar Chromium para PDF: %w", err)
	}
	defer browser.Close()
	browserContext, err := browser.NewContext()
	if err != nil {
		return fmt.Errorf("crear contexto PDF: %w", err)
	}
	defer browserContext.Close()
	page, err := browserContext.NewPage()
	if err != nil {
		return fmt.Errorf("crear página PDF: %w", err)
	}
	if err := page.SetContent(htmlContent); err != nil {
		return fmt.Errorf("preparar contenido PDF: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := page.PDF(playwright.PagePdfOptions{Path: playwright.String(absoluteOutput), Format: playwright.String("A4"), PrintBackground: playwright.Bool(true), Tagged: playwright.Bool(true)}); err != nil {
		return fmt.Errorf("guardar PDF: %w", err)
	}
	return nil
}

func (c BrowserCookie) playwrightCookie() (playwright.OptionalCookie, bool) {
	if strings.TrimSpace(c.Name) == "" {
		return playwright.OptionalCookie{}, false
	}
	item := playwright.OptionalCookie{
		Name:     c.Name,
		Value:    c.Value,
		HttpOnly: playwright.Bool(c.HttpOnly),
		Secure:   playwright.Bool(c.Secure),
	}
	if strings.TrimSpace(c.Domain) != "" && strings.TrimSpace(c.Path) != "" {
		item.Domain = playwright.String(c.Domain)
		item.Path = playwright.String(c.Path)
	} else if strings.TrimSpace(c.URL) != "" {
		item.URL = playwright.String(c.URL)
	} else {
		return playwright.OptionalCookie{}, false
	}
	if c.Expires != nil {
		item.Expires = playwright.Float(float64(c.Expires.Unix()))
	}
	switch strings.ToLower(strings.TrimSpace(c.SameSite)) {
	case "strict":
		item.SameSite = playwright.SameSiteAttributeStrict
	case "none":
		item.SameSite = playwright.SameSiteAttributeNone
	case "lax":
		item.SameSite = playwright.SameSiteAttributeLax
	}
	return item, true
}

// ensureStillOn returns to the captured URL if preparing the page navigated
// away, so a screenshot can never show a different page than the one
// recorded as the evidence's finalUrl.
func ensureStillOn(page playwright.Page, expected string) error {
	if sameCapturePage(page.URL(), expected) {
		return nil
	}
	if _, err := page.Goto(expected); err != nil {
		return fmt.Errorf("la página cambió al prepararla y no se pudo volver a %s: %w", expected, err)
	}
	_ = page.WaitForLoadState()
	if !sameCapturePage(page.URL(), expected) {
		return fmt.Errorf("la página cambió al prepararla (%s en lugar de %s)", page.URL(), expected)
	}
	return nil
}

// sameCapturePage compares two URLs ignoring the fragment (#section-N).
func sameCapturePage(current, expected string) bool {
	strip := func(value string) string {
		if index := strings.Index(value, "#"); index >= 0 {
			return value[:index]
		}
		return value
	}
	return strip(strings.TrimSpace(current)) == strip(strings.TrimSpace(expected))
}

const embeddedSheetSelector = `#region-main iframe[src*="docs.google.com/spreadsheets"]`

// sheetTabForTitle picks the published-sheet tab a cronograma page is about.
// The phase pages of a SENA course embed the same published spreadsheet
// without a tab, so "Fase - Hacer" used to show the FASE 1 PLANEAR tab.
func sheetTabForTitle(title string) string {
	value := strings.ToLower(title)
	for _, phase := range []string{"planear", "hacer", "verificar", "actuar", "analisis", "análisis"} {
		if strings.Contains(value, "fase - "+phase) || strings.Contains(value, "fase "+phase) || strings.Contains(value, "fase: "+phase) {
			return phase
		}
	}
	if strings.Contains(value, "cronograma general") {
		return "general"
	}
	return ""
}

// prepareEmbeddedSheets makes an embedded Google Sheets cronograma readable:
// it grows the iframe (fixed at ~726 px, which showed ~2 rows) and opens the
// tab that matches the page (phase or general); in the embedded widget the
// tabs are `td.switcherItem` cells (verified on a real course). Clicking a
// tab inside the
// published sheet only switches its own view; the page does not navigate.
func prepareEmbeddedSheets(page playwright.Page, title string) {
	frames := page.Locator(embeddedSheetSelector)
	count, err := frames.Count()
	if err != nil || count == 0 {
		return
	}
	tab := sheetTabForTitle(title)
	for index := 0; index < count; index++ {
		_, _ = frames.Nth(index).Evaluate(`(frame) => {
			frame.style.height = '2400px';
			frame.style.maxHeight = 'none';
			frame.setAttribute('height', '2400');
			return true;
		}`, nil)
		if tab == "" {
			continue
		}
		frame := page.FrameLocator(fmt.Sprintf("%s >> nth=%d", embeddedSheetSelector, index))
		// The published sheet renders its grid after load: wait for it, or
		// the evidence showed a blank grid. In the widget the grid lives in
		// a nested frame (#pageswitcher-content), not in the widget itself.
		if waitErr := frame.FrameLocator(embeddedSheetContentFrame).Locator("table.waffle td").First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: playwright.Float(8000)}); waitErr != nil {
			_ = frame.Locator("table.waffle td").First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: playwright.Float(2000)})
		}
		menu := frame.Locator("td.switcherItem, #sheet-menu a, #sheet-menu li")
		candidate := menu.Filter(playwright.LocatorFilterOptions{HasText: tab})
		if matches, countErr := candidate.Count(); countErr == nil && matches > 0 {
			_ = candidate.First().Click(playwright.LocatorClickOptions{Timeout: playwright.Float(3000)})
		}
	}
	page.WaitForTimeout(1500)
	// A fixed 2400 px still cut long phases (the general cronograma showed
	// only Inducción and part of Planear): grow each iframe to the height of
	// the sheet it now shows, within what the evidence review accepts.
	for index := 0; index < count; index++ {
		height := embeddedSheetNeededHeight(page.FrameLocator(fmt.Sprintf("%s >> nth=%d", embeddedSheetSelector, index)))
		if height <= embeddedSheetMinHeight {
			continue
		}
		if height > embeddedSheetMaxHeight {
			height = embeddedSheetMaxHeight
		}
		_, _ = frames.Nth(index).Evaluate(`(frame, height) => {
			frame.style.height = height + 'px';
			frame.setAttribute('height', String(height));
			return true;
		}`, height)
	}
	page.WaitForTimeout(1000)
}

const (
	embeddedSheetMinHeight = 2400
	// The captured region adds the page title and tabs to the iframe; the
	// review flags images taller than 9000 px.
	embeddedSheetMaxHeight = 8400
	// The published-sheet widget shows the selected tab in this nested
	// frame; the widget document only holds the tab bar around it.
	embeddedSheetContentFrame = "#pageswitcher-content"
)

// embeddedSheetNeededHeight returns the widget height that shows the whole
// selected sheet: the sheet content height plus the widget chrome (tab bar)
// around the nested content frame. 0 when it cannot be measured.
func embeddedSheetNeededHeight(widget playwright.FrameLocator) int {
	options := playwright.LocatorEvaluateOptions{Timeout: playwright.Float(3000)}
	raw, err := widget.FrameLocator(embeddedSheetContentFrame).Locator("body").Evaluate(embeddedSheetContentHeightScript, nil, options)
	if err != nil {
		// A plain pubhtml page (no widget) holds the grid directly.
		raw, err = widget.Locator("body").Evaluate(embeddedSheetContentHeightScript, nil, options)
		if err != nil {
			return 0
		}
		return evaluatedNumber(raw)
	}
	content := evaluatedNumber(raw)
	if content <= 0 {
		return 0
	}
	chrome, err := widget.Locator("body").Evaluate(embeddedSheetChromeScript, nil, options)
	if err != nil {
		return 0
	}
	return content + evaluatedNumber(chrome) + 40
}

// embeddedSheetContentHeightScript measures the full height of a sheet
// document: its scroll height or, when an inner container scrolls, the
// bottom of its grids.
const embeddedSheetContentHeightScript = `() => {
	let bottom = Math.max(document.documentElement.scrollHeight, document.body ? document.body.scrollHeight : 0);
	for (const grid of document.querySelectorAll('table.waffle')) {
		const box = grid.getBoundingClientRect();
		if (box.height > 0) bottom = Math.max(bottom, box.top + box.height + window.scrollY);
	}
	return Math.ceil(bottom);
}`

// embeddedSheetChromeScript returns the widget height not used by the
// nested content frame (tab bar, borders).
const embeddedSheetChromeScript = `() => {
	const content = document.querySelector('#pageswitcher-content');
	return content ? Math.max(0, Math.ceil(window.innerHeight - content.getBoundingClientRect().height)) : 0;
}`

func evaluatedNumber(raw any) int {
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	return 0
}

// neutralizeFloatingElements stops fixed/sticky page chrome from ending up
// inside evidence: Zajuna's top bar was stitched into the middle of full-page
// captures and a floating helper widget covered the right edge. Wide bars
// become static; small floating widgets are hidden.
func neutralizeFloatingElements(page playwright.Page) {
	_, _ = page.Evaluate(`() => {
		// Moodle's sticky action bars ("Guardar cambios" in the gradebook
		// setup) live inside #region-main and cover rows of the evidence.
		for (const node of document.querySelectorAll('#sticky-footer, .stickyfooter, [data-region="sticky-footer"]')) {
			node.style.setProperty('display', 'none', 'important');
		}
		for (const node of document.body.querySelectorAll('*')) {
			const style = getComputedStyle(node);
			if (style.position !== 'fixed' && style.position !== 'sticky') continue;
			if (node.closest('#region-main')) continue;
			const box = node.getBoundingClientRect();
			if (box.width >= window.innerWidth * 0.5) {
				node.style.setProperty('position', 'static', 'important');
			} else {
				node.style.setProperty('display', 'none', 'important');
			}
		}
		return true;
	}`)
}

// emptySectionMaxHeight is the height of a section that shows only its
// title row (Moodle renders ~85 px); the evidence review flags anything below
// 120 px as too small.
const emptySectionMaxHeight = 120

// emptyCourseSection reports a course section with no visible activity,
// resource or subsection whose box is only its title row. A text-only section
// taller than that is still captured.
func emptyCourseSection(element playwright.Locator, timeout float64) bool {
	if sectionContentItems(element, timeout) != 0 {
		return false
	}
	box, err := element.BoundingBox()
	return err == nil && box != nil && box.Height < emptySectionMaxHeight
}

// sectionContentItems counts the visible activities/resources of a course
// section, or returns -1 when the captured element is not a section.
func sectionContentItems(element playwright.Locator, timeout float64) int {
	raw, err := element.Evaluate(`(node) => {
		if (!node.matches('li.section, [data-for="section"]')) return -1;
		// Activities/resources and child subsections both count: a section
		// that organizes subsections is not empty.
		const items = [...node.querySelectorAll('li.activity, .activity-item, a[href*="/mod/"], li.section')];
		return items.filter((item) => item !== node && item.offsetParent !== null).length;
	}`, nil, playwright.LocatorEvaluateOptions{Timeout: playwright.Float(timeout)})
	if err != nil {
		return -1
	}
	switch value := raw.(type) {
	case int:
		return value
	case float64:
		return int(value)
	}
	return -1
}
