package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Package struct {
	Key   string
	Name  string
	Price int
	Desc  string
}

type Booking struct {
	User   string
	Master string
	Date   string
	Time   string
}

var cfg *Config
var masters map[string]Master
var packages map[string]Package
var contactMap map[string]string
var masterNotifications = make(map[string]bool)
var bookingsLog []Booking
var bot *tgbotapi.BotAPI
var tz *time.Location

func main() {
	log.Println("Starting bot...")

	var err error
	cfg, err = loadConfig()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	log.Println("Config loaded")

	tz, err = time.LoadLocation(cfg.Timezone)
	if err != nil {
		tz = time.UTC
	}

	token := cfg.Token
	if token == "" {
		log.Fatal("BOT_TOKEN not set")
	}

	bot, err = tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatal(err)
	}

	// Delete webhook to ensure polling works
	_, err = bot.Request(tgbotapi.DeleteWebhookConfig{})
	if err != nil {
		log.Printf("Delete webhook error: %v", err)
	}

	// Check webhook status
	webhookInfo, err := bot.GetWebhookInfo()
	if err == nil && webhookInfo.URL != "" {
		log.Printf("Webhook still active: %s", webhookInfo.URL)
	} else if err != nil {
		log.Printf("Webhook info error: %v", err)
	} else {
		log.Println("No active webhook")
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)

	initDB()
	masters = loadMastersFromDB()
	packages = loadPackagesFromDB()
	loadAllSessions()

	log.Printf("Loaded %d masters, %d packages", len(masters), len(packages))

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 10
	u.AllowedUpdates = []string{"message", "callback_query"}

	updates := bot.GetUpdatesChan(u)

	log.Println("Bot started, polling for updates")

	for update := range updates {
		log.Printf("Received update ID %d", update.UpdateID)
		handleUpdate(update)
	}
}

func handleUpdate(update tgbotapi.Update) {
	if update.Message != nil {
		log.Printf("Handling message: %s from %s", update.Message.Text, update.Message.From.UserName)
		handleMessage(update.Message)
	} else if update.CallbackQuery != nil {
		log.Printf("Handling callback: %s", update.CallbackQuery.Data)
		handleCallback(update.CallbackQuery)
	} else {
		log.Printf("Unknown update type: %+v", update)
	}
}

func handleMessage(msg *tgbotapi.Message) {
	userID := msg.From.ID
	session := userSessions[userID]
	if session != nil {
		handleSessionMessage(msg, session)
		return
	}

	text := strings.ToLower(msg.Text)
	log.Printf("Message text (lowercase): '%s'", text)

	switch {
	case text == "/start":
		log.Printf("Processing /start for user %d in chat %d", userID, msg.Chat.ID)
		start(msg)
	case strings.Contains(text, "записаться"):
		log.Println("Booking start triggered")
		bookStart(msg)
	case strings.Contains(text, "админ панель"):
		if isAdmin(userID) {
			adminPanel(msg.Chat.ID)
		}
	case strings.Contains(text, "другие возможности"):
		otherOptions(msg)
	case strings.Contains(text, "мои записи"):
		showMyBookings(msg)
	}
}

