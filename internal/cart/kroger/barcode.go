// Package kroger provides a grocery cart adapter for Kroger's Cart API.
package kroger

import (
	"fmt"

	"github.com/Rhionin/pantry/internal/cart"
)

// Normalize returns the 13-character Normalized_Barcode for a pantry barcode, or
// ok=false when the input is not 8, 12, or 13 digits (Requirement 17.8) or when
// its carried check digit disagrees with the recomputed one.
func Normalize(barcode string) (cart.ProductIdentity, bool) {
	switch len(barcode) {
	case 8:
		return normalizeUPCE(barcode)
	case 12:
		return normalizeUPCA(barcode)
	case 13:
		return normalizeEAN13(barcode)
	default:
		return "", false
	}
}

// normalizeUPCA handles 12-digit UPC-A / GTIN-12 barcodes.
// Steps: validate check digit, discard it, left-pad the 11 remaining data digits with two '0' characters.
func normalizeUPCA(barcode string) (cart.ProductIdentity, bool) {
	if !validateUPCA(barcode) {
		return "", false
	}

	// Discard check digit (last char), pad remaining 11 digits with two leading zeros
	data := barcode[:11]
	identity := fmt.Sprintf("00%s", data)
	return cart.ProductIdentity(identity), true
}

// normalizeEAN13 handles 13-digit EAN-13 / GTIN-13 barcodes.
// Steps: discard check digit, left-pad the 12 remaining data digits with one '0' character.
func normalizeEAN13(barcode string) (cart.ProductIdentity, bool) {
	if !validateEAN13(barcode) {
		return "", false
	}

	// Discard check digit (last char), pad remaining 12 digits with one leading zero
	data := barcode[:12]
	identity := fmt.Sprintf("0%s", data)
	return cart.ProductIdentity(identity), true
}

// normalizeUPCE handles 8-digit UPC-E barcodes.
// Steps: expand to GTIN-12 per GS1 Table 5-7, validate check digit, discard it, pad with two leading zeros.
func normalizeUPCE(barcode string) (cart.ProductIdentity, bool) {
	if !validateUPCE(barcode) {
		return "", false
	}

	// Expand UPC-E to GTIN-12
	gtin12, ok := expandUPCE(barcode)
	if !ok {
		return "", false
	}

	// Now we have a 12-digit GTIN-12, discard its check digit and pad
	identity := fmt.Sprintf("00%s", gtin12[:11])
	return cart.ProductIdentity(identity), true
}

// validateUPCA validates the check digit of a 12-digit UPC-A barcode.
func validateUPCA(barcode string) bool {
	if len(barcode) != 12 {
		return false
	}
	check := int(barcode[11] - '0')
	sum := 0
	for i := 0; i < 11; i++ {
		digit := int(barcode[i] - '0')
		if i%2 == 0 {
			sum += digit * 3
		} else {
			sum += digit
		}
	}
	return (sum%10 == 0 && check == 0) || (sum%10 != 0 && check == 10-sum%10)
}

// validateEAN13 validates the check digit of a 13-digit EAN-13 barcode.
func validateEAN13(barcode string) bool {
	if len(barcode) != 13 {
		return false
	}
	check := int(barcode[12] - '0')
	sum := 0
	for i := 0; i < 12; i++ {
		digit := int(barcode[i] - '0')
		if i%2 == 0 {
			sum += digit
		} else {
			sum += digit * 3
		}
	}
	return (sum%10 == 0 && check == 0) || (sum%10 != 0 && check == 10-sum%10)
}

// validateUPCE validates the 8-digit UPC-E barcode format and check digit.
// Returns true if format is valid (starts with '0', all digits).
func validateUPCE(barcode string) bool {
	if len(barcode) != 8 {
		return false
	}
	// Must start with '0' (number system digit)
	if barcode[0] != '0' {
		return false
	}
	// All characters must be digits
	for i := 0; i < 8; i++ {
		if barcode[i] < '0' || barcode[i] > '9' {
			return false
		}
	}
	return true
}

// expandUPCE expands an 8-digit UPC-E to a 12-digit GTIN-12 per GS1 Table 5-7.
// Returns ok=false if the barcode is invalid or the expansion fails.
func expandUPCE(barcode string) (string, bool) {
	// S = number system (always '0'), X1-X5 = encoded digits, P6 = 6th encoded, C = check
	S := barcode[0]
	X1 := barcode[1]
	X2 := barcode[2]
	X3 := barcode[3]
	X4 := barcode[4]
	X5 := barcode[5]
	P6 := barcode[6]
	C := barcode[7]

	// Determine expanded digits based on P6
	var expanded string
	switch P6 {
	case '0':
		expanded = string(S) + string(X1) + string(X2) + "00000" + string(X3) + string(X4) + string(X5) + string(C)
	case '1':
		expanded = string(S) + string(X1) + string(X2) + "10000" + string(X3) + string(X4) + string(X5) + string(C)
	case '2':
		expanded = string(S) + string(X1) + string(X2) + "20000" + string(X3) + string(X4) + string(X5) + string(C)
	case '3':
		expanded = string(S) + string(X1) + string(X2) + string(X3) + "00000" + string(X4) + string(X5) + string(C)
	case '4':
		expanded = string(S) + string(X1) + string(X2) + string(X3) + string(X4) + "00000" + string(X5) + string(C)
	case '5', '6', '7', '8', '9':
		expanded = string(S) + string(X1) + string(X2) + string(X3) + string(X4) + string(X5) + "0000" + string(P6) + string(C)
	default:
		return "", false
	}

	// Validate the check digit of the expanded GTIN-12
	if !validateGTIN12(expanded) {
		return "", false
	}

	return expanded, true
}

// validateGTIN12 validates the check digit of a 12-digit GTIN-12.
func validateGTIN12(barcode string) bool {
	if len(barcode) != 12 {
		return false
	}
	check := int(barcode[11] - '0')
	sum := 0
	for i := 0; i < 11; i++ {
		digit := int(barcode[i] - '0')
		if i%2 == 0 {
			sum += digit * 3
		} else {
			sum += digit
		}
	}
	return (sum%10 == 0 && check == 0) || (sum%10 != 0 && check == 10-sum%10)
}
