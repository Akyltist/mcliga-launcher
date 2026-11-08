//go:build !gui

package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	nick := flag.String("nick", "", "игровой ник (латиница, цифры, _; 3–16 знаков)")
	dir := flag.String("dir", "", "папка для игры (по умолчанию %APPDATA%\\MCLiga)")
	noJoin := flag.Bool("nojoin", false, "не подключаться к серверу сразу")
	dry := flag.Bool("dry", false, "только скачать, не запускать игру")
	flag.Parse()

	root := *dir
	if root == "" {
		root = dataDir()
	}
	if !nickRe.MatchString(*nick) {
		fmt.Println("Укажите ник: -nick Имя (латиница, цифры и _, от 3 до 16 знаков)")
		os.Exit(2)
	}
	java, args, err := prepare(root, *nick, !*noJoin, gameVersion)
	if err != nil {
		fmt.Println("Ошибка:", err)
		os.Exit(1)
	}
	if *dry {
		fmt.Println("Всё скачано и проверено. Запуск пропущен (-dry).")
		return
	}
	fmt.Println("Запускаю игру…")
	pid, err := startGame(root, java, args)
	if err != nil {
		fmt.Println("Не удалось запустить:", err)
		os.Exit(1)
	}
	fmt.Println("Игра запущена, PID", pid)
}
