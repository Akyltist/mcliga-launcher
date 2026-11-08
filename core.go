package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	launcherName    = "MCLigaLauncher"
	launcherVersion = "0.1"
	gameVersion     = "1.21.11"
	serverAddr      = "play.mcliga.ru"
)

var versionRe = regexp.MustCompile(`^[0-9]+(.[0-9]+){1,2}$`)

var nickRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)

// offlineUUID повторяет то, как сервер без проверки лицензии вычисляет UUID игрока.
func offlineUUID(name string) string {
	sum := md5.Sum([]byte("OfflinePlayer:" + name))
	sum[6] = (sum[6] & 0x0f) | 0x30
	sum[8] = (sum[8] & 0x3f) | 0x80
	h := fmt.Sprintf("%x", sum)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// say выводит сообщение (в консоль или в окно).
var say = func(a ...any) { fmt.Println(a...) }

// report сообщает ход этапа: название, сделано, всего.
var report = func(stage string, done, total int) {
	if done == total || done%200 == 0 {
		fmt.Printf("  %s: %d из %d\n", stage, done, total)
	}
}

func progress(title string) func(done, total int) {
	return func(done, total int) { report(title, done, total) }
}

// prepare скачивает всё нужное для игры и возвращает путь к Java и аргументы запуска.
func prepare(root, nick string, quickPlay bool, ver string) (javaPath string, args []string, err error) {
	// 1. версия
	var manifest struct {
		Versions []struct{ ID, URL, SHA1 string } `json:"versions"`
	}
	say("Получаю список версий у Mojang…")
	if err = getJSON(manifestURL, &manifest); err != nil {
		return
	}
	var vurl, vsha string
	for _, v := range manifest.Versions {
		if v.ID == ver {
			vurl, vsha = v.URL, v.SHA1
		}
	}
	if vurl == "" {
		err = fmt.Errorf("версия %s не найдена", ver)
		return
	}
	vdir := filepath.Join(root, "versions", ver)
	vfile := filepath.Join(vdir, ver+".json")
	if err = ensure(vfile, vurl, vsha); err != nil {
		return
	}
	var v versionJSON
	raw, _ := os.ReadFile(vfile)
	if err = json.Unmarshal(raw, &v); err != nil {
		return
	}

	features := map[string]bool{"is_quick_play_multiplayer": quickPlay}

	// 2. клиент и библиотеки
	jobs := []job{{filepath.Join(vdir, ver+".jar"), v.Downloads.Client.URL, v.Downloads.Client.SHA1}}
	var cp []string
	for _, l := range v.Libraries {
		if !ruleAllows(l.Rules, features) || l.Downloads.Artifact == nil {
			continue
		}
		a := l.Downloads.Artifact
		p := filepath.Join(root, "libraries", filepath.FromSlash(a.Path))
		jobs = append(jobs, job{p, a.URL, a.SHA1})
		cp = append(cp, p)
	}
	cp = append(cp, filepath.Join(vdir, ver+".jar"))
	say("Скачиваю игру и библиотеки…")
	if err = runJobs("игра", jobs, progress("файлы")); err != nil {
		return
	}

	// 3. ресурсы (звуки, языки)
	idxPath := filepath.Join(root, "assets", "indexes", v.AssetIndex.ID+".json")
	if err = ensure(idxPath, v.AssetIndex.URL, v.AssetIndex.SHA1); err != nil {
		return
	}
	var idx struct {
		Objects map[string]struct {
			Hash string `json:"hash"`
			Size int64  `json:"size"`
		} `json:"objects"`
	}
	raw, _ = os.ReadFile(idxPath)
	if err = json.Unmarshal(raw, &idx); err != nil {
		return
	}
	var ajobs []job
	for _, o := range idx.Objects {
		ajobs = append(ajobs, job{filepath.Join(root, "assets", "objects", o.Hash[:2], o.Hash), assetsURL + o.Hash[:2] + "/" + o.Hash, o.Hash})
	}
	say("Скачиваю звуки и ресурсы…")
	if err = runJobs("ресурсы", ajobs, progress("ресурсы")); err != nil {
		return
	}

	// 4. Java, которую выдаёт Mojang
	javaPath, err = ensureJava(root, v.JavaVersion.Component)
	if err != nil {
		return
	}

	// 5. аргументы запуска
	gameDir := filepath.Join(root, "game")
	os.MkdirAll(gameDir, 0o755)
	applyFirstRunOptions(gameDir)
	repl := map[string]string{
		"auth_player_name":     nick,
		"version_name":         ver,
		"game_directory":       gameDir,
		"assets_root":          filepath.Join(root, "assets"),
		"assets_index_name":    v.AssetIndex.ID,
		"auth_uuid":            offlineUUID(nick),
		"auth_access_token":    "0",
		"clientid":             "",
		"auth_xuid":            "",
		"user_type":            "legacy",
		"version_type":         launcherName,
		"launcher_name":        launcherName,
		"launcher_version":     launcherVersion,
		"classpath":            strings.Join(cp, string(os.PathListSeparator)),
		"natives_directory":    filepath.Join(root, "natives"),
		"quickPlayMultiplayer": serverAddr,
	}
	sub := func(s string) string {
		for k, val := range repl {
			s = strings.ReplaceAll(s, "${"+k+"}", val)
		}
		return s
	}
	collect := func(items []json.RawMessage) []string {
		var out []string
		for _, it := range items {
			var s string
			if json.Unmarshal(it, &s) == nil {
				out = append(out, sub(s))
				continue
			}
			var o struct {
				Rules []rule          `json:"rules"`
				Value json.RawMessage `json:"value"`
			}
			if json.Unmarshal(it, &o) != nil || !ruleAllows(o.Rules, features) {
				continue
			}
			var one string
			var many []string
			if json.Unmarshal(o.Value, &one) == nil {
				out = append(out, sub(one))
			} else if json.Unmarshal(o.Value, &many) == nil {
				for _, m := range many {
					out = append(out, sub(m))
				}
			}
		}
		return out
	}
	args = append(args, "-Xmx2G")
	args = append(args, collect(v.Arguments.JVM)...)
	args = append(args, v.MainClass)
	args = append(args, collect(v.Arguments.Game)...)
	return javaPath, args, nil
}

// ensureJava скачивает Java нужной версии из каталога Mojang и возвращает путь к javaw.exe.
func ensureJava(root, component string) (string, error) {
	var all map[string]map[string][]struct {
		Manifest fileRef `json:"manifest"`
	}
	say("Проверяю Java…")
	if err := getJSON(runtimeURL, &all); err != nil {
		return "", err
	}
	items := all["windows-x64"][component]
	if len(items) == 0 {
		return "", fmt.Errorf("Java %q для Windows не найдена в каталоге Mojang", component)
	}
	var man struct {
		Files map[string]struct {
			Type      string `json:"type"`
			Downloads struct {
				Raw fileRef `json:"raw"`
			} `json:"downloads"`
		} `json:"files"`
	}
	mpath := filepath.Join(root, "runtime", component+".manifest.json")
	if err := ensure(mpath, items[0].Manifest.URL, items[0].Manifest.SHA1); err != nil {
		return "", err
	}
	raw, _ := os.ReadFile(mpath)
	if err := json.Unmarshal(raw, &man); err != nil {
		return "", err
	}
	base := filepath.Join(root, "runtime", component)
	var jobs []job
	for p, f := range man.Files {
		if f.Type != "file" {
			continue
		}
		jobs = append(jobs, job{filepath.Join(base, filepath.FromSlash(p)), f.Downloads.Raw.URL, f.Downloads.Raw.SHA1})
	}
	say("Скачиваю Java…")
	if err := runJobs("Java", jobs, progress("Java")); err != nil {
		return "", err
	}
	exe := filepath.Join(base, "bin", "javaw.exe")
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("в скачанной Java нет javaw.exe")
	}
	return exe, nil
}

// startGame запускает игру и возвращает PID.
func startGame(root, java string, args []string) (int, error) {
	cmd := exec.Command(java, args...)
	cmd.Dir = filepath.Join(root, "game")
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	go cmd.Wait()
	return pid, nil
}

// dataDir — папка лаунчера по умолчанию.
func dataDir() string {
	base, _ := os.UserConfigDir()
	return filepath.Join(base, "MCLiga")
}

// installState сообщает, скачана ли игра нужной версии, и сколько места она занимает.
func installState(root, ver string) (installed bool, sizeMB int64) {
	jar := filepath.Join(root, "versions", ver, ver+".jar")
	idx := filepath.Join(root, "assets", "indexes")
	_, e1 := os.Stat(jar)
	_, e2 := os.Stat(idx)
	_, e3 := os.Stat(filepath.Join(root, "runtime"))
	installed = e1 == nil && e2 == nil && e3 == nil
	var total int64
	filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, e := d.Info(); e == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return installed, total / (1 << 20)
}
