package grocysync

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
)

// fakeGrocy is the slice of Grocy's API this sidecar calls.
type fakeGrocy struct {
	mu      sync.Mutex
	key     string
	next    int
	objects map[string]map[int]map[string]any
	stock   []fakeStock
	txs     map[string]*fakeTx
	calls   []string
	bodies  []string
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

func newFakeGrocy() *fakeGrocy {
	return &fakeGrocy{
		key:     "test-key",
		objects: map[string]map[int]map[string]any{},
		txs:     map[string]*fakeTx{},
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
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
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
	if len(parts) >= 3 && parts[0] == "userfields" && parts[1] == "products" && r.Method == http.MethodPut {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) >= 3 && parts[0] == "stock" && parts[1] == "products" {
		id, _ := strconv.Atoi(parts[2])
		switch {
		case len(parts) == 4 && parts[3] == "add" && r.Method == http.MethodPost:
			f.addStock(w, id, raw)
		case len(parts) == 4 && parts[3] == "consume" && r.Method == http.MethodPost:
			f.consume(w, id, raw)
		case len(parts) == 3 && r.Method == http.MethodGet:
			f.listStock(w, id)
		default:
			http.NotFound(w, r)
		}
		return
	}
	if len(parts) == 4 && parts[0] == "stock" && parts[1] == "transactions" && parts[3] == "undo" && r.Method == http.MethodPost {
		f.undo(w, parts[2])
		return
	}
	http.NotFound(w, r)
}

func (f *fakeGrocy) objectsRoute(w http.ResponseWriter, r *http.Request, parts []string, raw []byte) {
	entity := parts[0]
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

func (f *fakeGrocy) addStock(w http.ResponseWriter, productID int, raw []byte) {
	var body addStockBody
	_ = json.Unmarshal(raw, &body)
	f.next++
	entry := fakeStock{
		ProductID:  productID,
		Amount:     body.Amount,
		BestBefore: body.BestBefore,
		Purchased:  body.Purchased,
		StockID:    "st" + strconv.Itoa(f.next),
		Note:       body.Note,
		LocationID: body.LocationID,
	}
	f.stock = append(f.stock, entry)
	tx := "tx" + strconv.Itoa(f.next)
	f.txs[tx] = &fakeTx{Kind: "purchase", Entry: entry}
	_ = json.NewEncoder(w).Encode(map[string]any{"id": tx})
}

func (f *fakeGrocy) consume(w http.ResponseWriter, productID int, raw []byte) {
	var body consumeBody
	_ = json.Unmarshal(raw, &body)
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
	if idx < 0 {
		http.Error(w, "no stock", http.StatusBadRequest)
		return
	}
	entry := f.stock[idx]
	f.stock = append(f.stock[:idx], f.stock[idx+1:]...)
	f.next++
	tx := "tx" + strconv.Itoa(f.next)
	f.txs[tx] = &fakeTx{Kind: "consume", Entry: entry}
	_ = json.NewEncoder(w).Encode(map[string]any{"id": tx})
}

func (f *fakeGrocy) listStock(w http.ResponseWriter, productID int) {
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
	_ = json.NewEncoder(w).Encode(map[string]any{"stock_entries": entries})
}

func (f *fakeGrocy) undo(w http.ResponseWriter, id string) {
	tx := f.txs[id]
	if tx == nil || tx.Undone {
		http.Error(w, "missing", http.StatusBadRequest)
		return
	}
	tx.Undone = true
	if tx.Kind == "purchase" {
		kept := f.stock[:0]
		for _, entry := range f.stock {
			if entry.StockID != tx.Entry.StockID {
				kept = append(kept, entry)
			}
		}
		f.stock = kept
	} else if tx.Kind == "consume" {
		f.stock = append(f.stock, tx.Entry)
	}
	w.WriteHeader(http.StatusNoContent)
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
