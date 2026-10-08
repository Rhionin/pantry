package group

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Hint is the one line a scan row shows when a product looks like a group.
type Hint struct {
	GroupID string `json:"groupId,omitempty"`
	Name    string `json:"name"`
}

type hintGroup struct {
	id      string
	name    string
	nameKey string
	keys    map[string]struct{}
	members map[string]struct{}
}

// Hints matches scanned products to existing groups in one pass.
// A product that already belongs to a group, or that already has a from-scan
// suggestion, is left out so the row does not ask again.
func (g *Groups) Hints(ctx context.Context, names map[string]string) (map[string]Hint, error) {
	if len(names) == 0 {
		return map[string]Hint{}, nil
	}
	return hintsFor(ctx, g.db, names)
}

func hintsFor(ctx context.Context, q querier, names map[string]string) (map[string]Hint, error) {
	groups, memberOf, err := loadHintGroups(ctx, q)
	if err != nil {
		return nil, err
	}
	noted, err := loadNotedFromScan(ctx, q)
	if err != nil {
		return nil, err
	}
	known := map[string]struct{}{}
	for i := range groups {
		if groups[i].nameKey != "" {
			known[groups[i].nameKey] = struct{}{}
		}
		for key := range groups[i].keys {
			known[key] = struct{}{}
		}
	}
	for i := range groups {
		resolved := map[string]struct{}{}
		if groups[i].nameKey != "" {
			resolved[groups[i].nameKey] = struct{}{}
		}
		for key := range groups[i].keys {
			short, _ := LookAlikeKey(key, known)
			if short == "" {
				short = key
			}
			resolved[short] = struct{}{}
			resolved[key] = struct{}{}
		}
		groups[i].keys = resolved
	}

	out := map[string]Hint{}
	for productID, name := range names {
		if _, grouped := memberOf[productID]; grouped {
			continue
		}
		if _, already := noted[productID]; already {
			continue
		}
		key, _ := LookAlikeKey(name, known)
		if key == "" {
			continue
		}
		var exact, loose *hintGroup
		for i := range groups {
			if _, ok := groups[i].keys[key]; !ok {
				continue
			}
			if groups[i].nameKey == key {
				exact = &groups[i]
				break
			}
			if loose == nil {
				loose = &groups[i]
			}
		}
		pick := exact
		if pick == nil {
			pick = loose
		}
		if pick == nil {
			continue
		}
		out[productID] = Hint{GroupID: pick.id, Name: pick.name}
	}
	return out, nil
}

func loadHintGroups(ctx context.Context, q querier) ([]hintGroup, map[string]string, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT g.id, g.name, m.product_id, COALESCE(p.name, '')
		FROM product_groups g
		LEFT JOIN product_group_members m ON m.group_id = g.id
		LEFT JOIN products p ON p.id = m.product_id
		WHERE g.user_id = ?
		ORDER BY g.name, g.id`, householdUser)
	if err != nil {
		return nil, nil, fmt.Errorf("could not match this scan to a group: %w", err)
	}
	defer rows.Close()

	byID := map[string]*hintGroup{}
	var order []string
	memberOf := map[string]string{}
	for rows.Next() {
		var id, name string
		var productID sql.NullString
		var productName string
		if err := rows.Scan(&id, &name, &productID, &productName); err != nil {
			return nil, nil, fmt.Errorf("could not match this scan to a group: %w", err)
		}
		g, ok := byID[id]
		if !ok {
			nameKey, _ := LookAlikeKey(name, nil)
			g = &hintGroup{
				id:      id,
				name:    name,
				nameKey: nameKey,
				keys:    map[string]struct{}{},
				members: map[string]struct{}{},
			}
			byID[id] = g
			order = append(order, id)
		}
		if !productID.Valid {
			continue
		}
		g.members[productID.String] = struct{}{}
		memberOf[productID.String] = id
		key, _ := LookAlikeKey(productName, nil)
		if key != "" {
			g.keys[key] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("could not match this scan to a group: %w", err)
	}
	out := make([]hintGroup, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name < out[j].name
		}
		return out[i].id < out[j].id
	})
	return out, memberOf, nil
}

func loadNotedFromScan(ctx context.Context, q querier) (map[string]struct{}, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT m.product_id
		FROM group_suggestion_members m
		JOIN group_suggestions s ON s.id = m.suggestion_id
		WHERE s.user_id = ? AND s.kind = 'from_scan'`, householdUser)
	if err != nil {
		return nil, fmt.Errorf("could not match this scan to a group: %w", err)
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("could not match this scan to a group: %w", err)
		}
		out[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not match this scan to a group: %w", err)
	}
	return out, nil
}

// NoteFromScan records one from-scan suggestion for a product.
// A second call for the same product does nothing, including after dismiss.
func (g *Groups) NoteFromScan(ctx context.Context, productID string) error {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return invalid("Choose a product.")
	}
	tx, err := g.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not save that for later: %w", err)
	}
	defer tx.Rollback()

	var name string
	err = tx.QueryRowContext(ctx, `SELECT name FROM products WHERE id = ?`, productID).Scan(&name)
	if err == sql.ErrNoRows {
		return missing("That product was not found.")
	}
	if err != nil {
		return fmt.Errorf("could not save that for later: %w", err)
	}

	noted, err := loadNotedFromScan(ctx, tx)
	if err != nil {
		return err
	}
	if _, already := noted[productID]; already {
		return tx.Commit()
	}

	matched, err := hintsFor(ctx, tx, map[string]string{productID: name})
	if err != nil {
		return err
	}
	hint := matched[productID]
	title := name
	var existing any
	if hint.GroupID != "" {
		title = hint.Name
		existing = hint.GroupID
	}
	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO group_suggestions (id, user_id, kind, title, existing_group_id, status)
		VALUES (?, ?, 'from_scan', ?, ?, 'open')`,
		id, householdUser, title, existing); err != nil {
		return fmt.Errorf("could not save that for later: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO group_suggestion_members (suggestion_id, product_id, included, caution)
		VALUES (?, ?, 1, '')`,
		id, productID); err != nil {
		return fmt.Errorf("could not save that for later: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not save that for later: %w", err)
	}
	return nil
}
