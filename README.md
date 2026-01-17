# HGN Access Bot - Telegram Bot для записи на хиджаму

Telegram бот для записи на процедуры хиджамы с использованием Supabase.

## Возможности

- ✅ Запись на процедуры с выбором даты, времени и мастера
- ✅ Сбор контактных данных (имя и телефон)
- ✅ Просмотр своих записей
- ✅ Отмена записи (за 2 часа до процедуры)
- ✅ Админ-панель для управления мастерами
- ✅ Интеграция с Supabase

## Требования

- Go 1.21+
- Supabase аккаунт
- Telegram Bot Token

## Установка

### 1. Клонируйте репозиторий
```bash
git clone https://github.com/YOUR_USERNAME/hidjama-telegram-bot.git
cd hidjama-telegram-bot
```

### 2. Установите зависимости
```bash
go mod download
```

### 3. Настройте Supabase

1. Создайте проект на [supabase.com](https://supabase.com)
2. Выполните SQL из файла `schema.sql` в SQL Editor
3. Добавьте колонки для контактов:
```sql
ALTER TABLE slots ADD COLUMN client_name TEXT;
ALTER TABLE slots ADD COLUMN client_phone TEXT;
ALTER TABLE slots ADD COLUMN package_name TEXT;
```

### 4. Создайте `.env` файл
```bash
cp .env.example .env
```

Заполните `.env`:
```env
BOT_TOKEN=ваш_токен_telegram_бота
SUPABASE_URL=https://ваш-проект.supabase.co
SUPABASE_KEY=ваш_supabase_anon_key
ADMINS=ваш_telegram_id
DEV_PASSWORD=4116
```

### 5. Запустите бота
```bash
go run .
```

## Docker

Запуск через Docker:
```bash
docker-compose up -d
```

## Структура базы данных

### Таблица `masters`
- `id` - уникальный идентификатор
- `name` - имя мастера
- `code` - код доступа
- `contact` - телефон
- `gender` - пол (male/female)
- `active` - активен ли мастер

### Таблица `packages`
- `key` - ключ процедуры
- `name` - название
- `description` - описание
- `price` - стоимость

### Таблица `slots`
- `id` - ID записи
- `date` - дата (YYYY-MM-DD)
- `time` - время (HH:MM)
- `gender` - пол клиента
- `master_name` - имя мастера
- `status` - статус (free/booked/cancelled/completed/no_show)
- `user_id` - Telegram ID
- `username` - Telegram username
- `client_name` - имя клиента
- `client_phone` - телефон клиента
- `package_name` - название процедуры
- `booked_at` - время бронирования
- `source` - источник (bot/nfc/qr/link)

## Логика работы

### Бронирование
1. Выбор процедуры
2. Выбор пола
3. Подтверждение возраста 18+
4. Ввод имени
5. Ввод телефона
6. Выбор даты
7. Выбор времени
8. Выбор мастера
9. Подтверждение записи

### Отмена записи
- Возможна только за 2 часа до процедуры (T-2)
- После отмены слот становится свободным

## Лицензия

MIT
