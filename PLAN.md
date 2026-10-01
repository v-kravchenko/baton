# baton: план

## Мета

`baton` — самостійний CLI, яким агент (Claude Code, opencode, будь-який інший) зберігає стан сесії як Markdown-handoff для іменованої задачі, а пізніша сесія на будь-якій машині й з будь-яким агентом його підхоплює. Агенти — це інтеграції, які налаштовує користувач, а не ядро. Проєкт самодостатній: ні від чого не залежить, ні на що не посилається і не мігрує дані з інших інструментів.

Що вміє:
- **Задачі.** Кілька іменованих задач на проєкт (`@api-refactor`), history з keep-N, done/restore, rename, forks (`from:`), staleness-звіт за git.
- **Pickup.** `baton pickup <project> [@task]` переходить у теку проєкту на цій машині й запускає налаштованого агента з промптом відновлення.
- **Tips.** Короткі неперевірені підказки з кроком Verify: статуси verified/refuted/superseded, рівні проєкт/глобальний, пошук за ключовими словами.
- **Дашборд.** Сітка проєктів, панель задачі з history і diff, Done/Restore/Rename/Delete tip, пошук, кнопки pickup для кожного агента, вхід за паролем для доступу не з loopback. Доступ або повний, або жодного: read-only режиму немає.
- **Синхронізація.** Root — звичайні `.md`, які можна шарити між машинами (Syncthing тощо); стан конкретної машини лежить поза root.

## Рішення

| Питання | Рішення |
| --- | --- |
| Мова | **Go**, один статичний бінарник (обґрунтування нижче) |
| Ключ проєкту | Завжди назва теки (див. «Ключ проєкту»). Різні теки з однаковою назвою — один проєкт; розводить їх користувач перейменуванням теки |
| `paths` | Лише «ключ → тека на цій машині» для `pickup` і дашборда. `save`/`show` мовчки записують поточну теку: остання використана перемагає. Вручну — `baton path set\|list\|prune` |
| `pickup` без шляху | Якщо назва поточної теки дає той самий ключ — мовчки пише її в `paths` і запускає агента тут, як `save`/`show`. Якщо ключ інший (агент у `~` шукав би проєкт `vova`) — помилка з підказкою `cd` у теку проєкту або `baton path set <key> DIR` |
| Агенти | Іменовані шаблони `agent.<name>` і `agent.default`, без override на проєкт. `baton pickup` бере default, `--agent X` обирає інший. На дашборді окрема кнопка на кожного агента з config |
| Frontmatter | Лише мінімум без машинних шляхів (див. «Формат handoff») |
| Скіли | Викликають тільки `baton`; у каталогах скілів немає скриптів, лише `SKILL.md` |
| Claude Code | Лише `baton integrate claude` (короткі `/handoff` `/pickup` `/tips`), без plugin |
| Config | Плаский `key = value`, коментарі `#`, порядок `agent.*` зберігається |
| Обсяг першого релізу | Усе одразу: store, pickup, дашборд, tips, інтеграції Claude і opencode |
| Оновлення | `baton update`; перше встановлення — `install.sh` / `install.ps1` |
| Структура root | Усе про проєкт в одній теці `<root>/<project>/`: `tasks/`, `archive/`, `history/`, `tips/`; без `_` і без опису проєкту |

### Чому Go

- **Швидкодія.** Агенти викликають `baton` багато разів за сесію. Заміри на цій машині: `python3` з імпортами дашборда стартує за 70–80 мс, без них за 10–20 мс; на телефоні в Termux це в 3–5 разів довше. Go-бінарник стартує за 2–5 мс без жодних застережень. Без хуків tips цей аргумент слабший, але рішення тримається на поширенні й Termux.
- **Розширюваність.** Агенти — це дані (шаблони в config, вбудовані файли інтеграцій), а не код. Нова інтеграція — це каталог `integrations/<agent>/` плюс запис у реєстрі. Пакети `internal/*` з чіткими межами типізовані, тому рефакторинг безпечний.
- **Поширення.** Статична збірка з `CGO_ENABLED=0` для Linux, macOS і Windows (див. «Цілі збірки»). Бінарник `linux/arm64` запускається в Termux як є. HTML, JS і скіли вбудовуються через `embed`, тож це справді «один інструмент».
- **Ціна.** Зовнішня залежність одна: `golang.org/x/crypto/scrypt`. Фронтенд дашборда — vanilla HTML/CSS/JS без збірки, вбудований через `embed`. Go тут ще не встановлений: `sudo dnf install golang`.
- Rust відкинуто. Швидкодія та сама, бо вузьке місце — диск і git, а не мова. Натомість більше залежностей (clap, serde, YAML, scrypt, HTTP-стек), крос-збірка потребує musl-target і лінкера (`cross`/`cargo-zigbuild`), а цикли компіляції довші. Перевага Rust (бінарник 2–4 МБ, без GC) тут не критична.
- Python відкинуто через повільніший старт і через те, що на Termux його треба ставити окремо.

