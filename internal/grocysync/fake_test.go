package grocysync

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// fakeGrocy is the slice of Grocy's API this sidecar calls.
type fakeGrocy struct {
	mu             sync.Mutex
	key            string
	next           int
	objects        map[string]map[int]map[string]any
	stock          []fakeStock
	logs           []fakeLog
	txs            map[string]*fakeTx
	calls          []string
	bodies         []string
	fields         map[int]map[string]string
	productError   string
	bookingError   string
	undoError      string
	systemInfoCode int
}

type fakeStock struct {
	ProductID  int
	Amount     float64
	BestBefore string
	Purchased  string
	StockID    string
	Note       string
	LocationID int
}

type fakeTx struct {
	Kind   string
	Undone bool
	Entry  fakeStock
}

type fakeLog struct {
	ID         int
	ProductID  int
	Amount     float64
	BestBefore string
	Purchased  string
	Type       string
	Undone     int
	TxID       string
	StockID    string
	Note       string
}

func newFakeGrocy() *fakeGrocy {
	return &fakeGrocy{
		key:     "test-key",
		objects: map[string]map[int]map[string]any{},
		txs:     map[string]*fakeTx{},
		fields:  map[int]map[string]string{},
	}
}

func (f *fakeGrocy) server() *httptest.Server {
	return httptest.NewServer(f)
}

func (f *fakeGrocy) count(method, fragment string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, call := range f.calls {
		if strings.HasPrefix(call, method+" ") && strings.Contains(call, fragment) {
			n++
		}
	}
	return n
}

func (f *fakeGrocy) saw(method, fragment string) bool {
	return f.count(method, fragment) > 0
}

func (f *fakeGrocy) productByMarker(marker string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, obj := range f.objects["products"] {
		if markerID(strField(obj, "description")) == marker {
			return obj
		}
	}
	return nil
}

func (f *fakeGrocy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("GROCY-API-KEY") != f.key {
		http.Error(w, "no", http.StatusUnauthorized)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	f.calls = append(f.calls, r.Method+" "+r.URL.RequestURI())
	if len(raw) > 0 {
		f.bodies = append(f.bodies, string(raw))
	}
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(r.URL.Path, "/api")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "objects" {
		f.objectsRoute(w, r, parts[1:], raw)
		return
	}
	if len(parts) >= 3 && parts[0] == "userfields" && parts[1] == "products" {
		id, _ := strconv.Atoi(parts[2])
		switch r.Method {
		case http.MethodPut:
			var patch map[string]string
			_ = json.Unmarshal(raw, &patch)
			if f.fields[id] == nil {
				f.fields[id] = map[string]string{}
			}
			for key, value := range patch {
				f.fields[id][key] = value
			}
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			vals := f.fields[id]
			if vals == nil {
				vals = map[string]string{}
			}
			_ = json.NewEncoder(w).Encode(vals)
		default:
			http.NotFound(w, r)
		}
		return
	}
	if len(parts) >= 3 && parts[0] == "stock" && parts[1] == "products" {
		id, _ := strconv.Atoi(parts[2])
		switch {
		case len(parts) == 4 && parts[3] == "add" && r.Method == http.MethodPost:
			f.addStock(w, id, raw)
		case len(parts) == 4 && parts[3] == "consume" && r.Method == http.MethodPost:
			f.consume(w, id, raw)
		case len(parts) == 4 && parts[3] == "entries" && r.Method == http.MethodGet:
			f.listEntries(w, id)
		case len(parts) == 3 && r.Method == http.MethodGet:
			// Product details. Grocy 4 does not include stock_entries here.
			_ = json.NewEncoder(w).Encode(map[string]any{"product": map[string]any{"id": id}})
		default:
			http.NotFound(w, r)
		}
		return
	}
	if len(parts) == 4 && parts[0] == "stock" && parts[1] == "transactions" && parts[3] == "undo" && r.Method == http.MethodPost {
		f.undo(w, parts[2])
		return
	}
	if len(parts) == 2 && parts[0] == "system" && parts[1] == "info" && r.Method == http.MethodGet {
		f.systemInfo(w)
		return
	}
	http.NotFound(w, r)
}

func (f *fakeGrocy) systemInfo(w http.ResponseWriter) {
	if f.systemInfoCode >= 300 {
		w.WriteHeader(f.systemInfoCode)
		_, _ = w.Write([]byte(`{"unused":"do-not-leak"}`))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"grocy_version": map[string]string{"Version": "4.7.1", "ReleaseDate": "2026-09-04"},
	})
}

