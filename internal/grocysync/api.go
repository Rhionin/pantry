package grocysync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// API talks to Grocy's HTTP API. The API key is a header and is not logged.
type API struct {
	Base string
	Key  string
	HTTP *http.Client
}

// NewAPI normalizes base. A URL without /api gets /api appended.
func NewAPI(raw, key string) (*API, error) {
	base, err := normalizeGrocyURL(raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("GROCY_API_KEY is empty")
	}
	return &API{
		Base: base,
		Key:  strings.TrimSpace(key),
		HTTP: &http.Client{Timeout: 60 * time.Second},
	}, nil
}

func normalizeGrocyURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("GROCY_URL must be an http address such as http://grocy:80")
	}
	if !strings.HasSuffix(u.Path, "/api") {
		u.Path = strings.TrimRight(u.Path, "/") + "/api"
	}
	return u.String(), nil
}

type objectRow struct {
	ID             int       `json:"id"`
	Name           string    `json:"name"`
	Description    *string   `json:"description"`
	Barcode        string    `json:"barcode"`
	ProductID      int       `json:"product_id"`
	Note           *string   `json:"note"`
	ShoppingListID int       `json:"shopping_list_id"`
	Amount         flexFloat `json:"amount"`
	Entity         string    `json:"entity"`
}

func (o objectRow) description() string {
	if o.Description == nil {
		return ""
	}
	return *o.Description
}

func (o objectRow) note() string {
	if o.Note == nil {
		return ""
	}
	return *o.Note
}

// StockEntry is one open Grocy stock row.
type StockEntry struct {
	StockID    string    `json:"stock_id"`
	Amount     flexFloat `json:"amount"`
	Note       *string   `json:"note"`
	BestBefore string    `json:"best_before_date"`
	Purchased  string    `json:"purchased_date"`
}

func (e StockEntry) note() string {
	if e.Note == nil {
		return ""
	}
	return *e.Note
}

type flexID string

func (f *flexID) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexID(s)
		return nil
	}
	*f = flexID(b)
	return nil
}

func (f flexID) String() string { return string(f) }

func (f flexID) Int() (int, error) {
	if f == "" {
		return 0, fmt.Errorf("missing id")
	}
	n, err := strconv.Atoi(string(f))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("Grocy returned an unusable id")
	}
	return n, nil
}

type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			return nil
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		*f = flexFloat(n)
		return nil
	}
	var n float64
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexFloat(n)
	return nil
}

func (a *API) List(ctx context.Context, entity string) ([]objectRow, error) {
	var rows []objectRow
	if err := a.do(ctx, http.MethodGet, "/objects/"+url.PathEscape(entity), nil, &rows); err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []objectRow{}
	}
	return rows, nil
}

func (a *API) Create(ctx context.Context, entity string, body any) (int, error) {
	var created struct {
		ID flexID `json:"created_object_id"`
	}
	if err := a.do(ctx, http.MethodPost, "/objects/"+url.PathEscape(entity), body, &created); err != nil {
		return 0, err
	}
	return created.ID.Int()
}

func (a *API) Update(ctx context.Context, entity string, id int, body any) error {
	return a.do(ctx, http.MethodPut, fmt.Sprintf("/objects/%s/%d", url.PathEscape(entity), id), body, nil)
}

func (a *API) Delete(ctx context.Context, entity string, id int) error {
	return a.do(ctx, http.MethodDelete, fmt.Sprintf("/objects/%s/%d", url.PathEscape(entity), id), nil, nil)
}

func (a *API) PutUserfields(ctx context.Context, productID int, fields map[string]string) error {
	return a.do(ctx, http.MethodPut, fmt.Sprintf("/userfields/products/%d", productID), fields, nil)
}

func (a *API) Userfields(ctx context.Context, productID int) (map[string]string, error) {
	var raw map[string]any
	if err := a.do(ctx, http.MethodGet, fmt.Sprintf("/userfields/products/%d", productID), nil, &raw); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for key, value := range raw {
		s, ok := value.(string)
		if !ok || strings.TrimSpace(s) == "" {
			continue
		}
		out[key] = s
	}
	return out, nil
}

