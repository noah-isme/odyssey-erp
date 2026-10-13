package shared

import (
	"testing"
	"time"
)

func TestBusinessDateUsesJakartaCalendar(t *testing.T) {
	// 20:00 UTC on 31 May is 03:00 on 1 June in Jakarta (UTC+7).
	got := BusinessDate(time.Date(2026, 5, 31, 20, 0, 0, 0, time.UTC))
	if !got.Equal(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("BusinessDate = %s", got)
	}
	if BusinessLocation.String() != "Asia/Jakarta" {
		t.Skip("tzdata missing; fell back to UTC")
	}
}