### Цілі збірки

Усі бінарники статичні (`CGO_ENABLED=0`, `-trimpath -ldflags="-s -w"`, ~6–8 МБ), без libc і рантаймів. На кожну пару ОС/архітектура — окремий файл, усі збираються з однієї машини.

| Ціль | Де працює |
| --- | --- |
| `linux/amd64` | ПК, сервери, WSL |
| `linux/arm64` | ARM-сервери, Raspberry Pi 4/5, Termux на сучасних телефонах |
| `linux/arm` (`GOARM=7`) | старі 32-бітні телефони й Pi (за потреби) |
| `darwin/arm64`, `darwin/amd64` | macOS |
| `windows/amd64`, `windows/arm64` | Windows |

Від чого baton залежить під час роботи (вибір дизайну, а не вимога Go):
- `git`, для branch/commit/staleness; без нього все працює, просто без git-метаданих;
- `systemctl --user` / `launchctl` / `schtasks` — для `baton service`;
- засіб відкриття браузера; якщо його немає, baton лише друкує URL.

### Нюанси платформ

**Termux** (статичний `linux/arm64`):
- в Android немає `/etc/passwd` і `/etc/resolv.conf`, тому домашній каталог беремо з `$HOME`, а не через `os/user`; DNS потрібен лише `baton update`: статичний Go-резолвер без `/etc/resolv.conf` звертається до `127.0.0.1:53` і падає, тому на Termux baton бере nameserver з `$PREFIX/etc/resolv.conf` (власний `net.Resolver`) — перевірити на телефоні;
- `$PREFIX` беремо зі змінних оточення, `/usr` не хардкодимо;
- браузер відкриваємо через `termux-open-url`; сервісу немає, тільки `--background`.

**macOS:** бінарник без підпису, завантажений браузером, блокує Gatekeeper. Через `curl` в `install.sh` проблеми немає. Підпис і нотаризація — пізніше.

**Windows** (окремі файли `*_windows.go`):

| Частина | Рішення |
| --- | --- |
| `save/show/tasks/tips`, дашборд, auth | без змін |
| `pickup` | `exec` немає: агент стартує як дочірній процес у потрібній теці з успадкованим stdio, baton чекає й повертає його код |
| Пошук агента | `exec.LookPath` з `PATHEXT`. npm ставить `claude.cmd`, а Go 1.22+ суворо екранує аргументи для `.cmd/.bat` — перевірити лапки промпта на реальному `claude.cmd` |
| `service` | Task Scheduler (`schtasks /sc onlogon`), не Windows-служба |
| `--background/--stop` | відʼєднання через прапорці створення процесу, зупинка за PID через `Process.Kill` |
| Браузер | `rundll32 url.dll,FileProtocolHandler <url>` |
| Запис файлів | `rename` повторюється з паузою, якщо файл тримає Syncthing, антивірус чи редактор |
| `0600` | на Windows не діє; профіль закритий ACL-ами — описати це в README |
| Конфіг | `%USERPROFILE%\.config\baton\`, як і на інших ОС (плюс `BATON_CONFIG_DIR`) |
| SmartScreen | непідписаний `.exe` з браузера отримує попередження; підпис — пізніше |

Скіли Claude Code на Windows виконуються через Git Bash, тож виклики `baton ...` у них ті самі.

**Усі ОС:** markdown читається і з `\r\n`, а пишеться завжди з `\n`. Шляхи будуються через `filepath`, а в `paths` дозволені шляхи Windows (`C:\...`).

## Файли

```text
~/.config/baton/config     # усе про baton
~/.config/baton/paths      # key=path, по проєкту на рядок; per-machine, не синхронізується
~/.local/state/baton/      # не конфіг: password (0600), sessions, baton.pid, baton.log (журналу подій tips немає)
<root>/                    # за замовчуванням ~/.local/share/baton; див. «Структура root»
```

Оверрайди для тестів і нестандартних установок: `BATON_CONFIG_DIR` (тека з `config` і `paths`), `BATON_STATE`, `BATON_ROOT`. На Windows ті самі шляхи відносно `%USERPROFILE%`.

Приклад `config`:

```text
root = ~/Sync/handoffs
keep = 10
dashboard.host = 127.0.0.1
dashboard.port = 8765
dashboard.public_url =
dashboard.auth = off
dashboard.auth.idle = 7d
dashboard.auth.max = 30d
agent.default = claude
agent.claude = claude "/pickup {task}"
agent.opencode = opencode --prompt "/pickup {task}"   # прапорці opencode перевірити
```

Усі налаштування дашборда (включно з auth) мають префікс `dashboard.`. Невідомий ключ — попередження, а не помилка.

Шаблон агента розбивається на аргументи за пробілами з урахуванням лапок `"..."` і `'...'`. Зворотний слеш не є escape-символом, щоб шляхи Windows (`C:\tools\x.exe`) працювали як є. Плейсхолдери підставляються в кожен аргумент окремо, після чого програма запускається без shell, тож ін'єкцій немає. Плейсхолдери: `{task}` → `@name` або порожньо (кінцеві пробіли аргументу обрізаються: `"/pickup "` → `"/pickup"`), `{project}`, `{dir}` (тека проєкту), `{root}`. Кнопки агентів на дашборді йдуть у порядку `agent.*` у config.

## Структура root

Усе, що стосується проєкту, лежить в одній теці. Службових префіксів `_` немає. Глобальні tips лежать у `global/` з тією ж структурою:

```text
<root>/
├── <project>/
│   ├── tasks/<task>.md               # останній handoff задачі
│   ├── archive/<task>.md             # завершені задачі
│   ├── history/<task>/<created>.md   # попередні handoff-и (keep-N)
│   └── tips/<id>.md                  # tips проєкту
└── global/
    └── tips/<id>.md                  # глобальні tips
