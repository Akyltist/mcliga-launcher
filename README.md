# MCLiga Launcher

Простой лаунчер для сервера Minecraft **MCLiga** (play.mcliga.ru). Скачивает игру и Java с официальных серверов Mojang и запускает её сразу на нашем сервере. Родителю не нужно ничего настраивать: ни Java, ни версию игры, ни язык.

A small launcher for the MCLiga Minecraft server (play.mcliga.ru). It downloads the game and Java runtime from Mojang's official servers and starts the game connected to our server. Windows only for now.

Сайт проекта / Project website: https://mcliga.ru

## Что делает программа

1. Спрашивает у сайта mcliga.ru, какая версия игры сейчас нужна серверу.
2. Скачивает игру, библиотеки, ресурсы и Java **только с серверов Mojang** (`piston-meta.mojang.com`, `launchermeta.mojang.com`, `resources.download.minecraft.net`, `libraries.minecraft.net`) и проверяет каждый файл по контрольной сумме SHA-1, которую публикует Mojang.
3. При первом запуске записывает настройки игры (русский язык, без вводных окон). Дальше настройки меняет игрок.
4. Запускает игру с введённым ником и подключает к серверу.

Программа не устанавливает ничего в систему (нет установщика, нет автозапуска, нет служб). Игра и Java лежат в одной папке (по умолчанию `%APPDATA%\MCLiga`), которую можно выбрать в окне лаунчера. Удалить всё можно, удалив эту папку и сам файл.

## Конфиденциальность / Privacy

Программа обращается только к:

- `https://mcliga.ru/api/status`: узнать версию игры и количество игроков онлайн (без передачи данных об игроке);
- `play.mcliga.ru:25565`: проверить, доступен ли сервер, и измерить пинг;
- серверам Mojang (перечислены выше): скачать игру. Mojang видит ваш IP-адрес, как при использовании любого лаунчера.

Ваш игровой ник сохраняется только в локальном файле `config.json` рядом с игрой. Лаунчер не отправляет его ни на какие серверы, кроме самого игрового сервера при подключении. Аналитики и телеметрии в лаунчере нет. Сама игра (клиент Minecraft от Mojang) может обращаться к сервисам Mojang по своим правилам.

This program will not transfer any information to other networked systems unless specifically requested by the user or the person installing or operating it. It only contacts the addresses listed above; the player's nickname is stored locally and sent only to the game server upon connection.

## Code signing policy

Free code signing provided by [SignPath.io](https://signpath.io), certificate by [SignPath Foundation](https://signpath.org). *(Заявка ещё не подана: пока релизы не подписаны. Раздел заполняется после одобрения.)*

- Авторы и проверяющие: участники репозитория, список ниже.
- Утверждает каждую подпись: владелец репозитория.
- Все релизы собираются автоматически из этого репозитория (см. `.github/workflows/build.yml`); файл можно воспроизвести из исходного кода.

## Сборка

Нужен Go 1.27.3 (версия, которой собираются релизы) и Windows. Шаг `go run ./tools/mkres` создаёт свойства файла (название, версия из файла `VERSION`) и манифест. Параметры `CGO_ENABLED=0` и `-buildvcs=false` важны: с ними результат совпадает с автосборкой GitHub.

```
go vet -tags gui .
set CGO_ENABLED=0
go run ./tools/mkres
go build -tags gui -trimpath -buildvcs=false -ldflags "-s -w -H windowsgui" -o MCLiga.exe .
```

Консольная версия (для проверки, без окна):

```
go build -o mcliga-launcher.exe .
mcliga-launcher.exe -nick Имя -dry
```

## Зависимости и лицензии

- [github.com/jchv/go-webview2](https://github.com/jchv/go-webview2): MIT.
- [github.com/jchv/go-winloader](https://github.com/jchv/go-winloader): ISC.
- golang.org/x/sys: BSD-3-Clause.
- Microsoft WebView2 Loader (встроен в go-webview2): BSD-3-Clause, по лицензии Microsoft WebView2 SDK.
- Для работы окна на компьютере нужен компонент Microsoft Edge WebView2 (есть в Windows 10 и 11).

## Лицензия

MIT, см. файл [LICENSE](LICENSE). Minecraft является торговой маркой Mojang AB. Проект не связан с Mojang и Microsoft.
