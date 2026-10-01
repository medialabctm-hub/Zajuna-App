// Command zajuna-probe is a development tool: it logs into Zajuna with the
// account saved by the app (config.json + the system credential store) and
// checks which read-only AJAX functions and pages answer, so the app only
// depends on calls verified against the live site. It is not packaged.
//
// The output is redacted: it prints the shape of each answer (keys, types,
// counts) and only short course-structure strings (section and activity
// names), never people's names, messages, emails or the session secrets.
//
//	go -C core run ./cmd/zajuna-probe -method core_courseformat_get_state -args '{"courseid":41080}'
//	go -C core run ./cmd/zajuna-probe -page /zajuna/mod/forum/view.php?id=123
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/zajuna-app/core/internal/secrets"
	"github.com/zajuna-app/core/internal/zajuna"
)

// Course-structure keys whose short values are safe to print.
var printableKeys = map[string]bool{
	"name": true, "title": true, "subject": true, "modname": true, "module": true, "component": true,
	"format": true, "sectionname": true, "type": true, "itemtype": true, "itemmodule": true, "status": true,
	"visible": true, "uservisible": true, "number": true, "section": true, "id": true, "sectionid": true,
	"errorcode": true,
}

func main() {
	method := flag.String("method", "", "función AJAX de Moodle")
	rawArgs := flag.String("args", "{}", "argumentos JSON de la función")
	page := flag.String("page", "", "ruta de una página de Zajuna para listar enlaces (solo forma)")
	feature := flag.String("feature", "", "prueba una integración: forum-index, forum-instance, forum-export, grade-history, grade-items, pending-grading, sheet")
	featureArg := flag.Int("arg", 0, "id del curso, del foro o de la actividad para -feature")
	pattern := flag.String("match", `href="[^"]*(?:discuss|view)\.php\?[^"]*"`, "regex de enlaces a listar con -page")
	header := flag.Bool("header", false, "con -page: muestra solo la primera línea (encabezado de un CSV)")
	raw := flag.Bool("raw", false, "imprime la respuesta completa (puede contener datos personales; no la compartas)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, session, err := login(ctx)
	if err != nil {
		fail(err)
	}
	fmt.Printf("sesión: sesskey=%t userId=%t\n", session.Sesskey != "", session.UserID > 0)

	if *feature != "" {
		runFeature(ctx, client, session, *feature, *featureArg)
		return
	}
	if *page != "" {
		body, err := client.GetPage(ctx, session, *page)
		if err != nil {
			fail(err)
		}
		if *header {
			first, _, _ := strings.Cut(strings.TrimPrefix(body, string(rune(0xFEFF))), string(rune(10)))
			if len(first) > 400 {
				first = first[:400]
			}
			fmt.Printf("página %s: %d bytes, primera línea: %s\n", *page, len(body), first)
			return
		}
		links := regexp.MustCompile(*pattern).FindAllString(body, -1)
		fmt.Printf("página %s: %d bytes, %d coincidencias\n", *page, len(body), len(links))
		for index, link := range unique(links) {
			if index >= 40 {
				break
			}
			fmt.Println("  ", link)
		}
	}
	if *method != "" {
		var args map[string]any
		// "$USERID" stands for the logged-in user, so it is never typed or printed.
		*rawArgs = strings.ReplaceAll(*rawArgs, `"$USERID"`, fmt.Sprint(session.UserID))
		if err := json.Unmarshal([]byte(*rawArgs), &args); err != nil {
			fail(fmt.Errorf("-args no es JSON: %w", err))
		}
		var data any
		if err := client.CallAJAX(ctx, session, *method, args, &data); err != nil {
			fmt.Printf("%s: NO disponible: %v\n", *method, err)
			os.Exit(2)
		}
		fmt.Printf("%s: disponible\n", *method)
		// Some functions (core_courseformat_get_state) return JSON as a string.
		if text, ok := data.(string); ok {
			var nested any
			if json.Unmarshal([]byte(text), &nested) == nil {
				data = nested
			}
		}
		if *raw {
			out, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(out))
			return
		}
		describe(data, "", 0)
	}
}

func login(ctx context.Context) (*zajuna.Client, zajuna.Session, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, zajuna.Session{}, err
	}
	contents, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return nil, zajuna.Session{}, fmt.Errorf("no hay config.json en %s: %w", dir, err)
	}
	var config struct {
		Username     string `json:"zajunaUsername"`
		DocumentType string `json:"zajunaDocumentType"`
	}
	if err := json.Unmarshal(contents, &config); err != nil || config.Username == "" {
		return nil, zajuna.Session{}, fmt.Errorf("config.json sin usuario de Zajuna")
	}
	password, err := secrets.SystemStore{}.Get(config.Username)
	if err != nil {
		return nil, zajuna.Session{}, fmt.Errorf("no hay contraseña guardada para la cuenta: %w", err)
	}
	client, err := zajuna.NewClient(os.Getenv("ZAJUNA_BASE_URL"))
	if err != nil {
		return nil, zajuna.Session{}, err
	}
	session, err := client.Login(ctx, zajuna.Credentials{DocumentType: config.DocumentType, Document: config.Username, Password: password})
	return client, session, err
}