type addStockBody struct {
	Amount          float64 `json:"amount"`
	BestBefore      string  `json:"best_before_date"`
	Purchased       string  `json:"purchased_date"`
	TransactionType string  `json:"transaction_type"`
	LocationID      int     `json:"location_id"`
	Note            string  `json:"note"`
}

type consumeBody struct {
	Amount          float64 `json:"amount"`
	TransactionType string  `json:"transaction_type"`
	Spoiled         bool    `json:"spoiled"`
	LocationID      int     `json:"location_id"`
	StockEntryID    string  `json:"stock_entry_id,omitempty"`
}

func (a *API) AddStock(ctx context.Context, productID int, body addStockBody) (string, error) {
	var resp struct {
		ID flexID `json:"id"`
	}
	if err := a.do(ctx, http.MethodPost, fmt.Sprintf("/stock/products/%d/add", productID), body, &resp); err != nil {
		return "", err
	}
	if resp.ID.String() == "" {
		return "", fmt.Errorf("Grocy did not return a stock transaction")
	}
	return resp.ID.String(), nil
}

func (a *API) Consume(ctx context.Context, productID int, body consumeBody) (string, error) {
	var resp struct {
		ID flexID `json:"id"`
	}
	if err := a.do(ctx, http.MethodPost, fmt.Sprintf("/stock/products/%d/consume", productID), body, &resp); err != nil {
		return "", err
	}
	if resp.ID.String() == "" {
		return "", fmt.Errorf("Grocy did not return a stock transaction")
	}
	return resp.ID.String(), nil
}

func (a *API) Undo(ctx context.Context, transactionID string) error {
	if transactionID == "" {
		return fmt.Errorf("there is no Grocy booking to undo")
	}
	return a.do(ctx, http.MethodPost, "/stock/transactions/"+url.PathEscape(transactionID)+"/undo", map[string]any{}, nil)
}

func (a *API) StockEntries(ctx context.Context, productID int) ([]StockEntry, error) {
	var resp struct {
		Entries []StockEntry `json:"stock_entries"`
	}
	if err := a.do(ctx, http.MethodGet, fmt.Sprintf("/stock/products/%d", productID), nil, &resp); err != nil {
		return nil, err
	}
	if resp.Entries == nil {
		resp.Entries = []StockEntry{}
	}
	return resp.Entries, nil
}

// Version reads grocy_version from /api/system/info.
// Grocy 4 returns an object with Version; older replies used a string.
func (a *API) Version(ctx context.Context) (string, error) {
	var info struct {
		GrocyVersion json.RawMessage `json:"grocy_version"`
	}
	if err := a.do(ctx, http.MethodGet, "/system/info", nil, &info); err != nil {
		return "", err
	}
	return parseGrocyVersion(info.GrocyVersion)
}

func parseGrocyVersion(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("Grocy did not report a version")
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil || strings.TrimSpace(s) == "" {
			return "", fmt.Errorf("Grocy did not report a version")
		}
		return strings.TrimSpace(s), nil
	}
	var obj struct {
		Version string `json:"Version"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil || strings.TrimSpace(obj.Version) == "" {
		return "", fmt.Errorf("Grocy did not report a version")
	}
	return strings.TrimSpace(obj.Version), nil
}

func (a *API) do(ctx context.Context, method, path string, body any, dest any) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("could not build the Grocy request")
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.Base+path, rdr)
	if err != nil {
		return fmt.Errorf("could not call Grocy")
	}
	req.Header.Set("GROCY-API-KEY", a.Key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach Grocy")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("could not read Grocy's reply")
	}
	if resp.StatusCode >= 300 {
		return grocyStatusError(resp.StatusCode, path, raw)
	}
	if dest == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("could not read Grocy's reply for %s", path)
	}
	return nil
}

func grocyStatusError(status int, path string, raw []byte) error {
	msg := grocyErrorMessage(raw)
	if msg == "" {
		return fmt.Errorf("Grocy returned %d for %s", status, path)
	}
	return fmt.Errorf("Grocy returned %d for %s: %s", status, path, msg)
}

func grocyErrorMessage(raw []byte) string {
	var body struct {
		Message string `json:"error_message"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return ""
	}
	msg := strings.TrimSpace(body.Message)
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return msg
}
