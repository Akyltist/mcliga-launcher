package main

// Первые настройки игры: русский язык и без вводных окон.
// Записываются один раз для папки игры (метка .mcliga-first), дальше игрок меняет всё сам.

import (
	"os"
	"path/filepath"
	"strings"
)

var firstRunOptions = [][2]string{
	{"lang", "ru_ru"},
	{"onboardAccessibility", "false"},  // окно «Рассказчик / Специальные возможности»
	{"tutorialStep", "none"},           // подсказки обучения
	{"skipMultiplayerWarning", "true"}, // предупреждение перед сетевой игрой
	{"joinedFirstServer", "true"},      // подсказка про чат при первом входе
}

// applyFirstRunOptions добавляет наши настройки в options.txt, сохраняя остальное.
func applyFirstRunOptions(gameDir string) {
	marker := filepath.Join(gameDir, ".mcliga-first")
	if _, err := os.Stat(marker); err == nil {
		return
	}
	path := filepath.Join(gameDir, "options.txt")
	var lines []string
	if b, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	}
	for _, kv := range firstRunOptions {
		found := false
		for i, l := range lines {
			if strings.HasPrefix(l, kv[0]+":") {
				lines[i] = kv[0] + ":" + kv[1]
				found = true
			}
		}
		if !found {
			lines = append(lines, kv[0]+":"+kv[1])
		}
	}
	var clean []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			clean = append(clean, l)
		}
	}
	if err := os.WriteFile(path, []byte(strings.Join(clean, "\n")+"\n"), 0o644); err == nil {
		os.WriteFile(marker, []byte("1"), 0o644)
	}
}
