//go:build gui

package main

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jchv/go-webview2"
)

//go:embed ui/index.html
var indexHTML string

type config struct {
	Nick string `json:"nick"`
	Dir  string `json:"dir"`
}

// gameRoot — папка, куда скачивается игра.
func gameRoot() string {
	if c := loadConfig(); c.Dir != "" {
		return c.Dir
	}
	return dataDir()
}

// serverVersion спрашивает у сайта, какая версия игры сейчас нужна серверу.
func serverVersion() string {
	c := &http.Client{Timeout: 6 * time.Second}
	resp, err := c.Get("https://mcliga.ru/api/status")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var s struct {
		Version string `json:"version"`
	}
	if json.NewDecoder(resp.Body).Decode(&s) != nil {
		return ""
	}
	if !versionRe.MatchString(s.Version) {
		return ""
	}
	return s.Version
}

func configPath() string { return filepath.Join(dataDir(), "config.json") }

func loadConfig() config {
	var c config
	if b, err := os.ReadFile(configPath()); err == nil {
		json.Unmarshal(b, &c)
	}
	return c
}

func saveConfig(c config) {
	os.MkdirAll(dataDir(), 0o755)
	b, _ := json.Marshal(c)
	os.WriteFile(configPath(), b, 0o600)
}

func jsq(v any) string { b, _ := json.Marshal(v); return string(b) }

func fetchStatus() map[string]any {
	out := map[string]any{"up": false, "players": 0, "max": 30, "ping": -1}
	if st, err := queryServer(serverAddr, 25565); err == nil {
		out["up"], out["players"], out["ping"] = true, st.Online, st.PingMS
		if st.Max > 0 {
			out["max"] = st.Max
		}
	}
	if v := serverVersion(); v != "" {
		out["version"] = v
	} else {
		out["version"] = gameVersion
	}
	return out
}

func main() {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  filepath.Join(dataDir(), "webview"),
		WindowOptions: webview2.WindowOptions{
			Title:  "MCLiga",
			Width:  460,
			Height: 800,
			Center: true,
		},
	})
	if w == nil {
		os.MkdirAll(dataDir(), 0o755)
		os.WriteFile(filepath.Join(dataDir(), "error.txt"),
			[]byte("Не удалось открыть окно: нужен компонент Microsoft Edge WebView2 (есть в Windows 10 и 11)."), 0o644)
		os.Exit(1)
	}
	defer w.Destroy()
	w.SetSize(460, 800, webview2.HintFixed)

	push := func(event string, v any) {
		w.Dispatch(func() { w.Eval("window.onGo && window.onGo(" + jsq(event) + "," + jsq(v) + ")") })
	}
	say = func(a ...any) {
		parts := make([]string, 0, len(a))
		for _, x := range a {
			if s, ok := x.(string); ok {
				parts = append(parts, s)
			}
		}
		push("stage", strings.Join(parts, " "))
	}
	report = func(stage string, done, total int) {
		if done == total || done%25 == 0 {
			push("progress", map[string]any{"stage": stage, "done": done, "total": total})
		}
	}

	var mu sync.Mutex
	busy := false

	w.Bind("goInit", func() map[string]any {
		return map[string]any{"nick": loadConfig().Nick, "version": gameVersion, "dir": gameRoot(), "launcher": launcherVersion}
	})
	w.Bind("goStatus", func() {
		go func() { push("status", fetchStatus()) }()
	})
	w.Bind("goInstall", func(ver string) {
		if !versionRe.MatchString(ver) {
			return
		}
		go func() {
			root := gameRoot()
			ok, mb := installState(root, ver)
			push("install", map[string]any{"installed": ok, "sizeMB": mb, "dir": root, "version": ver})
		}()
	})
	w.Bind("goChooseDir", func() {
		go func() {
			p := pickFolder(uintptr(w.Window()), "Выберите, куда скачать игру")
			if p == "" {
				return
			}
			// отдельная подпапка, чтобы не смешивать с чужими файлами
			if filepath.Base(p) != "MCLiga" {
				p = filepath.Join(p, "MCLiga")
			}
			c := loadConfig()
			c.Dir = p
			saveConfig(c)
			push("dir", p)
		}()
	})
	w.Bind("goOpen", func(url string) {
		if strings.HasPrefix(url, "https://mcliga.ru") || strings.HasPrefix(url, "https://max.ru/") {
			exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
		}
	})
	w.Bind("goPlay", func(nick, ver string) string {
		nick = strings.TrimSpace(nick)
		if !versionRe.MatchString(ver) {
			ver = gameVersion
		}
		if !nickRe.MatchString(nick) {
			return "Ник: латинские буквы, цифры и «_», от 3 до 16 знаков."
		}
		mu.Lock()
		if busy {
			mu.Unlock()
			return "Уже запускается, подождите."
		}
		busy = true
		mu.Unlock()
		cfg := loadConfig()
		cfg.Nick = nick
		saveConfig(cfg)
		go func() {
			defer func() { mu.Lock(); busy = false; mu.Unlock() }()
			root := gameRoot()
			java, args, err := prepare(root, nick, true, ver)
			if err != nil {
				push("error", "Не удалось подготовить игру: "+err.Error())
				return
			}
			push("stage", "Запускаю игру…")
			if _, err := startGame(root, java, args); err != nil {
				push("error", "Не удалось запустить игру: "+err.Error())
				return
			}
			push("started", "Игра запущена. Это окно можно закрыть.")
		}()
		return ""
	})

	w.SetHtml(indexHTML)
	w.Run()
}