func dataDir() (string, error) {
	if dir := os.Getenv("ZAJUNA_DATA_DIR"); dir != "" {
		return dir, nil
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "ZajunaApp"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "ZajunaApp"), nil
}

// describe prints the shape of a JSON value with redacted strings.
func describe(value any, key string, depth int) {
	indent := strings.Repeat("  ", depth)
	label := key
	if label == "" {
		label = "·"
	}
	switch typed := value.(type) {
	case map[string]any:
		fmt.Printf("%s%s: {%d claves}\n", indent, label, len(typed))
		keys := make([]string, 0, len(typed))
		for name := range typed {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		for _, name := range keys {
			describe(typed[name], name, depth+1)
		}
	case []any:
		fmt.Printf("%s%s: [%d]\n", indent, label, len(typed))
		for index, item := range typed {
			if index >= 3 {
				fmt.Printf("%s  … %d más\n", indent, len(typed)-3)
				break
			}
			describe(item, fmt.Sprintf("[%d]", index), depth+1)
		}
	case string:
		if printableKeys[strings.ToLower(key)] && len([]rune(typed)) <= 90 && !strings.Contains(typed, "@") {
			fmt.Printf("%s%s: %q\n", indent, label, typed)
		} else {
			fmt.Printf("%s%s: <texto %d>\n", indent, label, len([]rune(typed)))
		}
	default:
		fmt.Printf("%s%s: %v\n", indent, label, typed)
	}
}

func unique(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

// runFeature exercises one session-only integration and prints only counts.
func runFeature(ctx context.Context, client *zajuna.Client, session zajuna.Session, feature string, arg int) {
	switch feature {
	case "forum-index":
		entries, err := client.ForumIndex(ctx, session, arg)
		report(feature, err, fmt.Sprintf("%d foros", len(entries)))
		for index, entry := range entries {
			if index < 5 {
				fmt.Printf("   foro %d: %d debates · %q\n", entry.ForumID, entry.Discussions, entry.Name)
			}
		}
	case "forum-instance":
		id, err := client.ForumInstanceID(ctx, session, arg)
		report(feature, err, fmt.Sprintf("id de foro encontrado: %t", id > 0))
	case "forum-export":
		posts, err := client.ExportForum(ctx, session, arg)
		unanswered, oldest := zajuna.UnansweredPosts(posts, session.UserID)
		authors, mine := map[int]bool{}, 0
		for _, post := range posts {
			authors[post.UserID] = true
			if post.UserID == session.UserID {
				mine++
			}
		}
		fmt.Printf("   autores distintos: %d · autor 0 presente: %t · mensajes del instructor: %d\n", len(authors), authors[0], mine)
		report(feature, err, fmt.Sprintf("%d mensajes, %d sin respuesta del instructor (el más antiguo %s)", len(posts), unanswered, time.Unix(oldest, 0).Format("2006-01-02")))
	case "grade-history":
		entries, err := client.GradeHistory(ctx, session, arg)
		graded, feedback := 0, 0
		for _, entry := range entries {
			if entry.Graded {
				graded++
			}
			if entry.HasFeedback {
				feedback++
			}
		}
		report(feature, err, fmt.Sprintf("%d eventos, %d con calificación, %d con retroalimentación", len(entries), graded, feedback))
	case "grade-items":
		count, err := client.GradeItemCount(ctx, session, arg)
		report(feature, err, fmt.Sprintf("%d ítems de calificación", count))
	case "pending-grading":
		pending, err := client.PendingGradings(ctx, session, arg)
		total := 0
		for _, entry := range pending {
			total += entry.Count
		}
		report(feature, err, fmt.Sprintf("%d actividades con %d entregas por calificar", len(pending), total))
	case "sheet":
		body, err := client.GetPage(ctx, session, fmt.Sprintf("/zajuna/mod/page/view.php?id=%d", arg))
		if err != nil {
			report(feature, err, "")
			return
		}
		for _, csvURL := range zajuna.PublishedSheetCSVURLs(body) {
			rows, sheetErr := zajuna.FetchPublishedSheet(ctx, csvURL)
			report(feature, sheetErr, fmt.Sprintf("%d filas · problemas: %v", len(rows), zajuna.SheetIssues(rows)))
		}
	default:
		fail(fmt.Errorf("integración desconocida %q", feature))
	}
}

func report(feature string, err error, detail string) {
	if err != nil {
		fmt.Printf("%s: NO disponible: %v\n", feature, err)
		return
	}
	fmt.Printf("%s: disponible · %s\n", feature, detail)
}
