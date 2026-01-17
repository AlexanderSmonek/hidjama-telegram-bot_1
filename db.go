package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/supabase-community/supabase-go"
)

type Master struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Code    string `json:"code"`
	Contact string `json:"contact"`
	Gender  string `json:"gender"`
	Active  bool   `json:"active"`
}

type UserSession struct {
	Step   string                 `json:"step"`
	Data   map[string]interface{} `json:"data"`
	Gender string                 `json:"gender,omitempty"`
	Date   string                 `json:"date,omitempty"`
	Time   string                 `json:"time,omitempty"`
	Master string                 `json:"master,omitempty"`
}

type Slot struct {
	ID         int       `json:"id"`
	Date       string    `json:"date"`
	Time       string    `json:"time"`
	Gender     string    `json:"gender"`
	MasterID   string    `json:"master_id"`
	MasterName string    `json:"master_name"`
	Status     string    `json:"status"`
	UserID     string    `json:"user_id"`
	Username   string    `json:"username"`
	BookedAt   time.Time `json:"booked_at"`
	Source     string    `json:"source"`
}

var supabaseClient *supabase.Client
var userSessions = make(map[int64]*UserSession)

func initDB() {
	var err error
	supabaseClient, err = supabase.NewClient(cfg.SupabaseURL, cfg.SupabaseKey, nil)
	if err != nil {
		log.Fatal("Failed to initialize Supabase client:", err)
	}
	log.Println("Supabase client initialized")
}

func loadMastersFromDB() map[string]Master {
	masters := make(map[string]Master)
	
	data, _, err := supabaseClient.From("masters").Select("*", "exact", false).Execute()
	if err != nil {
		log.Printf("Error loading masters: %v", err)
		return masters
	}
	
	var results []Master
	err = json.Unmarshal(data, &results)
	if err != nil {
		log.Printf("Error unmarshaling masters: %v", err)
		return masters
	}
	
	log.Printf("Loaded %d masters from DB", len(results))
	for _, m := range results {
		log.Printf("Master: %s, Active: %v", m.Name, m.Active)
		if m.Active {
			masters[m.ID] = m
		}
	}
	log.Printf("Active masters: %d", len(masters))
	return masters
}

func loadPackagesFromDB() map[string]Package {
	packages := make(map[string]Package)
	
	data, _, err := supabaseClient.From("packages").Select("*", "exact", false).Execute()
	if err != nil {
		log.Printf("Error loading packages: %v", err)
		return packages
	}
	
	var results []Package
	json.Unmarshal(data, &results)
	
	for _, p := range results {
		packages[p.Key] = p
	}
	return packages
}

func getBookedMasters(date, time string) map[string]bool {
	booked := make(map[string]bool)
	
	data, _, err := supabaseClient.From("slots").
		Select("master_name", "exact", false).
		Eq("date", date).
		Eq("time", time).
		Eq("status", "booked").
		Execute()
	
	if err != nil {
		log.Printf("Error getting booked masters: %v", err)
		return booked
	}
	
	var results []Slot
	json.Unmarshal(data, &results)
	
	for _, slot := range results {
		booked[slot.MasterName] = true
	}
	return booked
}

func bookSlot(date, slotTime, gender, master string, userID int64, username string) error {
	moscowTime := time.Now().In(tz)
	slot := map[string]interface{}{
		"date":        date,
		"time":        slotTime,
		"gender":      gender,
		"master_name": master,
		"status":      "booked",
		"user_id":     fmt.Sprintf("%d", userID),
		"username":    username,
		"booked_at":   moscowTime.Format("2006-01-02 15:04:05"),
		"source":      "bot",
	}
	
	log.Printf("Booking slot: %+v", slot)
	_, _, err := supabaseClient.From("slots").Insert(slot, false, "", "", "").Execute()
	if err != nil {
		log.Printf("Error booking slot: %v", err)
	}
	return err
}

func bookSlotWithContact(date, slotTime, gender, master string, userID int64, username, clientName, clientPhone string) error {
	moscowTime := time.Now().In(tz)
	slot := map[string]interface{}{
		"date":         date,
		"time":         slotTime,
		"gender":       gender,
		"master_name":  master,
		"status":       "booked",
		"user_id":      fmt.Sprintf("%d", userID),
		"username":     username,
		"client_name":  clientName,
		"client_phone": clientPhone,
		"booked_at":    moscowTime.Format("2006-01-02 15:04:05"),
		"source":       "bot",
	}
	
	log.Printf("Booking slot: %+v", slot)
	_, _, err := supabaseClient.From("slots").Insert(slot, false, "", "", "").Execute()
	if err != nil {
		log.Printf("Error booking slot: %v", err)
	}
	return err
}

func bookSlotWithPackage(date, slotTime, gender, master string, userID int64, username, clientName, clientPhone, packageName string) error {
	moscowTime := time.Now().In(tz)
	slot := map[string]interface{}{
		"date":          date,
		"time":          slotTime,
		"gender":        gender,
		"master_name":   master,
		"status":        "booked",
		"user_id":       fmt.Sprintf("%d", userID),
		"username":      username,
		"client_name":   clientName,
		"client_phone":  clientPhone,
		"package_name":  packageName,
		"booked_at":     moscowTime.Format("2006-01-02 15:04:05"),
		"source":        "bot",
	}
	
	log.Printf("Booking slot: %+v", slot)
	_, _, err := supabaseClient.From("slots").Insert(slot, false, "", "", "").Execute()
	if err != nil {
		log.Printf("Error booking slot: %v", err)
	}
	return err
}

func getUserBookings(userID int64) ([]Slot, error) {
	data, _, err := supabaseClient.From("slots").
		Select("*", "exact", false).
		Eq("user_id", fmt.Sprintf("%d", userID)).
		Eq("status", "booked").
		Execute()
	
	if err != nil {
		return nil, err
	}
	
	var results []Slot
	json.Unmarshal(data, &results)
	return results, nil
}

func cancelBooking(slotID int) error {
	update := map[string]interface{}{
		"status":    "cancelled",
		"user_id":   nil,
		"username":  nil,
		"booked_at": nil,
	}
	
	_, _, err := supabaseClient.From("slots").
		Update(update, "", "").
		Eq("id", fmt.Sprintf("%d", slotID)).
		Execute()
	
	return err
}

func canCancelBooking(slot Slot) bool {
	slotDateTime, err := time.Parse("2006-01-02 15:04", slot.Date+" "+slot.Time)
	if err != nil {
		return false
	}
	
	now := time.Now().In(tz)
	twoHoursBefore := slotDateTime.Add(-2 * time.Hour)
	
	return now.Before(twoHoursBefore)
}

func loadUserSession(userID int64) (*UserSession, error) {
	if session, ok := userSessions[userID]; ok {
		return session, nil
	}
	return &UserSession{Data: make(map[string]interface{})}, nil
}

func saveUserSession(userID int64, session *UserSession) {
	userSessions[userID] = session
}

func deleteUserSession(userID int64) {
	delete(userSessions, userID)
}

func loadAllSessions() {
	userSessions = make(map[int64]*UserSession)
}
