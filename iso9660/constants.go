package iso9660

const (
	SectorSize             = 2048
	JolietMaxFilenameChars = 64
	SystemAreaNumSectors   = 16 // # of blank sectors at the beginning of the ISO

	// vdTypePrimary identifies a Primary Volume Descriptor
	vdTypePrimary byte = 1
	// vdTypeSupplementary identifies a Supplementary Volume Descriptor (used for Joliet)
	vdTypeSupplementary byte = 2
	// vdTypeTerminator identifies a Volume Descriptor Set Terminator
	vdTypeTerminator byte = 255

	// drFixedPartSize is the size of a Directory Record excluding identifier-related fields
	// (ECMA-119 Section 9.1)
	drFixedPartSize = 33
	// ptRecFixedPartSize is the size of a Path Table Record excluding identifier-related fields
	// (LenDI (1), ExtAttrLen (1), LocExtent (4), ParentDirNum (2))
	// (ECMA-119 Section 9.4)
	ptRecFixedPartSize = 8
)
