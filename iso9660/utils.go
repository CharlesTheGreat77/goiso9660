package iso9660

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf16"
)

// sectorsToContainBytes calculates the number of sectors needed to hold byteSize data.
// Returns 0 if byteSize is 0.
func sectorsToContainBytes(byteSize int) uint32 {
	if byteSize == 0 {
		return 0
	}
	return (uint32(byteSize) + SectorSize - 1) / SectorSize
}

// sectorsToContainFileBytes calculates sectors needed for file data.
// Even an empty file's extent descriptor points to an LBA, conventionally consuming 1 sector on disk
// for its (empty) data extent, though the data length in its DR would be 0.
func sectorsToContainFileBytes(fileDataSizeBytes uint32) uint32 {
	if fileDataSizeBytes == 0 {
		// ECMA-119 9.1.4: Data Length. If 0, the file has no data.
		// The Location of Extent still points to a logical block.
		// Most implementations allocate at least one sector for an empty file's extent.
		return 1
	}
	return (fileDataSizeBytes + SectorSize - 1) / SectorSize
}

// sanitizeISO9660Name converts a name to be ISO9660 Level 1 compliant
// (uppercase, specific chars, 8.3 for files without version, directories just 8 chars).
// Version numbers (e.g., ";1") are typically appended *after* calling this for files.
func sanitizeISO9660Name(originalName string, isDirectory bool) string {
	var base, ext string
	nameToProcess := originalName

	if !isDirectory {
		lastDot := strings.LastIndex(nameToProcess, ".")
		// cases like ".bashrc" where the part before dot is empty
		if lastDot != -1 && lastDot < len(nameToProcess)-1 {
			base = nameToProcess[:lastDot]
			ext = nameToProcess[lastDot+1:]
		} else {
			base = nameToProcess
			ext = ""
		}
	} else {
		base = nameToProcess
		ext = "" // directories don't have extensions in ISO9660 Level 1 sense
	}

	sanitizePartFunc := func(part string, maxLength int, allowDot bool) string {
		part = strings.ToUpper(part)
		var sb strings.Builder
		for _, r := range part {
			if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
				sb.WriteRune(r)
			} else if allowDot && r == '.' { // only allow dot if explicitly permitted (in base part of filename)
				sb.WriteRune('.') // preserve dots in base for now, will be handled by 8.3 split
			} else {
				sb.WriteRune('_') // replace invalid characters with underscore
			}
		}
		sanitized := sb.String()
		if len(sanitized) > maxLength {
			sanitized = sanitized[:maxLength]
		}
		return sanitized
	}

	// max length for directory name or filename base part (before extension) is 8.
	// max length for extension is 3.
	maxBaseLen := 8
	maxExtLen := 3
	if isDirectory {
		// ECMA-119 7.5.1: Directory Identifiers are d1-characters, max 31.
		// Level 1 (6.8.2.1) is more restrictive: 8 chars, no dot, no version.
		// : aim for Level 1 compatibility for widest support.
		maxBaseLen = 8 // or 31 if not strictly Level 1, but 8 is safer
	}

	sanitizedBase := sanitizePartFunc(base, maxBaseLen, !isDirectory)
	if isDirectory {
		// for directories, remove any dots that might have slipped through
		sanitizedBase = strings.ReplaceAll(sanitizedBase, ".", "_")
		if len(sanitizedBase) > 8 { // re-truncate if replacing dots made it longer
			sanitizedBase = sanitizedBase[:8]
		}
		if sanitizedBase == "" {
			return "DIR" // default name for empty/invalid dir name
		}
		return sanitizedBase
	}

	// files -> ensure 8.3 format
	finalBase := sanitizedBase
	finalExt := ""

	if ext != "" {
		finalExt = sanitizePartFunc(ext, maxExtLen, false)
	}

	// base part contained dots, re-split for 8.3 if necessary,
	// or if original had no extension but base was like "file.name"
	if strings.Contains(finalBase, ".") && finalExt == "" {
		parts := strings.SplitN(finalBase, ".", 2)
		if len(parts) == 2 {
			potentialBase := sanitizePartFunc(parts[0], 8, false)
			potentialExt := sanitizePartFunc(parts[1], 3, false)
			if potentialBase != "" {
				finalBase = potentialBase
				if potentialExt != "" {
					finalExt = potentialExt
				}
			}
		}
	}
	// base does not exceed 8 chars after any re-splitting
	if len(finalBase) > 8 {
		finalBase = finalBase[:8]
	}

	finalName := finalBase
	if finalExt != "" {
		finalName += "." + finalExt
	}

	if finalName == "" || finalName == "." {
		finalName = "FILE" //default for invalid/empty file name
	}
	// remove trailing dots if base is empty and ext exists, e.g. ".TXT" -> "TXT"
	// or if finalName is just "."
	if strings.HasPrefix(finalName, ".") && len(finalName) > 1 && finalExt != "" && finalBase == "" {
		finalName = finalExt
	}

	return finalName
}

