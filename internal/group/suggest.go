package group

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

// RefreshSuggestions inserts look-alike cards that are not already grouped,
// already open, or fully dismissed. A second call does not copy them.
func (g *Groups) RefreshSuggestions(ctx context.Context) error {
	tx, err := g.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not refresh suggestions: %w", err)
	}
	defer tx.Rollback()
	if err := refreshSuggestions(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not refresh suggestions: %w", err)
	}
	return nil
}

type namedProduct struct {
	id   string
	name string
}

func refreshSuggestions(ctx context.Context, tx *sql.Tx) error {
	products, err := loadNamedProducts(ctx, tx)
	if err != nil {
		return err
	}
	if len(products) < 2 {
		return nil
	}
	groupOf, extraKeys, err := loadMembership(ctx, tx)
	if err != nil {
		return err
	}
	dismissed, err := loadDismissed(ctx, tx)
	if err != nil {
		return err
	}
	openSets, err := loadOpenSets(ctx, tx)
	if err != nil {
		return err
	}

	names := make([]string, len(products))
	for i, p := range products {
		names[i] = p.name
	}
	matches := ResolveLookAlikes(names, extraKeys)
	byKey := map[string][]int{}
	for i, match := range matches {
		if match.Key == "" {
			continue
		}
		byKey[match.Key] = append(byKey[match.Key], i)
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		indexes := byKey[key]
		if len(indexes) < 2 {
			continue
		}
		ids := make([]string, len(indexes))
		cautionOf := map[string]string{}
		for i, idx := range indexes {
			ids[i] = products[idx].id
			cautionOf[ids[i]] = matches[idx].Caution
		}
		sort.Strings(ids)
		for _, piece := range splitDismissed(ids, dismissed) {
			if len(piece) < 2 {
				continue
			}
			existing, skip := existingGroup(piece, groupOf)
			if skip {
				continue
			}
			sig := strings.Join(piece, "\n")
			if _, already := openSets[sig]; already {
				continue
			}
			if err := insertSuggestion(ctx, tx, titleFromKey(key), existing, piece, cautionOf); err != nil {
				return err
			}
			openSets[sig] = struct{}{}
		}
	}
	return nil
}

func loadNamedProducts(ctx context.Context, q querier) ([]namedProduct, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, name FROM products ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("could not refresh suggestions: %w", err)
	}
	defer rows.Close()
	var out []namedProduct
	for rows.Next() {
		var p namedProduct
		if err := rows.Scan(&p.id, &p.name); err != nil {
			return nil, fmt.Errorf("could not refresh suggestions: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not refresh suggestions: %w", err)
	}
	return out, nil
}

func loadMembership(ctx context.Context, q querier) (map[string]string, []string, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT m.product_id, m.group_id, g.name
		FROM product_group_members m
		JOIN product_groups g ON g.id = m.group_id
		WHERE g.user_id = ?`, householdUser)
	if err != nil {
		return nil, nil, fmt.Errorf("could not refresh suggestions: %w", err)
	}
	defer rows.Close()
	groupOf := map[string]string{}
	seen := map[string]struct{}{}
	var extra []string
	for rows.Next() {
		var productID, groupID, name string
		if err := rows.Scan(&productID, &groupID, &name); err != nil {
			return nil, nil, fmt.Errorf("could not refresh suggestions: %w", err)
		}
		groupOf[productID] = groupID
		key, _ := LookAlikeKey(name, nil)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		extra = append(extra, key)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("could not refresh suggestions: %w", err)
	}
	nameRows, err := q.QueryContext(ctx, `SELECT name FROM product_groups WHERE user_id = ?`, householdUser)
	if err != nil {
		return nil, nil, fmt.Errorf("could not refresh suggestions: %w", err)
	}
	defer nameRows.Close()
	for nameRows.Next() {
		var name string
		if err := nameRows.Scan(&name); err != nil {
			return nil, nil, fmt.Errorf("could not refresh suggestions: %w", err)
		}
		key, _ := LookAlikeKey(name, nil)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		extra = append(extra, key)
	}
	if err := nameRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("could not refresh suggestions: %w", err)
	}
	return groupOf, extra, nil
}

func loadDismissed(ctx context.Context, q querier) (map[string]struct{}, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT product_id_a, product_id_b FROM group_suggestion_dismissals WHERE user_id = ?`, householdUser)
	if err != nil {
		return nil, fmt.Errorf("could not refresh suggestions: %w", err)
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, fmt.Errorf("could not refresh suggestions: %w", err)
		}
		out[pairKey(a, b)] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not refresh suggestions: %w", err)
	}
	return out, nil
}

