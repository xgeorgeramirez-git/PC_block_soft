# Сервер связи PC Block на вашем VPS

Сервер связи передаёт сообщения между программой PC Block на компьютере и приложением на вашем
телефоне. Он ставится на ваш VPS рядом с n8n отдельной папкой `/opt/pcblock-relay` и работает по адресу
`https://pcblock.atlas404.ru`.

Что важно знать:

- n8n и Traefik эти команды **не трогают**: их папки `/opt/n8n` и `/opt/traefik` не меняются.
- Сертификат HTTPS сервер получает через ваш Traefik, как n8n. Новых портов наружу не открывается.
- Сервер занимает не больше 64 МБ памяти.
- Команды вводятся в окне SSH, как вы делали при проверке сервера. Каждую команду копируйте
  целиком, вставляйте и нажимайте Enter. Если система спросит пароль, введите пароль от VPS
  (символы при вводе не видны, это нормально).

## Установка (один раз, около 5 минут)

**Шаг 1.** Создаёт папку для сервера связи.

```bash
sudo mkdir -p /opt/pcblock-relay
```

**Шаг 2.** Скачивает программу сервера с GitHub в папку `/opt/pcblock-relay/src`.

```bash
sudo git clone --depth 1 --branch feature/m7-remote https://github.com/xgeorgeramirez-git/PC_block_soft.git /opt/pcblock-relay/src
```

Если ответ `git: command not found`, сначала выполните эту команду (ставит программу git), потом снова шаг 2:

```bash
sudo apt-get install -y git
```

**Шаг 3.** Кладёт файл настроек сервера в его папку.

```bash
sudo cp /opt/pcblock-relay/src/deploy/relay/compose.yaml /opt/pcblock-relay/compose.yaml
```

**Шаг 4.** Собирает и запускает сервер связи (первый раз 1 или 2 минуты).

```bash
cd /opt/pcblock-relay && sudo docker compose up -d --build
```

В конце должны быть строки со словами `Started` или `Running`.

## Проверка

**Проверка 1.** Показывает, работает ли сервер связи.

```bash
cd /opt/pcblock-relay && sudo docker compose ps
```

Ожидание: строка `relay` и в столбце `STATUS` слова `Up` и `(healthy)`. Сразу после запуска может
быть `(health: starting)`: подождите полминуты и повторите.

**Проверка 2.** Открывает адрес сервера через интернет, как это будет делать телефон.

```bash
curl -sS https://pcblock.atlas404.ru/healthz
```

Ожидание: строка, которая начинается с `{"ok":true`. В первую минуту после установки Traefik
получает сертификат, поэтому возможна ошибка про сертификат: подождите минуту и повторите.

**Проверка 3.** Показывает все контейнеры на сервере: n8n должен работать, как раньше.

```bash
sudo docker ps --format 'table {{.Names}}\t{{.Status}}'
```

Ожидание: у строк с `n8n` (и у `traefik`) в столбце `STATUS` слово `Up`.

## Если что-то не так: журнал

Показывает последние 100 строк журнала сервера связи.

```bash
cd /opt/pcblock-relay && sudo docker compose logs --tail 100 relay
```

Показывает последние 50 строк журнала Traefik (там видно, получен ли сертификат). Это только чтение, ничего не меняет.

```bash
cd /opt/traefik && sudo docker compose logs --tail 50
```

## Обновление (когда я попрошу)

**Шаг 1.** Скачивает новую версию программы сервера.

```bash
sudo git -C /opt/pcblock-relay/src pull --ff-only
```

**Шаг 2.** Обновляет файл настроек сервера (если он менялся).

```bash
sudo cp /opt/pcblock-relay/src/deploy/relay/compose.yaml /opt/pcblock-relay/compose.yaml
```

**Шаг 3.** Пересобирает и перезапускает сервер связи. Привязка телефона сохраняется.

```bash
cd /opt/pcblock-relay && sudo docker compose up -d --build
```

После обновления выполните проверки 1 и 2.

## Переход на основную версию (после выпуска 0.8.0, когда я попрошу)

Сейчас сервер установлен из предварительной версии. После выпуска его нужно один раз переключить
на основную. Привязка телефона сохраняется: она хранится отдельно от папки с программой.

**Шаг 1.** Удаляет скачанную предварительную версию программы (только папку `src`).

```bash
sudo rm -rf /opt/pcblock-relay/src
```

**Шаг 2.** Скачивает основную версию программы.

```bash
sudo git clone --depth 1 --branch main https://github.com/xgeorgeramirez-git/PC_block_soft.git /opt/pcblock-relay/src
```

**Шаг 3.** Обновляет файл настроек сервера.

```bash
sudo cp /opt/pcblock-relay/src/deploy/relay/compose.yaml /opt/pcblock-relay/compose.yaml
```

**Шаг 4.** Пересобирает и перезапускает сервер связи.

```bash
cd /opt/pcblock-relay && sudo docker compose up -d --build
```

Дальше обновление делается так же, как в разделе «Обновление».

## Удаление

Останавливает и убирает сервер связи. Привязка телефона остаётся на диске, если захотите вернуть сервер.

```bash
cd /opt/pcblock-relay && sudo docker compose down
```

Полное удаление вместе с данными (телефон потом придётся подключать заново). Сначала эта команда:

```bash
cd /opt/pcblock-relay && sudo docker compose down -v --rmi local
```

Потом эта (удаляет папку сервера связи):

```bash
sudo rm -rf /opt/pcblock-relay
```

## Что прислать мне

После установки скопируйте и пришлите мне то, что вывели эти три команды (весь текст из окна SSH):

```bash
cd /opt/pcblock-relay && sudo docker compose ps
```

```bash
curl -sS https://pcblock.atlas404.ru/healthz
```

```bash
sudo docker ps --format 'table {{.Names}}\t{{.Status}}'
```

Если на каком-то шаге была ошибка, пришлите её текст и вывод этой команды:

```bash
cd /opt/pcblock-relay && sudo docker compose logs --tail 100 relay
```
