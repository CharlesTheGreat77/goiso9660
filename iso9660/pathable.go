package iso9660

import (
	"bytes"
	"encoding/binary"
	"sort"
)

// marshalPathTableRecord converts pathTableRecordFields and an identifier into a PT record byte slice.
func marshalPathTableRecord(fields *pathTableRecordFields, identifier []byte, useBigEndian bool) ([]byte, error) {
	identifierLen := byte(len(identifier))
	// base record size (8) + identifier length
	recordFinalLen := ptRecFixedPartSize + int(identifierLen)
	if len(identifier)%2 != 0 {
		recordFinalLen++
	}

	record := make([]byte, recordFinalLen)
	record[0] = identifierLen
	record[1] = fields.ExtendedAttributeRecordLength // always 0s

	if useBigEndian { // M-Type Path Table (Big Endian)
		binary.BigEndian.PutUint32(record[2:6], fields.LocationOfExtent)
		binary.BigEndian.PutUint16(record[6:8], fields.ParentDirectoryNumber)
	} else { // L-Type Path Table (Little Endian)
		binary.LittleEndian.PutUint32(record[2:6], fields.LocationOfExtent)
		binary.LittleEndian.PutUint16(record[6:8], fields.ParentDirectoryNumber)
	}
	copy(record[8:], identifier) // dirb identifier
	return record, nil
}

// createPathTable generates the bytes for a Path Table (L-Type or M-Type).
// useBigEndian: true for M-Type (Big Endian), false for L-Type (Little Endian).
func (b *ISOBuilder) createPathTable(isJoliet bool, useBigEndian bool) []byte {
	buffer := new(bytes.Buffer)
	var pathTableDirs []fileEntry

	for _, fe := range b.fileEntries {
		if fe.isDir && fe.pathTableDirNum > 0 { // pathTableDirNum > 0 filters out any non-directory entries by mistake
			pathTableDirs = append(pathTableDirs, fe)
		}
	}

	// sort according to L-Type (by dir number) or M-Type (by name hierarchy)
	sort.Slice(pathTableDirs, func(i, j int) bool {
		dirI, dirJ := pathTableDirs[i], pathTableDirs[j]
		if useBigEndian { // M-Type: sort by directory identifier (name), but hierarchically (ECMA-119 9.4.4)
			var nameIBytes, nameJBytes []byte
			if dirI.pathTableDirNum == 1 {
				nameIBytes = []byte{0x00}
			} else {
				if isJoliet {
					nameIBytes = encodeUTF16BE(dirI.jolietName)
				} else {
					nameIBytes = []byte(dirI.iso9660Name)
				}
			}
			if dirJ.pathTableDirNum == 1 {
				nameJBytes = []byte{0x00}
			} else {
				if isJoliet {
					nameJBytes = encodeUTF16BE(dirJ.jolietName)
				} else {
					nameJBytes = []byte(dirJ.iso9660Name)
				}
			}
			// sort by parent directory number, secondary by name to group children of same parent
			if dirI.parentIndex != dirJ.parentIndex && b.fileEntries[dirI.parentIndex].pathTableDirNum != b.fileEntries[dirJ.parentIndex].pathTableDirNum {
				return b.fileEntries[dirI.parentIndex].pathTableDirNum < b.fileEntries[dirJ.parentIndex].pathTableDirNum
			}
			return bytes.Compare(nameIBytes, nameJBytes) < 0

		}
		// L-Type: sort by directory number (ECMA-119 9.4.3)
		return dirI.pathTableDirNum < dirJ.pathTableDirNum
	})

	for _, dir := range pathTableDirs {
		var ptFields pathTableRecordFields
		ptFields.ExtendedAttributeRecordLength = 0

		var identifierBytes []byte
		if dir.pathTableDirNum == 1 {
			identifierBytes = []byte{0x00}
			ptFields.ParentDirectoryNumber = 1
		} else {
			// non-root directory
			if isJoliet {
				identifierBytes = encodeUTF16BE(dir.jolietName)
			} else {
				identifierBytes = []byte(dir.iso9660Name)
			}
			ptFields.ParentDirectoryNumber = b.fileEntries[dir.parentIndex].pathTableDirNum
		}

		// locationn of the directory's extent
		if isJoliet {
			ptFields.LocationOfExtent = dir.jolietSector
		} else {
			ptFields.LocationOfExtent = dir.iso9660Sector
		}

		recordBytes, _ := marshalPathTableRecord(&ptFields, identifierBytes, useBigEndian)
		buffer.Write(recordBytes)
	}
	return buffer.Bytes()
}

// calculatePathTableTotalBytes calculates the total unpadded byte length of a path table.
// : determine how many sectors the path table will occupy.
func (b *ISOBuilder) calculatePathTableTotalBytes(isJoliet bool) int {
	totalBytes := 0
	for _, fe := range b.fileEntries {
		if fe.isDir && fe.pathTableDirNum > 0 {
			var identifierBytes []byte
			if fe.pathTableDirNum == 1 {
				identifierBytes = []byte{0x00}
			} else {
				if isJoliet {
					identifierBytes = encodeUTF16BE(fe.jolietName)
				} else {
					identifierBytes = []byte(fe.iso9660Name)
				}
			}

			recordFinalLen := ptRecFixedPartSize + len(identifierBytes)
			if len(identifierBytes)%2 != 0 {
				recordFinalLen++
			}
			totalBytes += recordFinalLen
		}
	}
	return totalBytes
}
