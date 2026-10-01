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
	pattern := flag.String("match", `href="[^"]*(?:discuss|view)\.php\?[^"]*"`, "regex de enlaces a listar con -page")
	raw := flag.Bool("raw", false, "imprime la respuesta completa (puede contener datos personales; no la compartas)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, session, err := login(ctx)
	if err != nil {
		fail(err)
	}
	fmt.Printf("sesión: sesskey=%t userId=%t\n", session.Sesskey != "", session.UserID > 0)

	if *page != "" {
		body, err := client.GetPage(ctx, session, *page)
		if err != nil {
			fail(err)
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