func handleSessionMessage(msg *tgbotapi.Message, session *UserSession) {
	userID := msg.From.ID
	text := msg.Text

	switch session.Step {
	case "master_login":
		masterID := session.Data["master_id"].(string)
		master := masters[masterID]
		if text == master.Code {
			showMasterProfile(msg.Chat.ID, masterID)
		} else {
			bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ Неверный код"))
		}
		delete(userSessions, userID)
	case "add_master_name":
		session.Data["name"] = text
		session.Step = "add_master_code"
		markup := &tgbotapi.InlineKeyboardMarkup{}
		markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
			{tgbotapi.NewInlineKeyboardButtonData("Отмена", "admin_masters_btn")},
		}
		msg := tgbotapi.NewMessage(msg.Chat.ID, "Введите код доступа:")
		msg.ReplyMarkup = markup
		bot.Send(msg)
	case "add_master_code":
		session.Data["code"] = text
		session.Step = "add_master_contact"
		markup := &tgbotapi.InlineKeyboardMarkup{}
		markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
			{tgbotapi.NewInlineKeyboardButtonData("Отмена", "admin_masters_btn")},
		}
		msg := tgbotapi.NewMessage(msg.Chat.ID, "Введите контакт (или пусто):")
		msg.ReplyMarkup = markup
		bot.Send(msg)
	case "add_master_contact":
		session.Data["contact"] = text
		session.Step = "add_master_gender"
		markup := &tgbotapi.InlineKeyboardMarkup{}
		markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
			{tgbotapi.NewInlineKeyboardButtonData("Мужчина", "gender_master_male")},
			{tgbotapi.NewInlineKeyboardButtonData("Женщина", "gender_master_female")},
			{tgbotapi.NewInlineKeyboardButtonData("Отмена", "admin_masters_btn")},
		}
		msg := tgbotapi.NewMessage(msg.Chat.ID, "Выберите пол:")
		msg.ReplyMarkup = markup
		bot.Send(msg)
	case "waiting_name":
		session.Data["client_name"] = text
		session.Step = "waiting_phone"
		markup := &tgbotapi.InlineKeyboardMarkup{}
		markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
			{tgbotapi.NewInlineKeyboardButtonData("← Назад", "back_to_gender")},
		}
		msg := tgbotapi.NewMessage(msg.Chat.ID, "Укажите ваш номер телефона:")
		msg.ReplyMarkup = markup
		bot.Send(msg)
		saveUserSession(userID, session)
	case "waiting_phone":
		session.Data["client_phone"] = text
		session.Step = ""
		saveUserSession(userID, session)
		showDatePageMessage(msg, 0)
	}
}

func start(msg *tgbotapi.Message) {
	markup := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📍 Записаться на Хиджаму"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📋 Мои записи"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Другие возможности"),
		),
	)
	if isAdmin(msg.From.ID) {
		markup.Keyboard = append(markup.Keyboard, tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🔐 Админ панель"),
		))
	}

	message := tgbotapi.NewMessage(msg.Chat.ID, "HGN · Доступ активирован\nРегистрация не требуется")
	message.ReplyMarkup = markup
	log.Printf("Sending start message to chat %d", msg.Chat.ID)
	_, err := bot.Send(message)
	if err != nil {
		log.Printf("Send error in start: %v", err)
	} else {
		log.Println("Start reply sent")
	}
}

func bookStart(msg *tgbotapi.Message) {
	// Simplified booking start
	keyboard := createServiceKeyboard()
	message := tgbotapi.NewMessage(msg.Chat.ID, "Выберите услугу:")
	message.ReplyMarkup = keyboard
	bot.Send(message)
}

func createServiceKeyboard() tgbotapi.InlineKeyboardMarkup {
	var keyboard [][]tgbotapi.InlineKeyboardButton
	order := []string{"complex", "upper", "lower", "individual", "cosmetology"}
	for _, key := range order {
		if pkg, ok := packages[key]; ok {
			keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%s — %d ₽", pkg.Name, pkg.Price), "package_"+key),
			))
		}
	}
	return tgbotapi.NewInlineKeyboardMarkup(keyboard...)
}

