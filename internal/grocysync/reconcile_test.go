package grocysync

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/syncsnap"
)

func TestReconcileMirrorsPantryAndIsIdempotent(t *testing.T) {
	fake := newFakeGrocy()
	srv := fake.server()
	defer srv.Close()
	api, err := NewAPI(srv.URL, fake.key)
	if err != nil {
		t.Fatal(err)
	}
	m, err := OpenMap(filepath.Join(t.TempDir(), "map.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	exp := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	inAt := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	pack := 6
	base := 400.0
	target := 4
	window := 2
	snap := syncsnap.Snapshot{
		Products: []syncsnap.Product{
			{
				ID: "p-chick", Name: "Chickpeas", Category: "Canned",
				NetBaseValue: &base, NetDimension: "mass", PackCount: &pack,
				Barcodes: []string{"111"}, TargetQuantity: &target, GroupID: "g1",
			},
			{
				ID: "p-oats", Name: "Oats", Category: "Cereal",
				Barcodes: []string{"222"},
			},
		},
		Groups: []syncsnap.Group{{
			ID: "g1", Name: "Beans", Rule: "same_as_ran_out", WindowMonths: &window,
			MemberIDs: []string{"p-chick"},
		}},
		Instances: []syncsnap.Instance{{
			ID: "inst-1", ProductID: "p-chick", StockInAt: inAt, ExpiresAt: &exp, StockMoveID: "move-in",
		}},
		Moves: []syncsnap.Move{{
			ID: "move-in", ProductID: "p-chick", Direction: "in", Quantity: 1, At: inAt,
		}},
		ShoppingList: []syncsnap.ShoppingLine{
			{ID: "line-1", ProductID: "p-chick", GroupID: "g1", Quantity: 2, Source: "auto"},
			{ID: "line-2", ProductID: "p-oats", Quantity: 1, Source: "manual"},
		},
	}
	// A row already on Grocy's default list, and a stray row that will land on the Pantry list.
	if _, err := api.Create(context.Background(), "shopping_lists", map[string]any{"name": "Default"}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Create(context.Background(), "shopping_list", map[string]any{
		"product_id": 1, "shopping_list_id": 1, "amount": 9, "note": "leave me", "done": 0,
	}); err != nil {
		t.Fatal(err)
	}
	src := &fixedSource{snap: snap}
	rec := &Reconciler{Pantry: src, Grocy: api, Map: m, Loc: time.UTC}
	if err := rec.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	posts := fake.count("POST", "/objects/products")
	if err := rec.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := fake.count("POST", "/objects/products"); got != posts {
		t.Fatalf("second reconcile created products, posts %d then %d", posts, got)
	}
	if got := fake.count("POST", "/stock/products/"); got != 1 {
		t.Fatalf("stock adds = %d, want 1", got)
	}

	parent := fake.productByMarker("group:g1")
	if parent == nil || strField(parent, "name") != "Beans" {
		t.Fatalf("parent = %#v", parent)
	}
	if numField(parent, "cumulate_min_stock_amount_of_sub_products") != 1 || numField(parent, "no_own_stock") != 1 {
		t.Fatalf("parent stock flags = %#v", parent)
	}
	if numField(parent, "min_stock_amount") != 0 {
		t.Fatalf("parent min stock = %v", parent["min_stock_amount"])
	}
	child := fake.productByMarker("p-chick")
	if child == nil {
		t.Fatal("missing chickpeas")
	}
	if numField(child, "parent_product_id") != numField(parent, "id") {
		t.Fatalf("child parent = %v, parent id = %v", child["parent_product_id"], parent["id"])
	}
	if numField(child, "min_stock_amount") != 4 {
		t.Fatalf("min stock = %v", child["min_stock_amount"])
	}
	if !strings.Contains(strField(child, "description"), "400 g") || !strings.Contains(strField(child, "description"), "pack of 6") {
		t.Fatalf("description = %q", child["description"])
	}
	oats := fake.productByMarker("p-oats")
	if oats == nil || oats["parent_product_id"] != nil {
		t.Fatalf("oats = %#v", oats)
	}

	var labels []string
	for _, obj := range fake.objects["product_groups"] {
		labels = append(labels, strField(obj, "name"))
	}
	if !hasAll(labels, "Beans", "Cereal") {
		t.Fatalf("labels = %v", labels)
	}
	if childGroup := numField(child, "product_group_id"); childGroup == 0 || childGroup != numField(parent, "product_group_id") {
		t.Fatalf("child group %d parent group %d", childGroup, numField(parent, "product_group_id"))
	}

	if len(fake.objects["product_barcodes"]) != 2 {
		t.Fatalf("barcodes = %d", len(fake.objects["product_barcodes"]))
	}
	if len(fake.stock) != 1 || fake.stock[0].Purchased != "2026-01-02" || fake.stock[0].BestBefore != "2026-06-01" {
		t.Fatalf("stock = %#v", fake.stock)
	}
	var pantryList int
	for _, obj := range fake.objects["shopping_lists"] {
		if strField(obj, "name") == "Pantry" {
			pantryList = numField(obj, "id")
		}
		if strField(obj, "name") == "Default" && numField(obj, "id") == pantryList {
			t.Fatal("renamed the default list")
		}
	}
	var onPantry, onDefault int
	for _, obj := range fake.objects["shopping_list"] {
		if numField(obj, "shopping_list_id") == pantryList {
			onPantry++
			if strField(obj, "note") == "pantry-line:line-1" && numField(obj, "product_id") != numField(parent, "id") {
				t.Fatalf("group line product = %v", obj["product_id"])
			}
		}
		if strField(obj, "note") == "leave me" {
			onDefault++
		}
	}
	if onPantry != 2 || onDefault != 1 {
		t.Fatalf("pantry lines %d, default lines %d", onPantry, onDefault)
	}
	for _, body := range fake.bodies {
		if strings.Contains(body, `"price"`) {
			t.Fatalf("sent a price: %s", body)
		}
	}
}

func TestReconcileStockCorrections(t *testing.T) {
	fake := newFakeGrocy()
	srv := fake.server()
	defer srv.Close()
	api, err := NewAPI(srv.URL, fake.key)
	if err != nil {
		t.Fatal(err)
	}
	m, err := OpenMap(filepath.Join(t.TempDir(), "map.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	inAt := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	exp := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	snap := syncsnap.Snapshot{
		Products: []syncsnap.Product{{ID: "p1", Name: "Milk"}},
		Instances: []syncsnap.Instance{{
			ID: "i1", ProductID: "p1", StockInAt: inAt, ExpiresAt: &exp, StockMoveID: "in1",
		}},
		Moves: []syncsnap.Move{{ID: "in1", ProductID: "p1", Direction: "in", Quantity: 1, At: inAt}},
	}
	rec := &Reconciler{Pantry: &fixedSource{snap: snap}, Grocy: api, Map: m, Loc: time.UTC}
	if err := rec.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Used, and the stock-in is still in the history: consume, don't undo the purchase.
	used := snap
	used.Instances = nil
	used.Removals = []syncsnap.Removal{{InstanceID: "i1", ProductID: "p1", Reason: "expired", StockMoveID: "in1", RemovedByMoveID: "out1"}}
	used.Moves = append(used.Moves, syncsnap.Move{ID: "out1", ProductID: "p1", Direction: "out", Quantity: 1, At: inAt.Add(time.Hour)})
	rec.Pantry = &fixedSource{snap: used}
	if err := rec.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.stock) != 0 {
		t.Fatalf("stock after use = %#v", fake.stock)
	}
	if fake.count("POST", "/consume") != 1 {
		t.Fatalf("consumes = %d", fake.count("POST", "/consume"))
	}
	// The use was undone in Pantry. The unit is back, so the consume booking is undone.
	rec.Pantry = &fixedSource{snap: snap}
	if err := rec.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.stock) != 1 {
		t.Fatalf("stock after undo use = %#v", fake.stock)
	}
	// The stock-in itself was undone. No move, no unit: undo the purchase.
	rec.Pantry = &fixedSource{snap: syncsnap.Snapshot{Products: snap.Products}}
	if err := rec.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.stock) != 0 {
		t.Fatalf("stock after undo purchase = %#v", fake.stock)
	}
}

func TestReconcileRewritesPurchaseDate(t *testing.T) {
	fake := newFakeGrocy()
	srv := fake.server()
	defer srv.Close()
	api, err := NewAPI(srv.URL, fake.key)
	if err != nil {
		t.Fatal(err)
	}
	m, err := OpenMap(filepath.Join(t.TempDir(), "map.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	inAt := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	snap := syncsnap.Snapshot{
		Products:  []syncsnap.Product{{ID: "p1", Name: "Milk"}},
		Instances: []syncsnap.Instance{{ID: "i1", ProductID: "p1", StockInAt: inAt, StockMoveID: "in1"}},
		Moves:     []syncsnap.Move{{ID: "in1", ProductID: "p1", Direction: "in", Quantity: 1, At: inAt}},
	}
	rec := &Reconciler{Pantry: &fixedSource{snap: snap}, Grocy: api, Map: m, Loc: time.UTC}
	if err := rec.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	later := inAt.Add(48 * time.Hour)
	snap.Instances[0].StockInAt = later
	snap.Moves[0].At = later
	rec.Pantry = &fixedSource{snap: snap}
	if err := rec.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.stock) != 1 || fake.stock[0].Purchased != "2026-03-03" {
		t.Fatalf("stock = %#v", fake.stock)
	}
	if fake.count("POST", "/undo") < 1 {
		t.Fatal("expected the old booking to be undone")
	}
}

func TestReconcileDeletesAProductThatLeft(t *testing.T) {
	fake := newFakeGrocy()
	srv := fake.server()
	defer srv.Close()
	api, err := NewAPI(srv.URL, fake.key)
	if err != nil {
		t.Fatal(err)
	}
	m, err := OpenMap(filepath.Join(t.TempDir(), "map.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	qty := 500.0
	snap := syncsnap.Snapshot{
		Products: []syncsnap.Product{{
			ID: "p1", Name: "Milk", Category: "Dairy",
			NetBaseValue: floatPtr(250), NetDimension: "volume", UnitOfMeasure: "carton",
		}},
		Groups: []syncsnap.Group{{
			ID: "g1", Name: "Milk", Rule: "favorite", PinnedProductID: "p1",
			QuantityBase: &qty, Dimension: "volume", MemberIDs: []string{"p1"},
		}},
	}
	snap.Products[0].GroupID = "g1"
	rec := &Reconciler{Pantry: &fixedSource{snap: snap}, Grocy: api, Map: m, Loc: time.UTC}
	if err := rec.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec.Pantry = &fixedSource{}
	if err := rec.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(fake.objects["products"]); n != 0 {
		t.Fatalf("products left = %d", n)
	}
	if n := len(fake.objects["product_groups"]); n != 0 {
		t.Fatalf("labels left = %d", n)
	}
}

func floatPtr(v float64) *float64 { return &v }

func TestReconcileAdoptsExistingProductAfterMapLoss(t *testing.T) {
	fake := newFakeGrocy()
	srv := fake.server()
	defer srv.Close()
	api, err := NewAPI(srv.URL, fake.key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	m, err := OpenMap(filepath.Join(dir, "map.db"))
	if err != nil {
		t.Fatal(err)
	}
	snap := syncsnap.Snapshot{Products: []syncsnap.Product{{ID: "p1", Name: "Milk"}}}
	rec := &Reconciler{Pantry: &fixedSource{snap: snap}, Grocy: api, Map: m, Loc: time.UTC}
	if err := rec.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.Close()
	fresh, err := OpenMap(filepath.Join(dir, "other.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	rec.Map = fresh
	if err := rec.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(fake.objects["products"]); n != 1 {
		t.Fatalf("products = %d", n)
	}
}

type fixedSource struct {
	snap syncsnap.Snapshot
}

func (s *fixedSource) Snapshot(context.Context) (syncsnap.Snapshot, error) {
	return s.snap, nil
}

func hasAll(got []string, want ...string) bool {
	have := map[string]bool{}
	for _, g := range got {
		have[g] = true
	}
	for _, w := range want {
		if !have[w] {
			return false
		}
	}
	return true
}
