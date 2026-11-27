// Package order is a deliberately broken fixture for the first-investigation
// walkthrough in docs/first-investigation.md. Do not fix the bug: the
// walkthrough depends on TestStatusPaid failing.
package order

// Order is a customer order.
type Order struct {
	Paid    bool
	Shipped bool
}

// Status reports whether an order is ready to ship.
// A paid order is ready to ship.
func Status(o Order) string {
	if o.Paid && o.Shipped {
		return "ready"
	}
	return "pending"
}