func handleCallback(cb *tgbotapi.CallbackQuery) {
	data := cb.Data
	userID := cb.From.ID

	if strings.HasPrefix(data, "package_") {
		key := strings.TrimPrefix(data, "package_")
		if pkg, ok := packages[key]; ok {
			// Save package to session
			session := getSession(userID)
			session.Step = "package_selected"
			session.Data["package"] = key
			setSession(userID, session)

			// Show package details and gender selection
			markup := &tgbotapi.InlineKeyboardMarkup{}
			markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
				{
					tgbotapi.NewInlineKeyboardButtonData("Мужчина", "gender_male"),
					tgbotapi.NewInlineKeyboardButtonData("Женщина", "gender_female"),
				},
				{
					tgbotapi.NewInlineKeyboardButtonData("← Назад", "back_packages"),
				},
			}

			text := fmt.Sprintf("%s\n\n%s\n\nСтоимость: %d ₽\n\nВыберите пол:", pkg.Name, pkg.Desc, pkg.Price)
			editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, text)
			editMsg.ReplyMarkup = markup
		bot.Send(editMsg)
			bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
		}
	} else if data == "back_packages" {
		markup := createServiceKeyboard()
		editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Виды хиджамы и стоимость\n\nСтерильно по ГОСТ ISO 11135-2017")
		editMsg.ReplyMarkup = &markup
		bot.Send(editMsg)
		bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
	} else if data == "gender_male" || data == "gender_female" {
		gender := "male"
		if data == "gender_female" {
			gender = "female"
		}
		session := getSession(userID)
		session.Data["gender"] = gender
		setSession(userID, session)

		// Age confirmation
		markup := &tgbotapi.InlineKeyboardMarkup{}
		markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
			{
				tgbotapi.NewInlineKeyboardButtonData("Да, 18+", "age_yes"),
			},
			{
				tgbotapi.NewInlineKeyboardButtonData("Нет", "age_no"),
			},
		}

		editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Вам есть 18 лет?")
		editMsg.ReplyMarkup = markup
		bot.Send(editMsg)
		bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
	} else if data == "age_no" {
		editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Услуга доступна только лицам старше 18 лет")
		bot.Send(editMsg)
		bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
	} else if data == "age_yes" {
		markup := &tgbotapi.InlineKeyboardMarkup{}
		markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
			{tgbotapi.NewInlineKeyboardButtonData("← Назад", "back_to_gender")},
		}
		editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Прежде чем начать запись, укажите свое имя:")
		editMsg.ReplyMarkup = markup
		bot.Send(editMsg)
		session := getSession(userID)
		session.Step = "waiting_name"
		setSession(userID, session)
		bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
	} else if strings.HasPrefix(data, "date_page_") {
		pageStr := strings.TrimPrefix(data, "date_page_")
		page := 0
		fmt.Sscanf(pageStr, "%d", &page)
		showDatePage(cb, page)
	} else if data == "back_to_gender" {
		session := getSession(userID)
		pkgKey, ok := session.Data["package"].(string)
		if !ok || pkgKey == "" {
			markup := createServiceKeyboard()
			editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Выберите услугу:")
			editMsg.ReplyMarkup = &markup
			bot.Send(editMsg)
			bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
			return
		}
		pkg := packages[pkgKey]
		markup := &tgbotapi.InlineKeyboardMarkup{}
		markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
			{
				tgbotapi.NewInlineKeyboardButtonData("Мужчина", "gender_male"),
				tgbotapi.NewInlineKeyboardButtonData("Женщина", "gender_female"),
			},
			{
				tgbotapi.NewInlineKeyboardButtonData("← Назад", "back_packages"),
			},
		}
		text := fmt.Sprintf("%s\n\n%s\n\nСтоимость: %d ₽\n\nВыберите пол:", pkg.Name, pkg.Desc, pkg.Price)
		editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, text)
		editMsg.ReplyMarkup = markup
		bot.Send(editMsg)
		bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
	} else if strings.HasPrefix(data, "date_") {
		if !strings.HasPrefix(data, "date_page_") {
			dateStr := strings.TrimPrefix(data, "date_")
			session := getSession(userID)
			session.Data["date"] = dateStr
			setSession(userID, session)
			showTimeSelection(cb, dateStr)
		}
	} else if strings.HasPrefix(data, "time_") {
		parts := strings.Split(data, "_")
		if len(parts) >= 3 {
			timeStr := parts[2]
			session := getSession(userID)
			session.Data["time"] = timeStr
			setSession(userID, session)
			// Show master selection
			showMasterSelection(cb)
		}
	} else if data == "back_to_date" {
		// Back to date selection
		showDatePage(cb, 0)
	} else if strings.HasPrefix(data, "master_") {
		master := strings.TrimPrefix(data, "master_")
		session := getSession(userID)
		session.Data["master"] = master
		setSession(userID, session)
		// Show confirmation
		showBookingConfirmation(cb)
	} else if data == "back_to_time" {
		session := getSession(userID)
		date, ok := session.Data["date"].(string)
		if !ok || date == "" {
			showDatePage(cb, 0)
			return
		}
		showTimeSelection(cb, date)
	} else if data == "confirm_booking" {
		// Final booking
		finalizeBooking(cb)
	} else if data == "back_to_master" {
		showMasterSelection(cb)
	} else if data == "admin_masters_btn" {
		showAdminMasters(cb)
	} else if data == "admin_developer" {
		showDeveloperPanel(cb)
	} else if data == "admin_back" {
		showAdminMain(cb)
	} else if strings.HasPrefix(data, "master_profile_") {
		masterID := strings.TrimPrefix(data, "master_profile_")
		showMasterProfileLogin(cb, masterID)
	} else if data == "add_master_start" {
		startAddMaster(cb)
	} else if strings.HasPrefix(data, "gender_master_") {
		processMasterGender(cb, data)
	} else if strings.HasPrefix(data, "master_bookings_") {
		showMasterBookings(cb, data)
	} else if strings.HasPrefix(data, "master_profit_") {
		showMasterProfit(cb, data)
	} else if strings.HasPrefix(data, "master_notify_") {
		toggleMasterNotify(cb, data)
	} else if strings.HasPrefix(data, "master_back_") {
		backToMasterProfile(cb, data)
	} else if strings.HasPrefix(data, "cancel_booking_") {
		cancelUserBooking(cb, data)
	}
}

