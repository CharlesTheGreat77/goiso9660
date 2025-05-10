package iso9660

import (
	"bytes"
	"encoding/binary"
	"log"
	"time"
)

// marshalBinary converts the header to its byte representation.
func (h *volumeDescriptorHeader) marshalBinary() []byte {
	buf := make([]byte, 7)
	buf[0] = h.Type
	copy(buf[1:6], h.StandardIdentifier[:])
	buf[6] = h.Version
	return buf
}

// createPrimaryVolumeDescriptor generates the PVD sector.
func (b *ISOBuilder) createPrimaryVolumeDescriptor() []byte {
	header := volumeDescriptorHeader{Type: vdTypePrimary, StandardIdentifier: [5]byte{'C', 'D', '0', '0', '1'}, Version: 1}
	headerBytes := header.marshalBinary()

	var pvdFields primaryVolumeDescriptorFields
	copy(pvdFields.SystemIdentifier[:], padString(b.options.SystemIdentifier, 32))
	copy(pvdFields.VolumeIdentifier[:], padString(b.options.VolumeIdentifierISO, 32))
	pvdFields.VolumeSpaceSize = b.totalSectors
	pvdFields.VolumeSetSize = 1
	pvdFields.VolumeSequenceNumber = 1
	pvdFields.LogicalBlockSize = SectorSize
	pvdFields.PathTableSizeBytes = uint32(len(b.pvdPathTableLData)) // size of Type L Path Table
	pvdFields.LPathTableLocation = b.lbaPvdPathTableL
	pvdFields.OptionalLPathTableLocation = b.lbaPvdPathTableL2
	pvdFields.MPathTableLocation = b.lbaPvdPathTableM
	pvdFields.OptionalMPathTableLocation = b.lbaPvdPathTableM2

	rootEntry := b.fileEntries[0] // root is always the first entry
	// Root DR in PVD describes the root directory using ISO9660 naming.
	// Its extent size is b.pvdRootDirExtentSize.
	rootDRBytes, err := b.createDirectoryRecordBytes(rootEntry.iso9660Sector, b.pvdRootDirExtentSize, rootEntry.iso9660Name, &rootEntry, false)
	if err != nil {
		log.Panicf("PVD: Failed to create root directory record: %v", err)
	}
	if len(rootDRBytes) != 34 { // PVD/SVD Root DR is always 34 bytes.
		log.Panicf("PVD: Marshalled Root DR length is %d, expected 34. Identifier was: '%x'", len(rootDRBytes), getDRIdentifierBytes(rootEntry.iso9660Name, false, true))
	}
	copy(pvdFields.RootDirectoryRecord[:], rootDRBytes)

	copy(pvdFields.VolumeSetIdentifier[:], padString("", 128)) // normally blank
	copy(pvdFields.PublisherIdentifier[:], padString(b.options.PublisherIdentifierISO, 128))
	copy(pvdFields.DataPreparerIdentifier[:], padString(b.options.DataPreparerIdentifierISO, 128))
	copy(pvdFields.ApplicationIdentifier[:], padString(b.options.ApplicationIdentifierISO, 128))
	copy(pvdFields.CopyrightFileIdentifier[:], padString("", 37))
	copy(pvdFields.AbstractFileIdentifier[:], padString("", 37))
	copy(pvdFields.BibliographicFileIdentifier[:], padString("", 37))

	now := time.Now().UTC()
	copy(pvdFields.VolumeCreationTimestamp[:], formatTimestamp(now))
	copy(pvdFields.VolumeModificationTimestamp[:], formatTimestamp(now))
	copy(pvdFields.VolumeExpirationTimestamp[:], formatTimestamp(time.Time{})) // zero time for "not specified"
	copy(pvdFields.VolumeEffectiveTimestamp[:], formatTimestamp(now))
	pvdFields.FileStructureVersion = 1

	// manually marshal the PVD fields into a sector-sized buffer
	pvdSectorBytes := make([]byte, SectorSize)
	copy(pvdSectorBytes[0:7], headerBytes) // 7-byte common header

	fieldBuf := new(bytes.Buffer)
	fieldBuf.WriteByte(0) // byte 7: unused (zero)
	fieldBuf.Write(pvdFields.SystemIdentifier[:])
	fieldBuf.Write(pvdFields.VolumeIdentifier[:])
	fieldBuf.Write(make([]byte, 8)) // bytes 72-79: unused (zeros)

	binary.Write(fieldBuf, binary.LittleEndian, pvdFields.VolumeSpaceSize)
	binary.Write(fieldBuf, binary.BigEndian, pvdFields.VolumeSpaceSize)

	fieldBuf.Write(make([]byte, 32)) // bytes 88-119: unused / Escape Sequences (zeros for basic)

	binary.Write(fieldBuf, binary.LittleEndian, pvdFields.VolumeSetSize)
	binary.Write(fieldBuf, binary.BigEndian, pvdFields.VolumeSetSize)
	binary.Write(fieldBuf, binary.LittleEndian, pvdFields.VolumeSequenceNumber)
	binary.Write(fieldBuf, binary.BigEndian, pvdFields.VolumeSequenceNumber)
	binary.Write(fieldBuf, binary.LittleEndian, pvdFields.LogicalBlockSize)
	binary.Write(fieldBuf, binary.BigEndian, pvdFields.LogicalBlockSize)
	binary.Write(fieldBuf, binary.LittleEndian, pvdFields.PathTableSizeBytes)
	binary.Write(fieldBuf, binary.BigEndian, pvdFields.PathTableSizeBytes) // spec says M-Path Table size, but L-size is common if M same

	binary.Write(fieldBuf, binary.LittleEndian, pvdFields.LPathTableLocation)
	binary.Write(fieldBuf, binary.LittleEndian, pvdFields.OptionalLPathTableLocation)
	binary.Write(fieldBuf, binary.BigEndian, pvdFields.MPathTableLocation)
	binary.Write(fieldBuf, binary.BigEndian, pvdFields.OptionalMPathTableLocation)

	fieldBuf.Write(pvdFields.RootDirectoryRecord[:]) // 34 bytes
	fieldBuf.Write(pvdFields.VolumeSetIdentifier[:])
	fieldBuf.Write(pvdFields.PublisherIdentifier[:])
	fieldBuf.Write(pvdFields.DataPreparerIdentifier[:])
	fieldBuf.Write(pvdFields.ApplicationIdentifier[:])
	fieldBuf.Write(pvdFields.CopyrightFileIdentifier[:])
	fieldBuf.Write(pvdFields.AbstractFileIdentifier[:])
	fieldBuf.Write(pvdFields.BibliographicFileIdentifier[:])
	fieldBuf.Write(pvdFields.VolumeCreationTimestamp[:])
	fieldBuf.Write(pvdFields.VolumeModificationTimestamp[:])
	fieldBuf.Write(pvdFields.VolumeExpirationTimestamp[:])
	fieldBuf.Write(pvdFields.VolumeEffectiveTimestamp[:])
	fieldBuf.WriteByte(pvdFields.FileStructureVersion)
	// bytes 883-2047 are Application Use and Reserved, zeroed by make([]byte, SectorSize) initially.
	copy(pvdSectorBytes[7:fieldBuf.Len()+7], fieldBuf.Bytes()) // copy marshalled fields after the common header
	return pvdSectorBytes
}

