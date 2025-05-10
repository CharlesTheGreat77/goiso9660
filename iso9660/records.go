package iso9660

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"sort"
	"time"
)

// marshalDirectoryRecord converts directoryRecordFields and an identifier into a full DR byte slice.
func marshalDirectoryRecord(fields *directoryRecordFields, identifier []byte) ([]byte, error) {
	identifierLen := byte(len(identifier))
	// base DR size (33) + identifier length
	recordLen := drFixedPartSize + int(identifierLen)
	if recordLen%2 != 0 { // DR length must be even
		recordLen++
	}

	buf := make([]byte, recordLen)
	buf[0] = byte(recordLen)
	buf[1] = fields.ExtendedAttributeRecordLength

	binary.LittleEndian.PutUint32(buf[2:6], fields.LocationExtent) // location of Extent (LBA) - Little Endian
	binary.BigEndian.PutUint32(buf[6:10], fields.LocationExtent)   // location of Extent (LBA) - Big Endian
	binary.LittleEndian.PutUint32(buf[10:14], fields.DataLength)   // data Length (size of extent/file) - Little Endian
	binary.BigEndian.PutUint32(buf[14:18], fields.DataLength)      // data Length - Big Endian

	copy(buf[18:25], fields.RecordingTime[:]) // 7 bytes for time
	buf[25] = fields.FileFlags
	buf[26] = fields.FileUnitSize      // interleaved files (0 if not)
	buf[27] = fields.InterleaveGapSize // ^

	binary.LittleEndian.PutUint16(buf[28:30], fields.VolumeSequenceNumber)
	binary.BigEndian.PutUint16(buf[30:32], fields.VolumeSequenceNumber)

	buf[32] = identifierLen // len of File Identifier
	copy(buf[33:], identifier)
	// padding byte (if any, due to identifierLen being odd, making overall DR length odd before final padding) is zero-filled by make().
	return buf, nil
}

// populateDirectoryRecordFields fills the fixed fields of a directoryRecordFields struct.
// drIDNameToEncode is the specific name for THIS DR entry (e.g., "FILE.TXT;1", "SUBDIR", ".", "..").
// targetEntry is the fileEntry that this DR *describes* (used for timestamps, flags, etc.).
// : extentLBA and extentOrDataSize are for the targetEntry.
func (b *ISOBuilder) populateDirectoryRecordFields(drFields *directoryRecordFields, extentLBA, extentOrDataSize uint32, drIDNameToEncode string, targetEntry *fileEntry) {
	drFields.ExtendedAttributeRecordLength = 0
	drFields.LocationExtent = extentLBA
	drFields.DataLength = extentOrDataSize

	var fileTime time.Time
	nowUTC := time.Now().UTC() // fallback
	if targetEntry != nil && targetEntry.diskPath != "" {
		// "." and ".." entries, use the ModTime of the directory they represent
		// for root's "." or "..", targetEntry might be the root entry itself.
		// other entries, it's the actual file/dir.
		statInfo, err := os.Stat(targetEntry.diskPath)
		if err == nil {
			fileTime = statInfo.ModTime().UTC()
		} else {
			log.Printf("Warning: Stat '%s' for timestamp: %v. Using current time.", targetEntry.diskPath, err)
			fileTime = nowUTC
		}
	} else {
		// might happen for the root DR in PVD/SVD if targetEntry is for the abstract root
		// or if diskPath is empty for some reason.
		fileTime = nowUTC
	}

	drFields.RecordingTime[0] = byte(fileTime.Year() - 1900)
	drFields.RecordingTime[1] = byte(fileTime.Month())
	drFields.RecordingTime[2] = byte(fileTime.Day())
	drFields.RecordingTime[3] = byte(fileTime.Hour())
	drFields.RecordingTime[4] = byte(fileTime.Minute())
	drFields.RecordingTime[5] = byte(fileTime.Second())
	drFields.RecordingTime[6] = 0 // GMT Offset (0 for UTC or local time unknown as per ECMA-119 9.1.5)

	var baseFileFlags byte
	if targetEntry.isDir {
		baseFileFlags |= 0x02 // bit 1: Directory
	}
	// other flags like Associated (0x04), Record attributes (0x08, 0x10) not used here.
	// implement them yourself...

	finalFileFlags := baseFileFlags
	// hidden flag (bit 0) applies to the target entry's attributes,
	// not to "." or ".." navigational entries, nor to the PVD/SVD root DR itself.
	// this ensures that the "Hidden" flag is only set on the DR for the actual file/directory entry,
	if drIDNameToEncode != "." && drIDNameToEncode != ".." && drIDNameToEncode != "" && drIDNameToEncode != "\x00" {
		if targetEntry.isHidden {
			finalFileFlags |= 0x01 // set to 0x01 for hidden
		}
	}
	drFields.FileFlags = finalFileFlags

	drFields.FileUnitSize = 0         // no interleaved files
	drFields.InterleaveGapSize = 0    // ^
	drFields.VolumeSequenceNumber = 1 // assuming single volume in set
}

