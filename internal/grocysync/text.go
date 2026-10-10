package grocysync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Rhionin/pantry/internal/syncsnap"
)

const (
	neverExpires   = "2999-12-31"
	barcodeNote    = "pantry"
	historyPrefix  = "pantry-history:"
	instancePrefix = "pantry-instance:"
	linePrefix     = "pantry-line:"
)

func markerLine(id string) string {
	return "pantry:" + id
}

func description(id string, lines ...string) string {
	var b strings.Builder
	b.WriteString(markerLine(id))
	for _, line := range lines {
		if line == "" {
			continue
		}
		b.WriteByte('\n')
		b.WriteString(line)
	}
	return b.String()
}

func markerID(description string) string {
	line, _, _ := strings.Cut(description, "\n")
	line = strings.TrimSpace(line)
	const prefix = "pantry:"
	if !strings.HasPrefix(line, prefix) {
		return ""
	}
	return strings.TrimPrefix(line, prefix)
}

func sizeNote(p syncsnap.Product) string {
	var parts []string
	if p.NetBaseValue != nil && p.NetDimension != "" {
		unit := "g"
		if p.NetDimension == "volume" {
			unit = "ml"
		}
		parts = append(parts, fmt.Sprintf("%g %s", *p.NetBaseValue, unit))
	}
	if p.PackCount != nil {
		parts = append(parts, fmt.Sprintf("pack of %d", *p.PackCount))
	}
	if p.UnitOfMeasure != "" {
		parts = append(parts, p.UnitOfMeasure)
	}
	return strings.Join(parts, ", ")
}

func ruleNote(g syncsnap.Group) string {
	parts := []string{g.Rule}
	if g.PinnedProductID != "" {
		parts = append(parts, "pinned "+g.PinnedProductID)
	}
	if g.WindowMonths != nil {
		parts = append(parts, fmt.Sprintf("window %d months", *g.WindowMonths))
	}
	if g.QuantityBase != nil && g.Dimension != "" {
		unit := "g"
		if g.Dimension == "volume" {
			unit = "ml"
		}
		parts = append(parts, fmt.Sprintf("%g %s", *g.QuantityBase, unit))
	}
	return strings.Join(parts, "; ")
}

func productFields(id, size, rule string) map[string]string {
	return map[string]string{
		"pantry_id":   id,
		"pantry_size": size,
		"pantry_rule": rule,
	}
}

func hashState(body any, fields map[string]string) string {
	raw, err := json.Marshal(struct {
		Body   any               `json:"body"`
		Fields map[string]string `json:"fields"`
	}{body, fields})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

func day(t time.Time, loc *time.Location) string {
	if t.IsZero() {
		return ""
	}
	if loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format("2006-01-02")
}

func stamp(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format("2006-01-02 15:04:05")
}

func sameDay(a, b string) bool {
	return datePrefix(a) == datePrefix(b)
}

func datePrefix(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func bestBefore(t *time.Time, loc *time.Location) string {
	if t == nil || t.IsZero() {
		return neverExpires
	}
	return day(*t, loc)
}

func instanceHash(inst syncsnap.Instance) string {
	exp := ""
	if inst.ExpiresAt != nil {
		exp = inst.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return strings.Join([]string{
		inst.ProductID,
		inst.StockInAt.UTC().Format(time.RFC3339),
		exp,
		inst.StockMoveID,
	}, "|")
}

func packDetail(moveID, stockID string) string {
	return moveID + "\n" + stockID
}

func unpackDetail(detail string) (moveID, stockID string) {
	moveID, stockID, _ = strings.Cut(detail, "\n")
	return moveID, stockID
}

func productIDFromHash(hash string) string {
	id, _, _ := strings.Cut(hash, "|")
	return id
}

func labelKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func parentMarker(groupID string) string {
	return "group:" + groupID
}