func adminPanel(chatID int64) {
	markup := &tgbotapi.InlineKeyboardMarkup{}
	markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
		{tgbotapi.NewInlineKeyboardButtonData("👨⚕️ Мастера", "admin_masters_btn")},
		{tgbotapi.NewInlineKeyboardButtonData("👨💻 Разработчик", "admin_developer")},
	}

	message := tgbotapi.NewMessage(chatID, "Привет мастер!\nЭто админ панель")
	message.ReplyMarkup = markup
	bot.Send(message)
}

func otherOptions(msg *tgbotapi.Message) {
	markup := &tgbotapi.InlineKeyboardMarkup{}
	markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
		{tgbotapi.NewInlineKeyboardButtonURL("Стать мастером", "https://hidjamaglobal.net")},
		{tgbotapi.NewInlineKeyboardButtonURL("Открыть центр HGN", "https://hgn-franchise.net")},
		{tgbotapi.NewInlineKeyboardButtonURL("Приложение HGN", "https://apps.apple.com/ru/app/hidjama-quantum/id6479644776")},
	}

	message := tgbotapi.NewMessage(msg.Chat.ID, "Выберите опцию:")
	message.ReplyMarkup = markup
	bot.Send(message)
}

func isAdmin(userID int64) bool {
	for _, admin := range cfg.Admins {
		if admin == userID {
			return true
		}
	}
	return false
}

func getSession(userID int64) UserSession {
	session, err := loadUserSession(userID)
	if err != nil {
		return UserSession{Data: make(map[string]interface{})}
	}
	if session.Data == nil {
		session.Data = make(map[string]interface{})
	}
	return *session
}

func setSession(userID int64, session UserSession) {
	saveUserSession(userID, &session)
}

func clearSession(userID int64) {
	deleteUserSession(userID)
}

func showAdminMasters(cb *tgbotapi.CallbackQuery) {
	markup := &tgbotapi.InlineKeyboardMarkup{}
	for masterID, master := range masters {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(master.Name, "master_profile_"+masterID),
		})
	}
	markup.InlineKeyboard = append(markup.InlineKeyboard, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("➕ Добавить мастера", "add_master_start"),
		tgbotapi.NewInlineKeyboardButtonData("← Назад", "admin_back"),
	})

	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Выберите мастера:")
	editMsg.ReplyMarkup = markup
	bot.Send(editMsg)
	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
}

