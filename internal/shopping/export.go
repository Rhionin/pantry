// Package shopping provides shopping list management and cart export functionality.
package shopping

import (
	"context"
	"fmt"
	"strings"
)

// ExportItem is an item to be submitted to an online shopping cart.
type ExportItem struct {
	ItemID   string
	Name     string
	Quantity int
}

// ExportError describes a partial cart export failure where one or more items
// could not be added to the cart.
type ExportError struct {
	FailedItems []ExportItem // items that could not be added to the cart
	Err         error        // underlying error
}

func (e *ExportError) Error() string {
	names := make([]string, len(e.FailedItems))
	for i, item := range e.FailedItems {
		names[i] = item.Name
	}
	return fmt.Sprintf("cart export failed for items: %s: %v", strings.Join(names, ", "), e.Err)
}

func (e *ExportError) Unwrap() error {
	return e.Err
}

// CartExporter submits shopping list items to an online cart service.
type CartExporter interface {
	Export(ctx context.Context, items []ExportItem) error
}

// NoOpExporter is a CartExporter used when cart integration is not configured.
// All export operations succeed immediately without contacting any external service.
type NoOpExporter struct{}

// Export always returns nil; cart integration is not configured.
func (n *NoOpExporter) Export(_ context.Context, _ []ExportItem) error {
	return nil
}
