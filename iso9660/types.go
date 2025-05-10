package iso9660

// volumeDescriptorHeader is common to PVD, SVD, Terminator.
// (ECMA-119 Section 8.4.1, 8.5.1, 8.6.1)
type volumeDescriptorHeader struct {
	Type               byte    // vdTypePrimary, vdTypeSupplementary, or vdTypeTerminator
	StandardIdentifier [5]byte // "CD001"
	Version            byte    // should be 1
}

// primaryVolumeDescriptorFields holds fields for a Primary Volume Descriptor,
// *excluding* the common 7-byte header and trailing application-use/reserved areas.
// -> ECMA-119 Section 8.4 for field details.
type primaryVolumeDescriptorFields struct {
	// byte 7: unused (1 byte)
	SystemIdentifier [32]byte // d-characters or a-characters
	VolumeIdentifier [32]byte // d-characters
	// bytes 72-79: unused (8 bytes)
	VolumeSpaceSize uint32 // size of logical blocks in the volume
	// bytes 88-119: unused (32 bytes), for Escape Sequences in ISO 9660:1999
	VolumeSetSize        uint16 // num. of volumes in the set (usually 1)
	VolumeSequenceNumber uint16 // sequence number of this volume in the set (usually 1)
	LogicalBlockSize     uint16 // size of a logical block (must be SectorSize)
	PathTableSizeBytes   uint32 // total size in bytes of the L-Type Path Table
	// LBA locations for Path Tables (Type L and Type M, first and second/optional copies)
	LPathTableLocation          uint32
	OptionalLPathTableLocation  uint32
	MPathTableLocation          uint32
	OptionalMPathTableLocation  uint32
	RootDirectoryRecord         [34]byte  // directoryRecord for the root directory
	VolumeSetIdentifier         [128]byte // d-characters
	PublisherIdentifier         [128]byte // a-characters
	DataPreparerIdentifier      [128]byte // ^
	ApplicationIdentifier       [128]byte // ^
	CopyrightFileIdentifier     [37]byte  // d-chars, d1-chars, filename
	AbstractFileIdentifier      [37]byte  // ^
	BibliographicFileIdentifier [37]byte  // ^
	VolumeCreationTimestamp     [17]byte  // decimal digits, offset
	VolumeModificationTimestamp [17]byte
	VolumeExpirationTimestamp   [17]byte // zero for "not specified"
	VolumeEffectiveTimestamp    [17]byte
	FileStructureVersion        byte // needs to be be 1
	// byte 882: unused (1 byte)
	// bytes 883-1394: Application Use (512 bytes) - zeroed
	// bytes 1395-2047: Reserved (653 bytes) - zeroed
}

// supplementaryVolumeDescriptorFields holds specific fields for a Supplementary Volume Descriptor (e.g., Joliet).
// -> ECMA-119 Section 8.5 for details
type supplementaryVolumeDescriptorFields struct {
	// byte 7: Volume Flags (1 byte) - 0 for basic Joliet
	SystemIdentifier [32]byte // os name or space
	VolumeIdentifier [32]byte // UCS-2BE for Joliet (max 16 chars)
	// bytes 72-79: unused (8 bytes)
	VolumeSpaceSize             uint32
	EscapeSequences             [32]byte // Joliet UCS level -> {'%', '/', 'E'} - Level 3
	VolumeSetSize               uint16
	VolumeSequenceNumber        uint16
	LogicalBlockSize            uint16
	PathTableSizeBytes          uint32
	LPathTableLocation          uint32
	OptionalLPathTableLocation  uint32
	MPathTableLocation          uint32
	OptionalMPathTableLocation  uint32
	RootDirectoryRecord         [34]byte  // DirectoryRecord for root (Joliet format)
	VolumeSetIdentifier         [128]byte // UCS-2BE for Joliet (max 64 chars)
	PublisherIdentifier         [128]byte // ^
	DataPreparerIdentifier      [128]byte // ^
	ApplicationIdentifier       [128]byte // ^
	CopyrightFileIdentifier     [37]byte  // UCS-2BE (max 18 chars + padding byte)
	AbstractFileIdentifier      [37]byte  // ^
	BibliographicFileIdentifier [37]byte  // ^
	VolumeCreationTimestamp     [17]byte
	VolumeModificationTimestamp [17]byte
	VolumeExpirationTimestamp   [17]byte
	VolumeEffectiveTimestamp    [17]byte
	FileStructureVersion        byte
}

// directoryRecordFields represents the fixed-size part of a Directory Record
// -> (ECMA-119 Section 9.1) for details
// : The variable-length identifier and its padding are handled during marshalling.
type directoryRecordFields struct {
	ExtendedAttributeRecordLength byte    // 0
	LocationExtent                uint32  // abs LBA of the file's data or directory's extent
	DataLength                    uint32  // size of file data or directory extent in bytes
	RecordingTime                 [7]byte // year(since 1900),month,dday,hour,min,sec,GMTOffset
	FileFlags                     byte    // bits for `Hidden, Directory, Associated`, etc.
	FileUnitSize                  byte    // interleaved files
	InterleaveGapSize             byte    // ^
	VolumeSequenceNumber          uint16  // volume number (usually 1)
}

// pathTableRecordFields represents fixed-size part of a Path Table Record
// -> ECMA-119 Section 9.4 for details
type pathTableRecordFields struct {
	ExtendedAttributeRecordLength byte
	LocationOfExtent              uint32
	ParentDirectoryNumber         uint16 // Path Table directory number of the parent directory
}

// fileEntry is the internal representation of a scanned file or directory from the source filesystem.
// It holds metadata needed to construct the ISO image.
type fileEntry struct {
	originalName string // og filename component
	diskPath     string // full path on the source disk
	isoPath      string // path relative to ISO root

	isDir       bool
	level       int   // depth in directory tree (root is 0)
	parentIndex int   // index in ISOBuilder.fileEntries slice of this entry's parent
	children    []int // indices of children fileEntry items

	iso9660Name string // ISO9660 sanitized name (e.g., "MY_DOC.TXT;1")
	jolietName  string // Joliet name

	// files: actual data length in bytes.
	// directories: sector-aligned byte size of their directory listing extent.
	iso9660Size uint32
	jolietSize  uint32

	// abs LBA of the start of the content.
	// files: LBA of file data. iso9660Sector and jolietSector will point to the same LBA.
	// directories: LBA of their specific (ISO9660 or Joliet) directory listing extent.
	iso9660Sector uint32
	jolietSector  uint32

	// actualISO9660DrSize and actualJolietDrSize store the exact byte length
	// (including padding) of the Directory Record for this entry when it appears
	// as a child in its parent's directory listing, or in PVD/SVD for the root.
	actualISO9660DrSize int
	actualJolietDrSize  int

	pathTableDirNum uint16 // number for directories in path tables (1 for root)
	isHidden        bool   // mark file as hidden in Directory Records
}
