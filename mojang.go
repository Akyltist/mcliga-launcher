package main

// Работа с официальными метаданными Mojang: версия игры, библиотеки, ресурсы, Java.

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	manifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
	runtimeURL  = "https://launchermeta.mojang.com/v1/products/java-runtime/2ec0cc96c44e5a76b9c8b7c39df7210883d12871/all.json"
	assetsURL   = "https://resources.download.minecraft.net/"
)

var httpc = &http.Client{Timeout: 5 * time.Minute}

// ---------- структуры JSON ----------

type fileRef struct {
	Path string `json:"path"`
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

type rule struct {
	Action   string                       `json:"action"`
	OS       *struct{ Name, Arch string } `json:"os"`
	Features map[string]bool              `json:"features"`
}

type library struct {
	Name      string `json:"name"`
	Downloads struct {
		Artifact *fileRef `json:"artifact"`
	} `json:"downloads"`
	Rules []rule `json:"rules"`
}

type versionJSON struct {
	ID         string `json:"id"`
	MainClass  string `json:"mainClass"`
	Type       string `json:"type"`
	AssetIndex struct {
		ID string `json:"id"`
		fileRef
	} `json:"assetIndex"`
	Assets    string `json:"assets"`
	Downloads struct {
		Client fileRef `json:"client"`
	} `json:"downloads"`
	JavaVersion struct {
		Component    string `json:"component"`
		MajorVersion int    `json:"majorVersion"`
	} `json:"javaVersion"`
	Libraries []library `json:"libraries"`
	Arguments struct {
		Game []json.RawMessage `json:"game"`
		JVM  []json.RawMessage `json:"jvm"`
	} `json:"arguments"`
}

// ---------- правила Mojang (операционная система, функции) ----------

func ruleAllows(rules []rule, features map[string]bool) bool {
	if len(rules) == 0 {
		return true
	}
	allowed := false
	for _, r := range rules {
		match := true
		if r.OS != nil {
			if r.OS.Name != "" && r.OS.Name != "windows" {
				match = false
			}
			if r.OS.Arch != "" && r.OS.Arch != "x86_64" && r.OS.Arch != "amd64" {
				match = false // x86 / arm64 для 64-битного Windows не подходят
			}
		}
		for k, v := range r.Features {
			if features[k] != v {
				match = false
			}
		}
		if match {
			allowed = r.Action == "allow"
		}
	}
	return allowed
}

// ---------- загрузка ----------

func getJSON(url string, v any) error {
	var last error
	for i := 0; i < 5; i++ {
		resp, err := httpc.Get(url)
		if err == nil {
			body, rerr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if rerr == nil && resp.StatusCode == 200 {
				return json.Unmarshal(body, v)
			}
			last = fmt.Errorf("%s: код %d %v", url, resp.StatusCode, rerr)
		} else {
			last = err
		}
		time.Sleep(time.Duration(i+1) * 2 * time.Second)
	}
	return last
}

func sha1File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ensure скачивает файл, если его нет или контрольная сумма не совпала.
func ensure(path, url, wantSHA1 string) error {
	if sum, err := sha1File(path); err == nil && (wantSHA1 == "" || strings.EqualFold(sum, wantSHA1)) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var last error
	for i := 0; i < 5; i++ {
		last = download(path, url, wantSHA1)
		if last == nil {
			return nil
		}
		time.Sleep(time.Duration(i+1) * 2 * time.Second)
	}
	return fmt.Errorf("%s: %w", url, last)
}

func download(path, url, wantSHA1 string) error {
	resp, err := httpc.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("код %d", resp.StatusCode)
	}
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	h := sha1.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	if wantSHA1 != "" && !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), wantSHA1) {
		os.Remove(tmp)
		return fmt.Errorf("контрольная сумма не совпала")
	}
	return os.Rename(tmp, path)
}

type job struct{ path, url, sha1 string }

// runJobs качает файлы параллельно и показывает ход.
func runJobs(title string, jobs []job, progress func(done, total int)) error {
	var done int32
	var first atomic.Value
	sem := make(chan struct{}, 12)
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			if first.Load() != nil {
				return
			}
			if err := ensure(j.path, j.url, j.sha1); err != nil {
				first.CompareAndSwap(nil, err)
			}
			progress(int(atomic.AddInt32(&done, 1)), len(jobs))
		}(j)
	}
	wg.Wait()
	if e := first.Load(); e != nil {
		return fmt.Errorf("%s: %v", title, e)
	}
	return nil
}