// createJolietVolumeDescriptor generates the SVD sector for Joliet.
func (b *ISOBuilder) createJolietVolumeDescriptor() []byte {
	header := volumeDescriptorHeader{Type: vdTypeSupplementary, StandardIdentifier: [5]byte{'C', 'D', '0', '0', '1'}, Version: 1}
	headerBytes := header.marshalBinary()

	var svdFields supplementaryVolumeDescriptorFields
	copy(svdFields.SystemIdentifier[:], padString(b.options.SystemIdentifier, 32))
	copy(svdFields.VolumeIdentifier[:], padUTF16StringBE(b.options.VolumeIdentifierJoliet, 16))
	svdFields.VolumeSpaceSize = b.totalSectors
	copy(svdFields.EscapeSequences[0:3], b.options.JolietEscapeSequence[:])
	for i := len(b.options.JolietEscapeSequence); i < 32; i++ {
		svdFields.EscapeSequences[i] = 0x00 // Zero rest of escape sequence field
	}
	svdFields.VolumeSetSize = 1
	svdFields.VolumeSequenceNumber = 1
	svdFields.LogicalBlockSize = SectorSize
	svdFields.PathTableSizeBytes = uint32(len(b.svdPathTableLData)) // L-Type for Joliet
	svdFields.LPathTableLocation = b.lbaSvdPathTableL
	svdFields.OptionalLPathTableLocation = b.lbaSvdPathTableL2
	svdFields.MPathTableLocation = b.lbaSvdPathTableM
	svdFields.OptionalMPathTableLocation = b.lbaSvdPathTableM2

	rootEntry := b.fileEntries[0]
	// Root DR in SVD describes the root directory using Joliet naming.
	rootDRJolietBytes, err := b.createDirectoryRecordBytes(rootEntry.jolietSector, b.svdRootDirExtentSize, rootEntry.jolietName, &rootEntry, true)
	if err != nil {
		log.Panicf("SVD: Failed root DR: %v", err)
	}
	if len(rootDRJolietBytes) != 34 {
		log.Panicf("SVD: Root DR len %d != 34. ID: '%x'", len(rootDRJolietBytes), getDRIdentifierBytes(rootEntry.jolietName, true, true))
	}
	copy(svdFields.RootDirectoryRecord[:], rootDRJolietBytes)

	copy(svdFields.VolumeSetIdentifier[:], padUTF16StringBE("", 64))
	copy(svdFields.PublisherIdentifier[:], padUTF16StringBE(b.options.PublisherIdentifierJoliet, 64))
	copy(svdFields.DataPreparerIdentifier[:], padUTF16StringBE(b.options.DataPreparerIdentifierJoliet, 64))
	copy(svdFields.ApplicationIdentifier[:], padUTF16StringBE(b.options.ApplicationIdentifierJoliet, 64))

	copy(svdFields.CopyrightFileIdentifier[:], padUTF16StringBEToFixedBytes("", 18, 37))
	copy(svdFields.AbstractFileIdentifier[:], padUTF16StringBEToFixedBytes("", 18, 37))
	copy(svdFields.BibliographicFileIdentifier[:], padUTF16StringBEToFixedBytes("", 18, 37))

	now := time.Now().UTC()
	copy(svdFields.VolumeCreationTimestamp[:], formatTimestamp(now))
	copy(svdFields.VolumeModificationTimestamp[:], formatTimestamp(now))
	copy(svdFields.VolumeExpirationTimestamp[:], formatTimestamp(time.Time{}))
	copy(svdFields.VolumeEffectiveTimestamp[:], formatTimestamp(now))
	svdFields.FileStructureVersion = 1

	svdSectorBytes := make([]byte, SectorSize)
	copy(svdSectorBytes[0:7], headerBytes) // 7-byte common header

	fieldBuf := new(bytes.Buffer)
	fieldBuf.WriteByte(0) // byte 7: Volume Flags (0 for basic Joliet)
	fieldBuf.Write(svdFields.SystemIdentifier[:])
	fieldBuf.Write(svdFields.VolumeIdentifier[:])
	fieldBuf.Write(make([]byte, 8)) // bytes 72-79: unused

	binary.Write(fieldBuf, binary.LittleEndian, svdFields.VolumeSpaceSize)
	binary.Write(fieldBuf, binary.BigEndian, svdFields.VolumeSpaceSize)

	fieldBuf.Write(svdFields.EscapeSequences[:]) // 32 bytes

	binary.Write(fieldBuf, binary.LittleEndian, svdFields.VolumeSetSize)
	binary.Write(fieldBuf, binary.BigEndian, svdFields.VolumeSetSize)
	binary.Write(fieldBuf, binary.LittleEndian, svdFields.VolumeSequenceNumber)
	binary.Write(fieldBuf, binary.BigEndian, svdFields.VolumeSequenceNumber)
	binary.Write(fieldBuf, binary.LittleEndian, svdFields.LogicalBlockSize)
	binary.Write(fieldBuf, binary.BigEndian, svdFields.LogicalBlockSize)
	binary.Write(fieldBuf, binary.LittleEndian, svdFields.PathTableSizeBytes)
	binary.Write(fieldBuf, binary.BigEndian, svdFields.PathTableSizeBytes)

	binary.Write(fieldBuf, binary.LittleEndian, svdFields.LPathTableLocation)
	binary.Write(fieldBuf, binary.LittleEndian, svdFields.OptionalLPathTableLocation)
	binary.Write(fieldBuf, binary.BigEndian, svdFields.MPathTableLocation)
	binary.Write(fieldBuf, binary.BigEndian, svdFields.OptionalMPathTableLocation)

	fieldBuf.Write(svdFields.RootDirectoryRecord[:]) // 34 bytes
	fieldBuf.Write(svdFields.VolumeSetIdentifier[:])
	fieldBuf.Write(svdFields.PublisherIdentifier[:])
	fieldBuf.Write(svdFields.DataPreparerIdentifier[:])
	fieldBuf.Write(svdFields.ApplicationIdentifier[:])
	fieldBuf.Write(svdFields.CopyrightFileIdentifier[:])
	fieldBuf.Write(svdFields.AbstractFileIdentifier[:])
	fieldBuf.Write(svdFields.BibliographicFileIdentifier[:])
	fieldBuf.Write(svdFields.VolumeCreationTimestamp[:])
	fieldBuf.Write(svdFields.VolumeModificationTimestamp[:])
	fieldBuf.Write(svdFields.VolumeExpirationTimestamp[:])
	fieldBuf.Write(svdFields.VolumeEffectiveTimestamp[:])
	fieldBuf.WriteByte(svdFields.FileStructureVersion)

	copy(svdSectorBytes[7:], fieldBuf.Bytes()) // copy marshalled fields after common header
	return svdSectorBytes
}

// createVolumeDescriptorTerminator generates the VD Set Terminator sector.
func (b *ISOBuilder) createVolumeDescriptorTerminator() []byte {
	termSectorBytes := make([]byte, SectorSize)
	header := volumeDescriptorHeader{Type: vdTypeTerminator, StandardIdentifier: [5]byte{'C', 'D', '0', '0', '1'}, Version: 1}
	headerBytes := header.marshalBinary()
	copy(termSectorBytes[0:7], headerBytes)
	// remainder of the sector (bytes 7-2047) should be zeros (see ECMA-119 8.6.3)
	return termSectorBytes
}
