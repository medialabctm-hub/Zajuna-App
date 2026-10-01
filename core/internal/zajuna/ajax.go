package zajuna

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/zajuna-app/core/internal/security"
)

// Moodle's internal AJAX API (/lib/ajax/service.php) answers the same calls
// the Zajuna pages make, with the session cookie and its sesskey. It returns
// structured data (course sections, forum posts) where scraping had to guess
// from the rendered HTML. Only functions marked "ajax => true" in Moodle are
// reachable, with the permissions of the logged-in instructor. Every caller
// must treat ErrAJAXUnavailable as "use the previous method": the API is not
// a public contract of Zajuna.

// ErrAJAXUnavailable means the call could not be answered through AJAX (no
// sesskey, function disabled, unexpected answer). Callers fall back.
var ErrAJAXUnavailable = errors.New("la API AJAX de Zajuna no está disponible")

// AJAXError is an exception returned by a Moodle web service function.
type AJAXError struct {
	Method    string
	ErrorCode string
	Message   string
}

func (e *AJAXError) Error() string {
	return fmt.Sprintf("AJAX %s: %s (%s)", e.Method, e.Message, e.ErrorCode)
}

func (e *AJAXError) Unwrap() error {
	switch e.ErrorCode {
	case "servicerequireslogin", "requireloginerror", "invalidsesskey":
		return ErrSessionExpired
	}
	return ErrAJAXUnavailable
}

var (
	sesskeyPattern = regexp.MustCompile(`"sesskey"\s*:\s*"([A-Za-z0-9]{6,32})"`)
	sesskeyInput   = regexp.MustCompile(`(?i)name=["']sesskey["']\s+value=["']([A-Za-z0-9]{6,32})["']`)
	// Candidate ids of the logged-in user. Zajuna's M.cfg has no userId and a
	// page can carry other people's ids (2 distinct ones on my/courses.php),
	// so candidates are confirmed through AJAX (ResolveUserID).
	userIDCandidatePattern = regexp.MustCompile(`"userId"\s*:\s*(\d+)|data-userid=["'](\d+)["']|/user/profile\.php\?id=(\d+)`)
)

// parseSesskey reads the session key Moodle publishes in M.cfg on every
// authenticated page.
func parseSesskey(body string) string {
	if match := sesskeyPattern.FindStringSubmatch(body); len(match) == 2 {
		return match[1]
	}
	if match := sesskeyInput.FindStringSubmatch(body); len(match) == 2 {
		return match[1]
	}
	return ""
}

