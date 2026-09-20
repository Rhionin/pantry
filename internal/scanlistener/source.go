package scanlistener

// Source names where the ScanListener obtains barcodes. It is fixed at
// startup (Requirement 10.6) so the active source is a reportable fact
// rather than a race against device availability.
type Source string

const (
	SourceDevice Source = "device" // default: the appliance path
	SourceStdin  Source = "stdin"  // local development in a terminal
)

// ParseSource maps a configured value to a Source. An unset or empty value
// yields SourceDevice; an unrecognized value yields SourceDevice with
// ok=false so the caller can log the offending input (Requirements 7.5-7.7).
// Defaulting to SourceDevice rather than to stdin is deliberate: a typo must
// not silently select the source that cannot work under a service manager.
func ParseSource(raw string) (Source, bool) {
	switch raw {
	case "", "device":
		return SourceDevice, true
	case "stdin":
		return SourceStdin, true
	default:
		return SourceDevice, false
	}
}