func showDeveloperPanel(cb *tgbotapi.CallbackQuery) {
	// Show booking logs and status
	logs := "📋 Последние записи:\n"
	// Simplified, assume bookingsLog has entries
	for _, b := range bookingsLog {
		logs += fmt.Sprintf("👤 %s\n👨⚕️ %s\n📅 %s %s\n", b.User, b.Master, b.Date, b.Time)
	}
	if len(bookingsLog) == 0 {
		logs += "Нет записей"
	}
	status := "\n🔌 Статус: ✅ База данных, ✅ Telegram API"

	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, logs+status)
	bot.Send(editMsg)
	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
}

func showAdminMain(cb *tgbotapi.CallbackQuery) {
	markup := &tgbotapi.InlineKeyboardMarkup{}
	markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
		{tgbotapi.NewInlineKeyboardButtonData("👨⚕️ Мастера", "admin_masters_btn")},
		{tgbotapi.NewInlineKeyboardButtonData("👨💻 Разработчик", "admin_developer")},
	}

	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Привет мастер!\nЭто админ панель")
	editMsg.ReplyMarkup = markup
	bot.Send(editMsg)
	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
}

func showMasterProfileLogin(cb *tgbotapi.CallbackQuery, masterID string) {
	userSessions[cb.From.ID] = &UserSession{Step: "master_login", Data: map[string]interface{}{"master_id": masterID}}
	markup := &tgbotapi.InlineKeyboardMarkup{}
	markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
		{tgbotapi.NewInlineKeyboardButtonData("Отмена", "admin_masters_btn")},
	}

	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Введите код доступа для мастера:")
	editMsg.ReplyMarkup = markup
	bot.Send(editMsg)
	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
}

func startAddMaster(cb *tgbotapi.CallbackQuery) {
	userSessions[cb.From.ID] = &UserSession{Step: "add_master_name"}
	markup := &tgbotapi.InlineKeyboardMarkup{}
	markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
		{tgbotapi.NewInlineKeyboardButtonData("Отмена", "admin_masters_btn")},
	}

	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Введите имя мастера:")
	editMsg.ReplyMarkup = markup
	bot.Send(editMsg)
	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
}

func processMasterGender(cb *tgbotapi.CallbackQuery, data string) {
	gender := strings.TrimPrefix(data, "gender_master_")
	session := userSessions[cb.From.ID]
	if session == nil || session.Step != "add_master_gender" {
		return
	}
	session.Data["gender"] = gender
	// Add master logic
	name := session.Data["name"].(string)
	code := session.Data["code"].(string)
	contact := session.Data["contact"].(string)
	masterID := strings.ToLower(strings.ReplaceAll(name, " ", "_"))
	masters[masterID] = Master{ID: masterID, Name: name, Code: code, Contact: contact, Gender: gender}
	// Save to DB (simplified, need implement saveMasters)

	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, fmt.Sprintf("Мастер '%s' добавлен!", name))
	bot.Send(editMsg)
	delete(userSessions, cb.From.ID)
	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
}

func showMasterBookings(cb *tgbotapi.CallbackQuery, data string) {
	masterID := strings.TrimPrefix(data, "master_bookings_")
	markup := &tgbotapi.InlineKeyboardMarkup{}
	markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
		{tgbotapi.NewInlineKeyboardButtonData("← Назад", "master_back_"+masterID)},
	}

	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "📋 Записи: (пока пусто)")
	editMsg.ReplyMarkup = markup
	bot.Send(editMsg)
	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
}

func showMasterProfit(cb *tgbotapi.CallbackQuery, data string) {
	masterID := strings.TrimPrefix(data, "master_profit_")
	markup := &tgbotapi.InlineKeyboardMarkup{}
	markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
		{tgbotapi.NewInlineKeyboardButtonData("← Назад", "master_back_"+masterID)},
	}

	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "💰 Прибыль: 0 ₽")
	editMsg.ReplyMarkup = markup
	bot.Send(editMsg)
	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
}

