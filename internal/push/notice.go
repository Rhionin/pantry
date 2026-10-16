package push

import (
	"strconv"
	"strings"

	"github.com/Rhionin/stockbot/internal/scan"
)

// Notice is one alert. Title, Body, Tag, and URL are the words the service
// worker can show. Count and Names belong to a batch: Notify sends only as
// many names as fit in a compact payload, not the whole list.
type Notice struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Tag   string   `json:"tag"`
	URL   string   `json:"url"`
	Count int      `json:"-"`
	Names []string `json:"-"`
}

func failureNotice(failure scan.ProcessingFailure) Notice {
	return Notice{
		Title: "Scan failed",
		Body:  sentence(failure.Message, "The scan didn't finish.") + " Open the scan queue to try again.",
		Tag:   "pantry-scan-failure-" + noticeID(failure.ID, failure.Barcode),
		URL:   "/",
	}
}

func qrNotice(entry scan.ScanEntry) Notice {
	preview := scan.QRPreview(entry.Barcode)
	body := "Rescan the barcode."
	if preview != "" {
		body = preview + ". Rescan the barcode."
	}
	return Notice{
		Title: "This was a QR code, not the barcode",
		Body:  body,
		Tag:   "pantry-scan-qr-" + noticeID(entry.ID, ""),
		URL:   queuePath(reviewDirection(entry.Direction)),
	}
}

func unrecognizedNotice(entry scan.ScanEntry) Notice {
	barcode := strings.TrimSpace(entry.Barcode)
	subject := "This barcode"
	if barcode != "" {
		subject = "Barcode " + barcode
	}
	direction := reviewDirection(entry.Direction)
	return Notice{
		Title: "Unrecognized product",
		Body:  subject + " needs a product. Open the scan queue to match it.",
		Tag:   "pantry-scan-unrecognized-" + noticeID(entry.ID, barcode),
		URL:   queuePath(direction),
	}
}

func batchNotice(direction scan.ScanDirection, count int, names []string) Notice {
	label := "Stock out"
	if direction == scan.StockIn {
		label = "Stock in"
	}
	waiting := "1 scan is waiting"
	pronoun := "it"
	if count != 1 {
		waiting = strconv.Itoa(count) + " scans are waiting"
		pronoun = "them"
	}
	return Notice{
		Title: label + " batch is ready",
		Body:  waiting + ". Open the scan queue to confirm " + pronoun + ".",
		Tag:   "pantry-scan-batch-" + string(direction),
		URL:   queuePath(direction),
		Count: count,
		Names: names,
	}
}

func productName(entry scan.ScanEntry) string {
	if entry.Product == nil {
		return ""
	}
	return strings.TrimSpace(entry.Product.Name)
}

func reviewDirection(direction *scan.ScanDirection) scan.ScanDirection {
	if direction != nil && *direction == scan.StockIn {
		return scan.StockIn
	}
	return scan.StockOut
}

func queuePath(direction scan.ScanDirection) string {
	if direction == scan.StockIn || direction == scan.StockOut {
		return "/?scanAlert=" + string(direction)
	}
	return "/"
}

func noticeID(id, barcode string) string {
	if id != "" {
		return id
	}
	if barcode != "" {
		return barcode
	}
	return "scan"
}

func sentence(message, fallback string) string {
	text := strings.TrimSpace(message)
	if text == "" {
		text = fallback
	}
	if text == "" {
		return text
	}
	switch text[len(text)-1] {
	case '.', '!', '?':
		return text
	default:
		return text + "."
	}
}