// userIDCandidates lists the distinct user ids a page mentions.
func userIDCandidates(body string) []int {
	seen := map[int]bool{}
	ids := []int{}
	for _, match := range userIDCandidatePattern.FindAllStringSubmatch(body, -1) {
		for _, group := range match[1:] {
			if id, err := strconv.Atoi(group); err == nil && id > 1 && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		if len(ids) >= 20 {
			break
		}
	}
	return ids
}

// ResolveUserID confirms which candidate id belongs to the logged-in
// instructor: core_user_get_users_by_field returns the users the account can
// see, and the one whose full name is the session's profile name wins (or
// the only one returned when the name is unknown). Any doubt returns 0 and
// the forum checks stay off: a wrong id would hide the instructor's replies.
func (c *Client) ResolveUserID(ctx context.Context, session Session, candidates []int) int {
	if len(candidates) == 0 || !session.HasAJAX() {
		return 0
	}
	values := make([]string, 0, len(candidates))
	for _, id := range candidates {
		values = append(values, strconv.Itoa(id))
	}
	var users []struct {
		ID       json.Number `json:"id"`
		FullName string      `json:"fullname"`
	}
	if err := c.CallAJAX(ctx, session, "core_user_get_users_by_field", map[string]any{"field": "id", "values": values}, &users); err != nil {
		return 0
	}
	name := FoldText(session.ProfileName)
	match := 0
	for _, user := range users {
		id := numberToInt(user.ID)
		if id <= 1 {
			continue
		}
		if name != "" && FoldText(user.FullName) == name {
			if match != 0 && match != id {
				return 0
			}
			match = id
		}
	}
	if match == 0 && name == "" && len(users) == 1 {
		match = numberToInt(users[0].ID)
	}
	return match
}

type ajaxRequest struct {
	Index      int    `json:"index"`
	MethodName string `json:"methodname"`
	Args       any    `json:"args"`
}

type ajaxResponse struct {
	Error     bool            `json:"error"`
	Data      json.RawMessage `json:"data"`
	Exception *struct {
		Message   string `json:"message"`
		ErrorCode string `json:"errorcode"`
	} `json:"exception"`
}

// CallAJAX runs one read-only Moodle web service function through the
// session and decodes its data into out.
func (c *Client) CallAJAX(ctx context.Context, session Session, method string, args any, out any) error {
	if session.Client == nil {
		return ErrSessionExpired
	}
	if strings.TrimSpace(session.Sesskey) == "" {
		return fmt.Errorf("%w: la sesión no tiene sesskey", ErrAJAXUnavailable)
	}
	if args == nil {
		args = map[string]any{}
	}
	payload, err := json.Marshal([]ajaxRequest{{Index: 0, MethodName: method, Args: args}})
	if err != nil {
		return fmt.Errorf("argumentos AJAX inválidos: %w", err)
	}
	target := c.baseURL + "/zajuna/lib/ajax/service.php?sesskey=" + url.QueryEscape(session.Sesskey) + "&info=" + url.QueryEscape(method)
	allowPrivate := strings.HasPrefix(strings.ToLower(c.baseURL), "http://127.0.0.1") || strings.HasPrefix(strings.ToLower(c.baseURL), "http://localhost")
	if _, err := security.ValidateHTTPURL(target, []string{c.baseURL}, allowPrivate); err != nil {
		return fmt.Errorf("destino de Zajuna no permitido: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	request.Header.Set("X-Requested-With", "XMLHttpRequest")
	request.Header.Set("User-Agent", c.userAgent)
	request.Header.Set("Origin", c.baseURL)
	request.Header.Set("Referer", c.baseURL+"/zajuna/my/courses.php")
	response, err := session.Client.Do(request)
	if err != nil {
		// A *url.Error carries the full URL, sesskey included: keep only the cause.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("conectar con Zajuna (AJAX %s): %w", method, err)
	}
	body, err := readBody(response)
	response.Body.Close()
	if err != nil {
		return err
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		// A redirect is Moodle sending an expired session to the login page.
		return fmt.Errorf("%w: AJAX %s redirigió al login", ErrSessionExpired, method)
	}
	if response.StatusCode >= 400 {
		return fmt.Errorf("%w: AJAX %s respondió HTTP %d", ErrAJAXUnavailable, method, response.StatusCode)
	}
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "{") {
		// A request-level failure (bad sesskey, login required) is one object.
		var failure struct {
			Error     string `json:"error"`
			ErrorCode string `json:"errorcode"`
		}
		_ = json.Unmarshal([]byte(trimmed), &failure)
		return &AJAXError{Method: method, ErrorCode: firstNonEmpty(failure.ErrorCode, "unknown"), Message: firstNonEmpty(failure.Error, "respuesta inesperada")}
	}
	var responses []ajaxResponse
	if err := json.Unmarshal([]byte(trimmed), &responses); err != nil || len(responses) != 1 {
		return fmt.Errorf("%w: AJAX %s devolvió una respuesta que no es JSON", ErrAJAXUnavailable, method)
	}
	answer := responses[0]
	if answer.Error {
		if answer.Exception != nil {
			return &AJAXError{Method: method, ErrorCode: answer.Exception.ErrorCode, Message: answer.Exception.Message}
		}
		return &AJAXError{Method: method, ErrorCode: "unknown", Message: "error sin detalle"}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(answer.Data, out); err != nil {
		return fmt.Errorf("%w: AJAX %s devolvió datos con otra forma: %v", ErrAJAXUnavailable, method, err)
	}
	return nil
}

// HasAJAX tells whether the session can use the AJAX API at all.
func (s Session) HasAJAX() bool {
	return s.Client != nil && strings.TrimSpace(s.Sesskey) != ""
}

// GetPage reads an authenticated page of the configured Zajuna origin.
func (c *Client) GetPage(ctx context.Context, session Session, path string) (string, error) {
	if session.Client == nil {
		return "", ErrSessionExpired
	}
	body, err := c.doGet(ctx, session, c.absolute(path), "/zajuna/my/courses.php")
	if err != nil {
		return "", err
	}
	if looksLikeLoginPage(body) {
		return "", fmt.Errorf("%w: la página pidió iniciar sesión", ErrSessionExpired)
	}
	return body, nil
}