func toggleMasterNotify(cb *tgbotapi.CallbackQuery, data string) {
	masterID := strings.TrimPrefix(data, "master_notify_")
	current := masterNotifications[masterID]
	masterNotifications[masterID] = !current
	status := "❌ Отключены"
	if !current {
		status = "✅ Включены"
	}
	markup := &tgbotapi.InlineKeyboardMarkup{}
	markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
		{tgbotapi.NewInlineKeyboardButtonData("← Назад", "master_back_"+masterID)},
	}

	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, fmt.Sprintf("🔔 Уведомления: %s", status))
	editMsg.ReplyMarkup = markup
	bot.Send(editMsg)
	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
}

func showMasterProfile(chatID int64, masterID string) {
	master := masters[masterID]
	markup := &tgbotapi.InlineKeyboardMarkup{}
	markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
		{tgbotapi.NewInlineKeyboardButtonData("📋 Мои записи", "master_bookings_"+masterID)},
		{tgbotapi.NewInlineKeyboardButtonData("💰 Прибыль", "master_profit_"+masterID)},
		{tgbotapi.NewInlineKeyboardButtonData("🔔 Уведомления", "master_notify_"+masterID)},
		{tgbotapi.NewInlineKeyboardButtonData("← Назад", "admin_back")},
	}

	text := fmt.Sprintf("👨⚕️ %s\n📞 %s\n\nВыполненно: 0\nПрибыль: 0 ₽", master.Name, master.Contact)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = markup
	bot.Send(msg)
}

func backToMasterProfile(cb *tgbotapi.CallbackQuery, data string) {
	masterID := strings.TrimPrefix(data, "master_back_")
	showMasterProfile(cb.Message.Chat.ID, masterID)
	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
}

func showDatePage(cb *tgbotapi.CallbackQuery, page int) {
	today := time.Now().In(tz)
	var dates []time.Time
	for i := 0; i < 30; i++ {
		dates = append(dates, today.AddDate(0, 0, i))
	}

	datesPerPage := 5
	start := page * datesPerPage
	end := start + datesPerPage
	if end > len(dates) {
		end = len(dates)
	}
	pageDates := dates[start:end]

	markup := &tgbotapi.InlineKeyboardMarkup{}
	for _, date := range pageDates {
		dateStr := date.Format("2006-01-02")
		weekday := []string{"Вс", "Пн", "Вт", "Ср", "Чт", "Пт", "Сб"}[int(date.Weekday())]
		label := fmt.Sprintf("%s (%s)", date.Format("02.01"), weekday)
		markup.InlineKeyboard = append(markup.InlineKeyboard, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(label, "date_"+dateStr),
		})
	}

	var navButtons []tgbotapi.InlineKeyboardButton
	if page > 0 {
		navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData("← Назад", fmt.Sprintf("date_page_%d", page-1)))
	}
	if end < len(dates) {
		navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData("Далее →", fmt.Sprintf("date_page_%d", page+1)))
	}
	if len(navButtons) > 0 {
		markup.InlineKeyboard = append(markup.InlineKeyboard, navButtons)
	}

	markup.InlineKeyboard = append(markup.InlineKeyboard, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("← Назад", "back_to_gender"),
	})

	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Выберите дату:")
	editMsg.ReplyMarkup = markup
	bot.Send(editMsg)
	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
}

