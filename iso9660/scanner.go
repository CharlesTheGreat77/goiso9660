package iso9660

import (
	"fmt"
	"os"
	"path/filepath"
)

// ScanSourceDirectory scans the input directory structure and populates b.fileEntries.
// This can be called explicitly by the user or implicitly by Build.
func (b *ISOBuilder) ScanSourceDirectory() error {
	b.fileEntries = nil // Clear previous scan results if any
	absPath, err := filepath.Abs(b.sourceDir)
	if err != nil {
		return fmt.Errorf("getting absolute path for source '%s': %w", b.sourceDir, err)
	}

	rootEntry := fileEntry{
		originalName:    "\x00",
		diskPath:        absPath,
		isoPath:         "/",
		isDir:           true,
		level:           0,
		parentIndex:     0, // roots parent is itself (index 0)
		pathTableDirNum: 1, // root directory is always #1 in path table
	}
	b.fileEntries = append(b.fileEntries, rootEntry)

	nextPathTableNum := uint16(2) // next available path table number
	return b.scanDirectoryRecursive(absPath, 0 /*parentIndex for root*/, &nextPathTableNum, absPath /*sourceBaseDiskPath*/)
}

// scanDirectoryRecursive performs a depth-first scan of the filesystem.
func (b *ISOBuilder) scanDirectoryRecursive(currentDiskPath string, parentEntryIndex int, nextPathTableNumber *uint16, sourceBaseDiskPath string) error {
	osEntries, err := os.ReadDir(currentDiskPath)
	if err != nil {
		return fmt.Errorf("reading directory '%s': %w", currentDiskPath, err)
	}

	for _, osEntry := range osEntries {
		fullDiskPath := filepath.Join(currentDiskPath, osEntry.Name())
		fileInfo, err := osEntry.Info()
		if err != nil {
			return fmt.Errorf("getting info for '%s': %w", fullDiskPath, err)
		}

		relativePath, err := filepath.Rel(sourceBaseDiskPath, fullDiskPath)
		if err != nil {
			return fmt.Errorf("calculating relative path for '%s' (base '%s'): %w", fullDiskPath, sourceBaseDiskPath, err)
		}
		currentIsoPath := "/" + filepath.ToSlash(relativePath) // normalize to Unix-style paths

		fe := fileEntry{
			originalName: osEntry.Name(),
			diskPath:     fullDiskPath,
			isoPath:      currentIsoPath,
			level:        b.fileEntries[parentEntryIndex].level + 1,
			parentIndex:  parentEntryIndex,
		}

		if osEntry.IsDir() {
			fe.isDir = true
			fe.pathTableDirNum = *nextPathTableNumber
			(*nextPathTableNumber)++
			b.fileEntries = append(b.fileEntries, fe)
			newEntryIndex := len(b.fileEntries) - 1 // newly added dir
			b.fileEntries[parentEntryIndex].children = append(b.fileEntries[parentEntryIndex].children, newEntryIndex)
			if errRec := b.scanDirectoryRecursive(fullDiskPath, newEntryIndex, nextPathTableNumber, sourceBaseDiskPath); errRec != nil {
				return errRec
			}
		} else if fileInfo.Mode().IsRegular() {
			fe.isDir = false
			fe.iso9660Size = uint32(fileInfo.Size()) // data size
			fe.jolietSize = fe.iso9660Size           // ^ same for joliet
			b.fileEntries = append(b.fileEntries, fe)
			newEntryIndex := len(b.fileEntries) - 1
			b.fileEntries[parentEntryIndex].children = append(b.fileEntries[parentEntryIndex].children, newEntryIndex)
		}
	}
	return nil
}