```

- **Переваги.** Проєкт — одна тека: перейменування, видалення, бекап чи перенесення робляться однією операцією. Syncthing може шарити окремий проєкт. Усередині проєкту немає зарезервованих імен: задачі лежать у `tasks/`, тож ні з чим не перетинаються. Для коду `global` — такий самий «проєкт», тож обхід tips один.
- **Опису проєкту немає.** Агенту він нічого не додає: контекст проєкту є в README і CLAUDE.md репозиторію.
- **Зарезервоване ім'я одне: `global`** на рівні root. Для теки з такою назвою baton видає помилку з проханням перейменувати теку.
- **Конфлікти Syncthing.** Файли `*.sync-conflict-*.md` у root не губляться: `show`, `tasks` і дашборд показують їх як попередження.
- **Запис атомарний:** прихований тимчасовий файл `.<name>.tmp` у тій самій теці + `rename`. Зміни в одному проєкті (save з ротацією history, done, rename) серіалізуються lock-файлом у `~/.local/state/baton/` (flock / LockFileEx), щоб дві сесії на одній машині не зіпсували history. Між машинами блокувань немає — для цього є виявлення конфліктів Syncthing.

### Ключ проєкту

Ключ — завжди назва теки проєкту, приведена до нижнього регістру, де все, крім `a-z0-9._-`, замінено на `-`. Ні `paths`, ні git на ключ не впливають. Наслідки, які приймаємо свідомо:

- дві різні теки з однаковою назвою (`client-a/api`, `client-b/api`) — це один проєкт; розвести їх можна лише перейменуванням теки;
- дві копії одного репо з однаковою назвою — це той самий проєкт, як і має бути;
- перейменована тека — новий проєкт; старий ключ переносять `mv <root>/<old> <root>/<new>`.

Конфліктів, питань «той самий проєкт?» та станів на кшталт UNBOUND немає.

## Формат handoff

Frontmatter містить лише те, що не залежить від машини і потрібне для роботи:

```yaml
---
title: "Refresh token flow"
created: 2026-10-01T10:15:00+03:00   # RFC3339 зі зміщенням
branch: main          # лише якщо є git-репозиторій
commit: a1b2c3d       # лише якщо є git-репозиторій
from: auth-rewrite    # лише для fork: батьківська задача
---
```

| Поле | Чому |
| --- | --- |
| `title` | заголовок задачі в списках і на дашборді |
| `created` | вік, staleness, імена файлів history |
| `branch`, `commit` | staleness; лише якщо проєкт — git-репозиторій |
| `from` | зв'язок fork і parent |

Свідомо немає: абсолютних шляхів і хоста (шлях береться з `paths`), імені задачі (це ім'я файлу), id сесії агента (формат не прив'язаний до агента), вкладених репо (git-дані — з репозиторію самого проєкту).

Невідомі поля ігноруються. Frontmatter пише лише `baton save`; агент передає тіло і `--title`. Імена файлів у `history/` — `created` в UTC (`20261001T071500Z.md`), щоб порядок не залежав від часового поясу машини.

Шаблон секцій (Goal, State, Decisions, Key context, Gotchas, User preferences, Next steps, Verify) живе в бінарнику: `baton template` друкує його для скілів, `baton save` попереджає про відсутні секції. Так скіли для різних агентів короткі й генеруються з одного джерела.

Frontmatter парситься власним мінімальним парсером (плоскі `key: value`, рядки в лапках, списки `[a, b]` для tips), без YAML-бібліотеки.

Frontmatter tip-а: `title`, `when` (коли застосовний), `keywords`, `cites` (`файл@commit`), `origin` (`session`/`failure`/`web`), `source` (`project`, `commit`, `date`), `status` (`active`/`verified`/`refuted`/`superseded`), `env` (`termux`, `darwin`, ...). Тіло: `Tip:`, `Why:`, `Verify:`.

## CLI

Каталог проєкту — cwd або `--dir`. Усі команди читання мають `--json` (дашборд, тести, інтеграції). Агенти запускають команди в корені проєкту, а скіл велить викликати `baton` без `cd` перед ним. Команди-«звіти» (`show`, `tasks`, `stale`, `tips search`) повертають 0 і на станах на кшталт «нема handoff», бо стан пишуть у вивід. Ненульовий код буває лише при реальній помилці, тому `!`-ін'єкції Claude не зриваються.

```text
baton save [@task] [--from PARENT] [--title T]   # тіло md з stdin; frontmatter, git, ротація history, keep-N, paths — робить baton
baton show [@task|FILE]                          # handoff + staleness + forks + стан (CHOOSE TASK / NO TASK / ARCHIVED / NO HANDOFF)
baton tasks | done TASK | restore TASK | rename OLD NEW | history TASK [N] | stale FILE
baton pickup [PROJECT] [@task] [--agent X] [--print]   # тека з paths (або cwd з тим самим ключем) + запуск агента (default або X)
baton path set KEY DIR | list | prune
baton tips search [--error] WORDS | show ID | new [--global] | verified ID | refuted ID WHY | supersede OLD NEW | move ID global|project | list
                                                 # tips new: тіло tip-а з stdin, як у save