func loadOpenSets(ctx context.Context, q querier) (map[string]struct{}, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT m.suggestion_id, m.product_id
		FROM group_suggestion_members m
		JOIN group_suggestions s ON s.id = m.suggestion_id
		WHERE s.user_id = ? AND s.status = 'open'
		ORDER BY m.suggestion_id, m.product_id`, householdUser)
	if err != nil {
		return nil, fmt.Errorf("could not refresh suggestions: %w", err)
	}
	defer rows.Close()
	sets := map[string][]string{}
	var order []string
	for rows.Next() {
		var sid, pid string
		if err := rows.Scan(&sid, &pid); err != nil {
			return nil, fmt.Errorf("could not refresh suggestions: %w", err)
		}
		if _, ok := sets[sid]; !ok {
			order = append(order, sid)
		}
		sets[sid] = append(sets[sid], pid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not refresh suggestions: %w", err)
	}
	out := map[string]struct{}{}
	for _, sid := range order {
		ids := sets[sid]
		sort.Strings(ids)
		out[strings.Join(ids, "\n")] = struct{}{}
	}
	return out, nil
}

func existingGroup(ids []string, groupOf map[string]string) (string, bool) {
	seen := map[string]struct{}{}
	var only string
	ungrouped := 0
	for _, id := range ids {
		gid := groupOf[id]
		if gid == "" {
			ungrouped++
			continue
		}
		seen[gid] = struct{}{}
		only = gid
	}
	if len(seen) > 1 {
		return "", true
	}
	if len(seen) == 1 && ungrouped == 0 {
		return "", true
	}
	if len(seen) == 1 {
		return only, false
	}
	return "", false
}

func splitDismissed(ids []string, dismissed map[string]struct{}) [][]string {
	rest := append([]string(nil), ids...)
	sort.Strings(rest)
	var cards [][]string
	for len(rest) >= 2 {
		keep := append([]string(nil), rest...)
		for {
			drop := dismissedIndex(keep, dismissed)
			if drop < 0 {
				break
			}
			keep = append(keep[:drop], keep[drop+1:]...)
		}
		if len(keep) >= 2 {
			cards = append(cards, keep)
		}
		next := difference(rest, keep)
		if len(next) == len(rest) {
			break
		}
		rest = next
	}
	return cards
}

func dismissedIndex(ids []string, dismissed map[string]struct{}) int {
	best := -1
	bestCount := 0
	for i, id := range ids {
		n := 0
		for j, other := range ids {
			if i == j {
				continue
			}
			if _, ok := dismissed[pairKey(id, other)]; ok {
				n++
			}
		}
		if n == 0 {
			continue
		}
		if best < 0 || n > bestCount || (n == bestCount && id > ids[best]) {
			best = i
			bestCount = n
		}
	}
	return best
}

func difference(all, keep []string) []string {
	in := map[string]struct{}{}
	for _, id := range keep {
		in[id] = struct{}{}
	}
	var out []string
	for _, id := range all {
		if _, ok := in[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}

func pairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "\n" + b
}

func titleFromKey(key string) string {
	r := []rune(strings.TrimSpace(key))
	if len(r) == 0 {
		return "Group"
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func majorityCaution(ids []string, cautionOf map[string]string) string {
	counts := map[string]int{}
	for _, id := range ids {
		counts[cautionOf[id]]++
	}
	labels := make([]string, 0, len(counts))
	for label := range counts {
		labels = append(labels, label)
	}
	sort.Slice(labels, func(i, j int) bool {
		if counts[labels[i]] != counts[labels[j]] {
			return counts[labels[i]] > counts[labels[j]]
		}
		if labels[i] == "" {
			return true
		}
		if labels[j] == "" {
			return false
		}
		return labels[i] < labels[j]
	})
	return labels[0]
}

func insertSuggestion(ctx context.Context, tx *sql.Tx, title, existing string, ids []string, cautionOf map[string]string) error {
	id := uuid.NewString()
	var existingVal any
	if existing != "" {
		existingVal = existing
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO group_suggestions (id, user_id, kind, title, existing_group_id, status)
		VALUES (?, ?, 'looks_alike', ?, ?, 'open')`,
		id, householdUser, title, existingVal); err != nil {
		return fmt.Errorf("could not refresh suggestions: %w", err)
	}
	style := majorityCaution(ids, cautionOf)
	for _, productID := range ids {
		caution := cautionOf[productID]
		included := 1
		if caution != style {
			included = 0
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO group_suggestion_members (suggestion_id, product_id, included, caution)
			VALUES (?, ?, ?, ?)`,
			id, productID, included, caution); err != nil {
			return fmt.Errorf("could not refresh suggestions: %w", err)
		}
	}
	return nil
}