// createDirectoryRecordBytes creates the full byte slice for a Directory Record.
// : populates fields and then marshals them with the appropriate identifier.
func (b *ISOBuilder) createDirectoryRecordBytes(extentLBA, extentOrDataSize uint32, drIDNameToEncode string, targetEntry *fileEntry, isJoliet bool) ([]byte, error) {
	var drFields directoryRecordFields
	b.populateDirectoryRecordFields(&drFields, extentLBA, extentOrDataSize, drIDNameToEncode, targetEntry)

	isTargetEntryRoot := (targetEntry.pathTableDirNum == 1)

	var isNameForRootItself bool
	if isTargetEntryRoot {
		if isJoliet && (drIDNameToEncode == "\x00" || drIDNameToEncode == ".") { // Joliet root DR (\x00) or root's "."
			isNameForRootItself = true
		} else if !isJoliet && (drIDNameToEncode == "" || drIDNameToEncode == ".") { // ISO root DR ("") or root's "."
			isNameForRootItself = true
		}
	}
	// non-root entries, or for names like "..", isNameForRootItself remains false.

	identifierBytes := getDRIdentifierBytes(drIDNameToEncode, isJoliet, isNameForRootItself)
	return marshalDirectoryRecord(&drFields, identifierBytes)
}

// getDRIdentifierBytes returns the byte representation for a Directory Record identifier,
// handling special cases for root, ".", and "..".
// isIdentifierForRootItself: true if this identifier is for the root directory itself (e.g., PVD/SVD root DR, or root's "." entry).
func getDRIdentifierBytes(name string, isJoliet bool, isIdentifierForRootItself bool) []byte {
	if isJoliet {
		if isIdentifierForRootItself && (name == "\x00" || name == ".") {
			return []byte{0x00}
		}
		if name == "." { // "." for a non-root directory
			return encodeUTF16BE(".")
		}
		if name == ".." {
			return encodeUTF16BE("..")
		}
		// Joliet name
		return encodeUTF16BE(name)
	}

	// ISO9660
	if name == "." || (isIdentifierForRootItself && name == "") { // "" is placeholder for root in PVD
		return []byte{0x00}
	}
	if name == ".." {
		return []byte{0x01}
	}
	// ISO9660 name
	return []byte(name)
}

// calculateDirectoryRecordSize calculates the total byte length of a Directory Record, including padding.
func calculateDirectoryRecordSize(identifierBytes []byte) int {
	length := drFixedPartSize + len(identifierBytes) // base + len(identifier)
	if length%2 != 0 {                               // DRs must be an even number of bytes
		length++
	}
	return length
}