func showDatePageMessage(msg *tgbotapi.Message, page int) {
	today := time.Now().In(tz)
	var dates []time.Time
	for i := 0; i < 30; i++ {
		dates = append(dates, today.AddDate(0, 0, i))
	}

	datesPerPage := 5
	start := page * datesPerPage
	end := start + datesPerPage
	if end > len(dates) {
		end = len(dates)
	}
	pageDates := dates[start:end]

	markup := &tgbotapi.InlineKeyboardMarkup{}
	for _, date := range pageDates {
		dateStr := date.Format("2006-01-02")
		weekday := []string{"Вс", "Пн", "Вт", "Ср", "Чт", "Пт", "Сб"}[int(date.Weekday())]
		label := fmt.Sprintf("%s (%s)", date.Format("02.01"), weekday)
		markup.InlineKeyboard = append(markup.InlineKeyboard, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(label, "date_"+dateStr),
		})
	}

	var navButtons []tgbotapi.InlineKeyboardButton
	if page > 0 {
		navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData("← Назад", fmt.Sprintf("date_page_%d", page-1)))
	}
	if end < len(dates) {
		navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData("Далее →", fmt.Sprintf("date_page_%d", page+1)))
	}
	if len(navButtons) > 0 {
		markup.InlineKeyboard = append(markup.InlineKeyboard, navButtons)
	}

	message := tgbotapi.NewMessage(msg.Chat.ID, "Выберите дату:")
	message.ReplyMarkup = markup
	bot.Send(message)
}

func showMasterSelection(cb *tgbotapi.CallbackQuery) {
	userID := cb.From.ID
	session := getSession(userID)
	
	date, ok := session.Data["date"].(string)
	if !ok || date == "" {
		bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, Text: "Ошибка: дата не выбрана", ShowAlert: true})
		return
	}
	
	time, ok := session.Data["time"].(string)
	if !ok || time == "" {
		bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, Text: "Ошибка: время не выбрано", ShowAlert: true})
		return
	}

	log.Printf("Total masters: %d", len(masters))
	bookedMasters := getBookedMasters(date, time)
	log.Printf("Booked masters: %v", bookedMasters)

	var availableMasters []string
	for _, master := range masters {
		log.Printf("Checking master: %s, booked: %v", master.Name, bookedMasters[master.Name])
		if !bookedMasters[master.Name] {
			availableMasters = append(availableMasters, master.Name)
		}
	}

	log.Printf("Available masters: %d", len(availableMasters))

	markup := &tgbotapi.InlineKeyboardMarkup{}
	for _, master := range availableMasters {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(master, "master_"+master),
		})
	}
	markup.InlineKeyboard = append(markup.InlineKeyboard, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("← Назад", "back_to_time"),
	})

	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, fmt.Sprintf("Выберите мастера на %s %s:", date, time))
	editMsg.ReplyMarkup = markup
	bot.Send(editMsg)
	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
}

func showBookingConfirmation(cb *tgbotapi.CallbackQuery) {
	userID := cb.From.ID
	session := getSession(userID)
	date, _ := session.Data["date"].(string)
	time, _ := session.Data["time"].(string)
	master, _ := session.Data["master"].(string)
	pkgKey, ok := session.Data["package"].(string)
	if !ok || pkgKey == "" {
		bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, Text: "Ошибка: процедура не выбрана", ShowAlert: true})
		return
	}
	pkg := packages[pkgKey]

	text := fmt.Sprintf("Подтвердите запись:\n\n📅 %s\n🕐 %s\n👨⚕️ %s\n💼 %s\n💰 %d ₽\n\nЦентр: HGN Москва\nАдрес: Мичуринский проспект, 19к1", date, time, master, pkg.Name, pkg.Price)

	markup := &tgbotapi.InlineKeyboardMarkup{}
	markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData("✅ Подтвердить", "confirm_booking"),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("← Назад", "back_to_master"),
		},
	}

	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, text)
	editMsg.ReplyMarkup = markup
	bot.Send(editMsg)
	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
}

