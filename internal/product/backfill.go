package product

import (
	"context"
	"database/sql"
	"fmt"
)

// BackfillNetSizes fills net size on products whose size has never been set,
// using a local parse of the package word and then the name. Origin is
// backfill. A row the person cleared (origin manual, value empty) is left
// alone, and a second run does not write those rows again. This does not
// call Open Food Facts.
func BackfillNetSizes(ctx context.Context, db *sql.DB) (int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, COALESCE(unit_of_measure, '')
		FROM products
		WHERE net_size_origin IS NULL
		  AND net_base_value IS NULL
		  AND net_dimension IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("could not read products that need a size: %w", err)
	}
	defer rows.Close()

	type row struct {
		id   string
		name string
		unit string
	}
	var pending []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.id, &item.name, &item.unit); err != nil {
			return 0, fmt.Errorf("could not read products that need a size: %w", err)
		}
		pending = append(pending, item)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("could not read products that need a size: %w", err)
	}

	filled := 0
	for _, item := range pending {
		parsed, ok := parseBackfill(item.unit, item.name)
		if !ok {
			continue
		}
		var pack any
		if parsed.HasPack {
			pack = parsed.PackCount
		}
		res, err := db.ExecContext(ctx, `
			UPDATE products
			SET net_base_value = ?,
			    net_dimension = ?,
			    net_size_origin = ?,
			    pack_count = COALESCE(pack_count, ?)
			WHERE id = ?
			  AND net_size_origin IS NULL
			  AND net_base_value IS NULL
			  AND net_dimension IS NULL`,
			parsed.BaseValue, parsed.Dimension, OriginBackfill, pack, item.id,
		)
		if err != nil {
			return filled, fmt.Errorf("could not save a product size: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return filled, fmt.Errorf("could not save a product size: %w", err)
		}
		filled += int(n)
	}
	return filled, nil
}

func parseBackfill(unit, name string) (ParsedSize, bool) {
	if parsed, ok := ParseNetSize(unit); ok {
		return parsed, true
	}
	return ParseNetSize(name)
}