baton dashboard [--port] [--host] [--no-open] [--background|--stop] [--json]
baton service install|status|restart|uninstall [--dry-run]
baton auth status|on|off|password|logout-all
baton integrate claude|opencode [--uninstall]    # ставить вбудовані скіли/команди та блок інструкцій
baton template                                   # шаблон handoff для скілів
baton update [--check]                           # оновлення з GitHub Releases
baton version
```

Агент ніколи не пише файли в root сам, а передає тіло в `baton save` через stdin. Тому скілам потрібен лише дозвіл `Bash(baton *)`.

Потоки скілів:
- `/handoff [@task]`: `baton tasks` (вибір задачі) → `baton show @task` (попередній handoff, з якого переносяться відкриті рішення й gotchas) → `baton template` → агент складає текст → `baton save @task --title ...`. Tips: `baton tips search` на дублікати → `baton tips new|supersede`.
- `/handoff @task done`: за потреби фінальний `save`, потім `baton done task`.
- `/handoff fork`: `baton save @parent` (чекає на fork) → `baton save @fork --from parent`.
- `/pickup [@task]`: `baton show [@task]`; на станах CHOOSE TASK / NO TASK / ARCHIVED агент питає користувача й повторює `show` або `restore`.

## Скіли та інтеграції

- Каталог скіла містить лише `SKILL.md` (і, за потреби, довідкові `.md`). Жодних скриптів чи `${CLAUDE_SKILL_DIR}/...`: скрізь викликається `baton` із `PATH`.
- Claude Code: `allowed-tools: Bash(baton *)`; `!`-блоки (`baton show ...`) лише прискорюють старт, інструкції працюють і без них.
- opencode: команди `handoff`/`pickup`/`tips` з тими самими викликами `baton`.
- **Tips без хуків.** Жодних хуків ні для Claude, ні для opencode. Tips знаходяться двома шляхами: (1) `baton show` під час pickup додає заголовки tips, що збігаються з назвою задачі, title і секцією Gotchas (лише заголовки, повний текст — `baton tips show ID`); (2) блок у CLAUDE.md / AGENTS.md і опис скіла `/tips` велять агенту шукати `baton tips search` перед дебагом і при помилках. `integrate` не чіпає `settings.json`.
- `baton integrate claude|opencode` пише ці файли з копій, вбудованих у бінарник; `--uninstall` прибирає їх.

## Структура репо

```text
cmd/baton/            # main, розбір команд
internal/config/      # config + paths, env-оверрайди
internal/store/       # project key, frontmatter, tasks/history/archive/rename/forks
internal/gitinfo/     # branch, commit, staleness
internal/tips/        # пошук (скоринг, словоформи), запис, статуси
internal/agent/       # шаблони, резолв агента, pickup
internal/dashboard/   # HTTP API, auth (scrypt, сесії, lockout), Host/Origin/token, web/ (embed)
internal/service/     # systemd --user, launchd, --background
integrations/claude/  # SKILL.md handoff/pickup/tips, блок CLAUDE.md
integrations/opencode/# команди handoff/pickup/tips
```

## Етапи

Кожен етап закінчується тестами, які проходять.

0. **Скелет.** `go mod`, CI з тестами на ubuntu, macos і windows та крос-збіркою всіх цілей, парсери `config`/`paths` (з CRLF і шляхами Windows), `baton version`.
1. **Store.** `save/show/tasks/done/restore/rename/history`, keep-N, forks, ключ проєкту з назви теки, lock, атомарний запис, виявлення sync-conflict, `template`.
2. **Git.** Метадані й staleness-звіт: нові коміти, rebased/missing commit, dirty.
3. **Paths + pickup.** Автозапис paths (остання тека перемагає), `path set|list|prune`, cwd з тим самим ключем у `pickup`, шаблони агентів, `pickup --print`. Запуск агента: `exec` на Unix, дочірній процес на Windows, перевірка з `claude.cmd`.
4. **Tips.** Пошук (скоринг за keywords/title/when/тілом, прості словоформи, режим `--error`), запис, статуси, supersede, move, tips у `show`. Без хуків.
5. **Dashboard.** HTTP API поверх `internal/store` і `internal/tips`, сторінки входу й застосунку (embed). Кнопка на кожного агента з config («pickup claude», «pickup opencode») копіює `baton pickup <project> @task --agent <name>`. Auth (scrypt, сесії, lockout), перевірка Host/Origin проти DNS-rebinding, CSRF-токен для змін.
6. **Service.** systemd/launchd/Task Scheduler, `--background/--stop` для всіх ОС, відкриття браузера (xdg-open, open, termux-open-url, wslview, rundll32).
7. **Інтеграції.** Скіли Claude переписуються під `baton`. `!`-блоки лишаються лише як прискорення, бо інструкції мають працювати й без них, як в opencode. Команди opencode. `baton integrate`.
8. **Реліз і оновлення.** goreleaser у GitHub Actions: бінарники всіх цілей і `checksums.txt` (SHA256). `install.sh` (Linux, macOS, Termux) і `install.ps1` (Windows) завантажують бінарник, перевіряють суму і за бажанням викликають `baton integrate`. `baton update`: знаходить реліз для своєї ОС/архітектури, перевіряє SHA256, атомарно замінює себе (на Windows — через перейменування запущеного `.exe`), оновлює файли `integrate` і перезапускає сервіс. Також `go install`.
9. **Документація.** README як специфікація, CHANGELOG, CLAUDE.md.

## Тести

- `go test ./...`: unit-тести для парсерів, скорингу tips і frontmatter.
- Лінт у CI: `gofmt`, `go vet`, `golangci-lint`. Мінімальна версія Go — 1.22.
- e2e через `testscript` (`rogpeppe/go-internal`) у тимчасових `BATON_ROOT`/`BATON_STATE`/`BATON_CONFIG_DIR`.
- Дашборд: HTTP-тести API та auth, а для вбудованого JS — `node --check`.

## Перевірити перед відповідними етапами

- Чи пропускає `allowed-tools: Bash(baton *)` у Claude Code виклик `baton save ... <<'EOF'` без запиту дозволу; якщо ні — альтернативний спосіб передати тіло (етап 7).
- opencode: каталог команд, прапорець для початкового промпта, куди писати блок інструкцій (AGENTS.md) (етап 7).