// truncateJolietName truncates a name component if it exceeds JolietMaxFilenameChars (64 UCS-2 characters).
func truncateJolietName(originalName string) string {
	if originalName == "\x00" || originalName == "." || originalName == ".." {
		return originalName
	}
	runes := []rune(originalName)
	if len(runes) > JolietMaxFilenameChars {
		log.Printf("Warning: Joliet name '%s' truncated to '%s' (%d char limit)", originalName, string(runes[:JolietMaxFilenameChars]), JolietMaxFilenameChars)
		return string(runes[:JolietMaxFilenameChars])
	}
	return originalName
}

// formatTimestamp creates an ISO9660 17-byte timestamp string.
// (ECMA-119 Section 8.4.26.1)
// : if t is zero, returns a "not specified" timestamp (16 zeros + zero offset byte)
func formatTimestamp(t time.Time) []byte {
	tsBytes := make([]byte, 17)
	if t.IsZero() {
		for i := 0; i < 16; i++ {
			tsBytes[i] = '0'
		}
		// tsBytes[16] is already 0 (GMT offset) by make
		return tsBytes
	}
	// YYYYMMDDHHMMSSmm (mm = hundredths of a second, unused, set to 00)
	// last byte is GMT offset: signed char, 15 min intervals from GMT. 0 for UTC/local.
	timestampStr := fmt.Sprintf("%04d%02d%02d%02d%02d%02d00",
		t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
	copy(tsBytes, []byte(timestampStr))
	tsBytes[16] = 0 // GMT offset (0 indicates local time or UTC)
	return tsBytes
}

// encodeUTF16BE encodes a Go string to UCS-2 Big Endian bytes.
func encodeUTF16BE(s string) []byte {
	uint16s := utf16.Encode([]rune(s))
	buf := new(bytes.Buffer)
	for _, rVal := range uint16s {
		_ = binary.Write(buf, binary.BigEndian, rVal)
	}
	return buf.Bytes()
}

// padString pads/truncates a string with spaces for fixed-length ISO string fields
// (d-characters or a-characters -> see ECMA-119).
func padString(s string, length int) []byte {
	b := make([]byte, length)
	for i := range b {
		b[i] = ' ' // pad with space (0x20)
	}
	bytesToCopy := len(s)
	if bytesToCopy > length {
		bytesToCopy = length
	}
	copy(b, s[:bytesToCopy])
	return b
}

// padUTF16StringBE encodes a string to UCS-2BE and pads/truncates to fit a field specified in characters.
// padding with 0x0000 UCS-2 characters (which are 0x00 0x00 bytes).
// numCharsInField: the number of UCS-2 characters the field should hold.
func padUTF16StringBE(s string, numCharsInField int) []byte {
	targetByteLength := numCharsInField * 2       // UCS-2 char is 2 bytes
	resultBytes := make([]byte, targetByteLength) // zero-filled by default -> for 0x0000 padding

	encodedStringBytes := encodeUTF16BE(s)

	bytesToCopy := len(encodedStringBytes)
	if bytesToCopy > targetByteLength {
		bytesToCopy = targetByteLength
	}
	copy(resultBytes, encodedStringBytes[:bytesToCopy])
	return resultBytes
}

// padUTF16StringBEToFixedBytes pads/truncates a UTF-16BE string for a field of fixed total byte length,
// respecting a maximum character count within that byte length (Joliet Copyright File ID)
// s: The string to encode.
// maxCharsInString: maximum number of UCS-2 characters the string part can occupy.
// totalBytesInField: total byte length of the field in the ISO structure.
func padUTF16StringBEToFixedBytes(s string, maxCharsInString int, totalBytesInField int) []byte {
	if maxCharsInString*2 > totalBytesInField {
		log.Panicf("Logic error in padUTF16StringBEToFixedBytes: maxCharsInString (%d) * 2 > totalBytesInField (%d)", maxCharsInString, totalBytesInField)
	}

	resultBytes := make([]byte, totalBytesInField)

	encodedStringBytes := encodeUTF16BE(s)
	maxByteLengthForStringPart := maxCharsInString * 2

	if len(encodedStringBytes) > maxByteLengthForStringPart {
		encodedStringBytes = encodedStringBytes[:maxByteLengthForStringPart]
	}

	copy(resultBytes, encodedStringBytes)
	return resultBytes
}