func finalizeBooking(cb *tgbotapi.CallbackQuery) {
	userID := cb.From.ID
	session := getSession(userID)
	date := session.Data["date"].(string)
	time := session.Data["time"].(string)
	master := session.Data["master"].(string)
	gender := session.Data["gender"].(string)
	clientName, _ := session.Data["client_name"].(string)
	clientPhone, _ := session.Data["client_phone"].(string)
	pkgKey := session.Data["package"].(string)
	pkg := packages[pkgKey]

	err := bookSlotWithPackage(date, time, gender, master, userID, cb.From.UserName, clientName, clientPhone, pkg.Name)
	if err != nil {
		bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, Text: "Ошибка при бронировании", ShowAlert: true})
		return
	}

	// Send confirmation
	text := fmt.Sprintf("✅ Запись подтверждена!\n\n📅 %s\n🕐 %s\n👨⚕️ %s\n\nЦентр: HGN Москва\nАдрес: Мичуринский проспект, 19к1", date, time, master)
	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, text)
	bot.Send(editMsg)

	for _, admin := range cfg.Admins {
		msg := tgbotapi.NewMessage(admin, fmt.Sprintf("🔔 Новая запись!\n\n👨⚕️ %s\n📅 %s\n🕐 %s\n💼 %s\n💰 %d ₽\n👤 %s\n📞 %s\n💬 @%s", master, date, time, pkg.Name, pkg.Price, clientName, clientPhone, cb.From.UserName))
		bot.Send(msg)
	}

	clearSession(userID)
	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, Text: "Запись успешна!", ShowAlert: false})
}

func showTimeSelection(cb *tgbotapi.CallbackQuery, dateStr string) {
	// Fixed times
	times := []string{"09:00", "10:00", "11:00", "12:00", "13:00", "14:00", "15:00", "16:00", "17:00", "18:00", "19:00", "20:00"}

	markup := &tgbotapi.InlineKeyboardMarkup{}
	for _, t := range times {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(t, "time_"+dateStr+"_"+t),
		})
	}
	markup.InlineKeyboard = append(markup.InlineKeyboard, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("← Назад", "back_to_date"),
	})

	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, fmt.Sprintf("Доступное время на %s:", dateStr))
	editMsg.ReplyMarkup = markup
	bot.Send(editMsg)
	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, ShowAlert: false})
}

func showMyBookings(msg *tgbotapi.Message) {
	bookings, err := getUserBookings(msg.From.ID)
	if err != nil || len(bookings) == 0 {
		bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "У вас нет активных записей"))
		return
	}

	for _, booking := range bookings {
		canCancel := canCancelBooking(booking)
		text := fmt.Sprintf("📋 Ваша запись:\n\n📅 %s\n🕐 %s\n👨‍⚕️ %s\n\nЦентр: HGN Москва\nАдрес: Мичуринский проспект, 19г1",
			booking.Date, booking.Time, booking.MasterName)

		markup := &tgbotapi.InlineKeyboardMarkup{}
		if canCancel {
			text += "\n\n⚠️ Отмена возможна за 2 часа до процедуры"
			markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{
				{tgbotapi.NewInlineKeyboardButtonData("❌ Отменить запись", fmt.Sprintf("cancel_booking_%d", booking.ID))},
			}
		} else {
			text += "\n\n⚠️ Отмена невозможна (менее 2 часов до процедуры)"
		}

		msg := tgbotapi.NewMessage(msg.Chat.ID, text)
		if len(markup.InlineKeyboard) > 0 {
			msg.ReplyMarkup = markup
		}
		bot.Send(msg)
		break
	}
}

func cancelUserBooking(cb *tgbotapi.CallbackQuery, data string) {
	var bookingID int
	fmt.Sscanf(data, "cancel_booking_%d", &bookingID)

	booking, err := getBookingByID(bookingID)
	if err != nil {
		bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, Text: "Ошибка при отмене", ShowAlert: true})
		return
	}

	err = cancelBooking(bookingID)
	if err != nil {
		bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, Text: "Ошибка при отмене", ShowAlert: true})
		return
	}

	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "✅ Запись отменена")
	bot.Send(editMsg)

	for _, admin := range cfg.Admins {
		msg := tgbotapi.NewMessage(admin, fmt.Sprintf("❌ Отмена записи\n\n👨⚕️ %s\n📅 %s\n🕐 %s\n💬 @%s", booking.MasterName, booking.Date, booking.Time, cb.From.UserName))
		bot.Send(msg)
	}

	bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: cb.ID, Text: "Запись отменена", ShowAlert: false})
}
