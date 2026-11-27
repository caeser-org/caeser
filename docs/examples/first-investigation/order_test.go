package order

import "testing"

func TestStatusPaid(t *testing.T) {
	got := Status(Order{Paid: true})
	if got != "ready" {
		t.Fatalf("Status() = %q, want %q", got, "ready")
	}
}