// createDirectoryListing generates the byte stream for a directory's content (., .., and children DRs).
func (b *ISOBuilder) createDirectoryListing(dirEntryIndex int, isJoliet bool) ([]byte, error) {
	buffer := new(bytes.Buffer)
	currentDir := b.fileEntries[dirEntryIndex]

	var selfLBA, selfExtentSizeBytes uint32
	if isJoliet {
		selfLBA, selfExtentSizeBytes = currentDir.jolietSector, currentDir.jolietSize
	} else {
		selfLBA, selfExtentSizeBytes = currentDir.iso9660Sector, currentDir.iso9660Size
	}

	// "." entry (points to the current directory itself)
	dotDRBytes, err := b.createDirectoryRecordBytes(selfLBA, selfExtentSizeBytes, ".", &currentDir, isJoliet)
	if err != nil {
		return nil, fmt.Errorf("creating '.' DR for '%s' (joliet: %t): %w", currentDir.isoPath, isJoliet, err)
	}
	expectedDotDRLen := calculateDirectoryRecordSize(getDRIdentifierBytes(".", isJoliet, currentDir.pathTableDirNum == 1))
	if len(dotDRBytes) != expectedDotDRLen {
		log.Panicf("CriticalDRLenMismatch: '.' in '%s'(j:%t): Marshalled %d != Expected %d", currentDir.isoPath, isJoliet, len(dotDRBytes), expectedDotDRLen)
	}
	buffer.Write(dotDRBytes)

	// ".." entry (points to the parent directory)
	parentDir := b.fileEntries[currentDir.parentIndex] // for root, parentIndex is 0 (self)
	var parentLBA, parentExtentSizeBytes uint32
	if isJoliet {
		parentLBA, parentExtentSizeBytes = parentDir.jolietSector, parentDir.jolietSize
	} else {
		parentLBA, parentExtentSizeBytes = parentDir.iso9660Sector, parentDir.iso9660Size
	}
	// targetEntry for ".." is the parent directory.
	dotDotDRBytes, err := b.createDirectoryRecordBytes(parentLBA, parentExtentSizeBytes, "..", &parentDir, isJoliet)
	if err != nil {
		return nil, fmt.Errorf("creating '..' DR for '%s' (joliet: %t): %w", currentDir.isoPath, isJoliet, err)
	}
	expectedDotDotDRLen := calculateDirectoryRecordSize(getDRIdentifierBytes("..", isJoliet, false)) // ".." is never root itself in this context
	if len(dotDotDRBytes) != expectedDotDotDRLen {
		log.Panicf("CriticalDRLenMismatch: '..' in '%s'(j:%t): Marshalled %d != Expected %d", currentDir.isoPath, isJoliet, len(dotDotDRBytes), expectedDotDotDRLen)
	}
	buffer.Write(dotDotDRBytes)

	// entries for children, sorted alphabetically by their respective standard's name
	if len(currentDir.children) > 0 {
		childrenEntries := make([]fileEntry, len(currentDir.children))
		for i, idx := range currentDir.children {
			childrenEntries[i] = b.fileEntries[idx]
		}
		sort.Slice(childrenEntries, func(i, j int) bool {
			if isJoliet {
				return childrenEntries[i].jolietName < childrenEntries[j].jolietName
			}
			return childrenEntries[i].iso9660Name < childrenEntries[j].iso9660Name
		})

		for _, childEntry := range childrenEntries {
			var childLBA, childSizeOrDataLen uint32
			var childRecordName string
			var expectedChildDRLen int

			if childEntry.isDir {
				if isJoliet {
					childLBA, childSizeOrDataLen, childRecordName = childEntry.jolietSector, childEntry.jolietSize, childEntry.jolietName
					expectedChildDRLen = childEntry.actualJolietDrSize
				} else {
					childLBA, childSizeOrDataLen, childRecordName = childEntry.iso9660Sector, childEntry.iso9660Size, childEntry.iso9660Name
					expectedChildDRLen = childEntry.actualISO9660DrSize
				}
			} else {
				// files -> LBA and data length are the same for ISO9660 and Joliet
				childLBA, childSizeOrDataLen = childEntry.iso9660Sector, childEntry.iso9660Size
				if isJoliet {
					childRecordName = childEntry.jolietName
					expectedChildDRLen = childEntry.actualJolietDrSize
				} else {
					childRecordName = childEntry.iso9660Name
					expectedChildDRLen = childEntry.actualISO9660DrSize
				}
			}

			childDRBytes, err := b.createDirectoryRecordBytes(childLBA, childSizeOrDataLen, childRecordName, &childEntry, isJoliet)
			if err != nil {
				return nil, fmt.Errorf("creating child DR for '%s' in '%s' (joliet: %t): %w", childEntry.isoPath, currentDir.isoPath, isJoliet, err)
			}
			if len(childDRBytes) != expectedChildDRLen {
				log.Panicf("CriticalDRLenMismatch: Child '%s'(orig:'%s',isDir:%t,j:%t) in '%s': Marshalled %d != Expected %d. IDForDR:'%s'(%x)", childEntry.isoPath, childEntry.originalName, childEntry.isDir, isJoliet, currentDir.isoPath, len(childDRBytes), expectedChildDRLen, childRecordName, getDRIdentifierBytes(childRecordName, isJoliet, false))
			}
			buffer.Write(childDRBytes)
		}
	}
	return buffer.Bytes(), nil
}
