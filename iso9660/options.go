package iso9660

// Options configures the ISO image creation.
type Options struct {
	VolumeIdentifierISO          string  // PVD, max 32 d-characters (e.g., "Whatever")
	VolumeIdentifierJoliet       string  // SVD, max 16 UCS-2 characters (e.g., "Whatever")
	SystemIdentifier             string  // PVD/SVD, max 32 a-characters (e.g., "WINDOWS", "LINUX", or whatever you want)
	PublisherIdentifierISO       string  // PVD, max 128 a-characters
	PublisherIdentifierJoliet    string  // SVD, max 64 UCS-2 characters
	DataPreparerIdentifierISO    string  // PVD, max 128 a-characters
	DataPreparerIdentifierJoliet string  // SVD, max 64 UCS-2 characters
	ApplicationIdentifierISO     string  // PVD, max 128 a-characters
	ApplicationIdentifierJoliet  string  // SVD, max 64 UCS-2 characters
	JolietEscapeSequence         [3]byte // Joliet UCS level -> {'%', '/', 'E'} - Level 3
}

// DefaultOptions returns a new Options struct with sensible defaults.
func DefaultOptions() *Options {
	return &Options{
		VolumeIdentifierISO:          "ISO_VOLUME",    // default
		VolumeIdentifierJoliet:       "JOLIET_VOLUME", // ^
		SystemIdentifier:             " ",             // blank or OS-specific
		PublisherIdentifierISO:       "CharlesTheGreat (j) goiso9660",
		PublisherIdentifierJoliet:    "",
		DataPreparerIdentifierISO:    "",
		DataPreparerIdentifierJoliet: "",
		ApplicationIdentifierISO:     "goiso9660",
		ApplicationIdentifierJoliet:  "goiso9660 joliet",
		JolietEscapeSequence:         [3]byte{'%', '/', 'E'}, // UCS-2 Level 3
	}
}
