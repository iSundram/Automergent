package app

import (
	"encoding/json"
	"net/http"
	"time"
)

// userClockMsg delivers the user's IP-located timezone (and country code) so
// the footer can show their local time.
type userClockMsg struct {
	tz      *time.Location
	country string
}

// geoTimeout bounds the lookup; the clock is chrome, never worth waiting on.
const geoTimeout = 4 * time.Second

// resolveUserClock locates the user from their public IP and returns the
// timezone that follows from it. The result is display-only; on any failure
// it falls back to the system timezone with no country label, so the footer
// clock degrades to local time rather than disappearing.
func resolveUserClock() userClockMsg {
	fallback := userClockMsg{tz: time.Local}
	client := &http.Client{Timeout: geoTimeout}
	resp, err := client.Get("https://ipinfo.io/json")
	if err != nil {
		return fallback
	}
	defer func() { _ = resp.Body.Close() }()

	var info struct {
		Timezone string `json:"timezone"`
		Country  string `json:"country"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return fallback
	}
	if info.Timezone == "" {
		return fallback
	}
	tz, err := time.LoadLocation(info.Timezone)
	if err != nil {
		return fallback
	}
	return userClockMsg{tz: tz, country: info.Country}
}