func (f *fakeGrocy) objectsRoute(w http.ResponseWriter, r *http.Request, parts []string, raw []byte) {
	entity := parts[0]
	if entity == "stock_log" && len(parts) == 1 && r.Method == http.MethodGet {
		f.listStockLogs(w, r)
		return
	}
	if f.objects[entity] == nil {
		f.objects[entity] = map[int]map[string]any{}
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		list := []map[string]any{}
		for id, obj := range f.objects[entity] {
			obj["id"] = id
			list = append(list, obj)
		}
		_ = json.NewEncoder(w).Encode(list)
	case len(parts) == 1 && r.Method == http.MethodPost:
		obj := map[string]any{}
		_ = json.Unmarshal(raw, &obj)
		if !f.acceptProductWrite(w, entity, 0, obj) {
			return
		}
		f.next++
		id := f.next
		obj["id"] = id
		f.objects[entity][id] = obj
		_ = json.NewEncoder(w).Encode(map[string]any{"created_object_id": id})
	case len(parts) == 2 && r.Method == http.MethodPut:
		id, _ := strconv.Atoi(parts[1])
		obj := f.objects[entity][id]
		if obj == nil {
			http.NotFound(w, r)
			return
		}
		patch := map[string]any{}
		_ = json.Unmarshal(raw, &patch)
		if !f.acceptProductWrite(w, entity, id, patch) {
			return
		}
		for k, v := range patch {
			obj[k] = v
		}
		w.WriteHeader(http.StatusNoContent)
	case len(parts) == 2 && r.Method == http.MethodDelete:
		id, _ := strconv.Atoi(parts[1])
		delete(f.objects[entity], id)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeGrocy) acceptProductWrite(w http.ResponseWriter, entity string, id int, obj map[string]any) bool {
	if entity != "products" {
		return true
	}
	if col := unknownProductColumn(obj); col != "" {
		writeGrocyError(w, fmt.Sprintf("SQLSTATE[HY000]: General error: 1 table products has no column named %s", col))
		return false
	}
	if name, ok := obj["name"].(string); ok && strings.TrimSpace(name) != "" && f.productWithName(name, id) != 0 {
		writeGrocyError(w, "UNIQUE constraint failed: products.name")
		return false
	}
	if f.productError != "" {
		writeGrocyError(w, f.productError)
		return false
	}
	return true
}

func (f *fakeGrocy) productWithName(name string, except int) int {
	want := strings.ToLower(strings.TrimSpace(name))
	for id, obj := range f.objects["products"] {
		if id == except {
			continue
		}
		if strings.ToLower(strings.TrimSpace(strField(obj, "name"))) == want {
			return id
		}
	}
	return 0
}

func writeGrocyError(w http.ResponseWriter, message string) {
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error_message": message})
}

func unknownProductColumn(obj map[string]any) string {
	var bad []string
	for key := range obj {
		if !grocyProductColumns[key] {
			bad = append(bad, key)
		}
	}
	sort.Strings(bad)
	if len(bad) == 0 {
		return ""
	}
	return bad[0]
}

// grocyProductColumns is the products table in Grocy 4.7.1.
// qu_factor_purchase_to_stock was removed in 4.0.0; the objects API writes this table, not products_view.
var grocyProductColumns = map[string]bool{
	"id": true, "name": true, "description": true, "product_group_id": true, "active": true,
	"location_id": true, "shopping_location_id": true, "qu_id_purchase": true, "qu_id_stock": true,
	"min_stock_amount": true, "default_best_before_days": true, "default_best_before_days_after_open": true,
	"default_best_before_days_after_freezing": true, "default_best_before_days_after_thawing": true,
	"picture_file_name": true, "enable_tare_weight_handling": true, "tare_weight": true,
	"not_check_stock_fulfillment_for_recipes": true, "parent_product_id": true, "calories": true,
	"cumulate_min_stock_amount_of_sub_products": true, "due_type": true, "quick_consume_amount": true,
	"hide_on_stock_overview": true, "default_stock_label_type": true, "should_not_be_frozen": true,
	"treat_opened_as_out_of_stock": true, "no_own_stock": true, "default_consume_location_id": true,
	"move_on_open": true, "row_created_timestamp": true, "qu_id_consume": true,
	"auto_reprint_stock_label": true, "quick_open_amount": true, "qu_id_price": true,
	"disable_open": true, "default_purchase_price_type": true,
}

func (f *fakeGrocy) addStock(w http.ResponseWriter, productID int, raw []byte) {
	if f.bookingError != "" {
		writeAPIError(w, http.StatusBadRequest, f.bookingError)
		return
	}
	var body addStockBody
	_ = json.Unmarshal(raw, &body)
	if body.Amount == 0 {
		body.Amount = 1
	}
	kind := body.TransactionType
	if kind == "" {
		kind = "purchase"
	}
	f.next++
	tx := "tx" + strconv.Itoa(f.next)
	stockID := ""
	merged := -1
	for i, entry := range f.stock {
		if entry.ProductID == productID && entry.LocationID == body.LocationID && entry.BestBefore == body.BestBefore && entry.Purchased == body.Purchased && entry.Note == body.Note {
			merged = i
			break
		}
	}
	if merged >= 0 {
		f.stock[merged].Amount += body.Amount
		stockID = f.stock[merged].StockID
	} else {
		stockID = "st" + strconv.Itoa(f.next)
		f.stock = append(f.stock, fakeStock{
			ProductID:  productID,
			Amount:     body.Amount,
			BestBefore: body.BestBefore,
			Purchased:  body.Purchased,
			StockID:    stockID,
			Note:       body.Note,
			LocationID: body.LocationID,
		})
	}
	booked := fakeStock{ProductID: productID, Amount: body.Amount, BestBefore: body.BestBefore, Purchased: body.Purchased, StockID: stockID, Note: body.Note, LocationID: body.LocationID}
	f.txs[tx] = &fakeTx{Kind: "purchase", Entry: booked}
	f.logs = append(f.logs, fakeLog{
		ID: f.next, ProductID: productID, Amount: body.Amount, BestBefore: body.BestBefore, Purchased: body.Purchased,
		Type: kind, TxID: tx, StockID: stockID, Note: body.Note,
	})
	f.writeBooking(w, f.logs[len(f.logs)-1])
}

func (f *fakeGrocy) consume(w http.ResponseWriter, productID int, raw []byte) {
	if f.bookingError != "" {
		writeAPIError(w, http.StatusBadRequest, f.bookingError)
		return
	}
	var body consumeBody
	_ = json.Unmarshal(raw, &body)
	if body.Amount == 0 {
		body.Amount = 1
	}
	idx := -1
	for i, entry := range f.stock {
		if entry.ProductID != productID {
			continue
		}
		if body.StockEntryID != "" && entry.StockID != body.StockEntryID {
			continue
		}
		idx = i
		break
	}
	if idx < 0 || f.stock[idx].Amount+1e-9 < body.Amount {
		writeAPIError(w, http.StatusBadRequest, "Amount to be consumed cannot be > current stock amount (if supplied, at the desired location)")
		return
	}
	entry := f.stock[idx]
	entry.Amount -= body.Amount
	if entry.Amount <= 1e-9 {
		f.stock = append(f.stock[:idx], f.stock[idx+1:]...)
	} else {
		f.stock[idx] = entry
	}
	f.next++
	tx := "tx" + strconv.Itoa(f.next)
	taken := entry
	taken.Amount = body.Amount
	f.txs[tx] = &fakeTx{Kind: "consume", Entry: taken}
	f.logs = append(f.logs, fakeLog{
		ID: f.next, ProductID: productID, Amount: -body.Amount, BestBefore: entry.BestBefore, Purchased: entry.Purchased,
		Type: "consume", TxID: tx, StockID: entry.StockID, Note: entry.Note,
	})
	f.writeBooking(w, f.logs[len(f.logs)-1])
}

func (f *fakeGrocy) writeBooking(w http.ResponseWriter, log fakeLog) {
	_ = json.NewEncoder(w).Encode([]map[string]any{f.logJSON(log)})
}

func (f *fakeGrocy) logJSON(log fakeLog) map[string]any {
	return map[string]any{
		"id":               log.ID,
		"product_id":       log.ProductID,
		"amount":           log.Amount,
		"best_before_date": log.BestBefore,
		"purchased_date":   log.Purchased,
		"transaction_type": log.Type,
		"undone":           log.Undone,
		"transaction_id":   log.TxID,
		"stock_id":         log.StockID,
		"note":             log.Note,
	}
}

func (f *fakeGrocy) listEntries(w http.ResponseWriter, productID int) {
	var entries []map[string]any
	for _, entry := range f.stock {
		if entry.ProductID != productID {
			continue
		}
		entries = append(entries, map[string]any{
			"stock_id":         entry.StockID,
			"amount":           entry.Amount,
			"note":             entry.Note,
			"best_before_date": entry.BestBefore,
			"purchased_date":   entry.Purchased,
		})
	}
	if entries == nil {
		entries = []map[string]any{}
	}
	_ = json.NewEncoder(w).Encode(entries)
}

func (f *fakeGrocy) listStockLogs(w http.ResponseWriter, r *http.Request) {
	queries := r.URL.Query()["query[]"]
	var rows []map[string]any
	for _, log := range f.logs {
		ok := true
		for _, q := range queries {
			match, valid := logMatches(log, q)
			if !valid {
				writeAPIError(w, http.StatusBadRequest, "Invalid query")
				return
			}
			if !match {
				ok = false
				break
			}
		}
		if ok {
			rows = append(rows, f.logJSON(log))
		}
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	_ = json.NewEncoder(w).Encode(rows)
}

func logMatches(log fakeLog, q string) (match bool, valid bool) {
	field, op, value, ok := splitQuery(q)
	if !ok || strings.Contains(value, ":") {
		return false, false
	}
	var got string
	switch field {
	case "product_id":
		got = strconv.Itoa(log.ProductID)
	case "note":
		got = log.Note
	case "transaction_type":
		got = log.Type
	default:
		return false, false
	}
	switch op {
	case "=":
		return got == value, true
	case "~":
		return strings.Contains(got, value), true
	default:
		return false, false
	}
}

func splitQuery(q string) (field, op, value string, ok bool) {
	for _, candidate := range []string{"!=", ">=", "<=", "!~", "~", "="} {
		if i := strings.Index(q, candidate); i > 0 {
			return q[:i], candidate, q[i+len(candidate):], true
		}
	}
	return "", "", "", false
}

func (f *fakeGrocy) undo(w http.ResponseWriter, id string) {
	if f.undoError != "" {
		writeAPIError(w, http.StatusBadRequest, f.undoError)
		return
	}
	tx := f.txs[id]
	if tx == nil || tx.Undone {
		writeAPIError(w, http.StatusBadRequest, "This transaction was not found or already undone")
		return
	}
	for _, log := range f.logs {
		if log.StockID == tx.Entry.StockID && log.Undone == 0 && log.TxID != id && log.ID > f.logID(id) {
			writeAPIError(w, http.StatusBadRequest, "Booking has subsequent dependent bookings, undo not possible")
			return
		}
	}
	tx.Undone = true
	for i := range f.logs {
		if f.logs[i].TxID == id {
			f.logs[i].Undone = 1
		}
	}
	if tx.Kind == "purchase" {
		// Grocy deletes the stock row. A second purchase only raised the amount; it does not survive.
		kept := f.stock[:0]
		for _, entry := range f.stock {
			if entry.StockID != tx.Entry.StockID {
				kept = append(kept, entry)
			}
		}
		f.stock = kept
	} else if tx.Kind == "consume" {
		f.restore(tx.Entry)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeGrocy) logID(txID string) int {
	for _, log := range f.logs {
		if log.TxID == txID {
			return log.ID
		}
	}
	return 0
}

func (f *fakeGrocy) changeAmount(stockID string, delta float64) error {
	for i, entry := range f.stock {
		if entry.StockID != stockID {
			continue
		}
		entry.Amount += delta
		if entry.Amount <= 1e-9 {
			f.stock = append(f.stock[:i], f.stock[i+1:]...)
			return nil
		}
		f.stock[i] = entry
		return nil
	}
	return fmt.Errorf("The transaction could not be undone")
}

func (f *fakeGrocy) restore(entry fakeStock) {
	for i, row := range f.stock {
		if row.StockID == entry.StockID {
			row.Amount += entry.Amount
			f.stock[i] = row
			return
		}
	}
	f.stock = append(f.stock, entry)
}

// duplicatePurchase models the live failure: a second purchase with the same
// pantry-instance note shares the stock row and raises its amount.
func (f *fakeGrocy) duplicatePurchase(note string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, log := range f.logs {
		if log.Note != note || log.Type != "purchase" || log.Undone != 0 {
			continue
		}
		f.next++
		tx := "tx" + strconv.Itoa(f.next)
		_ = f.changeAmount(log.StockID, log.Amount)
		f.logs = append(f.logs, fakeLog{
			ID: f.next, ProductID: log.ProductID, Amount: log.Amount, BestBefore: log.BestBefore, Purchased: log.Purchased,
			Type: "purchase", TxID: tx, StockID: log.StockID, Note: log.Note,
		})
		f.txs[tx] = &fakeTx{Kind: "purchase", Entry: fakeStock{ProductID: log.ProductID, Amount: log.Amount, BestBefore: log.BestBefore, Purchased: log.Purchased, StockID: log.StockID, Note: log.Note}}
		return tx
	}
	return ""
}

func writeAPIError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error_message": msg})
}

func strField(obj map[string]any, key string) string {
	s, _ := obj[key].(string)
	return s
}

func numField(obj map[string]any, key string) int {
	switch n := obj[key].(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}
