// The QR card asks the person to scan the barcode on the package. The field
// stays visually hidden; focusing it is what an HID scanner types into.
export function focusBarcodeScanner(): void {
  document.querySelector<HTMLInputElement>('input[aria-label="Barcode scanner input"]')
    ?.focus({ preventScroll: true });
}
