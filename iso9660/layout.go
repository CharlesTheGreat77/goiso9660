package iso9660

import (
	"fmt"
	"log"
)

// calculateLayout determines all sizes, LBA locations, and pre-generates path tables.
func (b *ISOBuilder) calculateLayout() error {
	if err := b.assignSanitizedNamesAndDrSizes(); err != nil {
		return fmt.Errorf("assigning names/DR sizes: %w", err)
	}
	if err := b.calculateAllDirectoryExtentSizes(); err != nil {
		return fmt.Errorf("calculating dir extent sizes: %w", err)
	}

	currentLBA := uint32(SystemAreaNumSectors + 3) // VD area (PVD, SVD, Terminator)
	currentLBA = b.determinePathTableLBAs(currentLBA)
	currentLBA = b.assignContentLBAs(currentLBA)

	b.totalSectors = currentLBA // LBA after the last sector used by content
	b.totalSectors++            // add one trailing [padding] sector for compatibility
	// was getting a major headache because of this!!

	if err := b.pregeneratePathTables(); err != nil {
		return fmt.Errorf("pre-generating path tables: %w", err)
	}
	return nil
}

// assignSanitizedNamesAndDrSizes prepares ISO9660/Joliet names and calculates DR sizes for all entries.
func (b *ISOBuilder) assignSanitizedNamesAndDrSizes() error {
	for i := range b.fileEntries {
		f := &b.fileEntries[i]
		isRootEntry := (f.pathTableDirNum == 1)
		if f.isDir {
			if isRootEntry {
				f.iso9660Name = ""    // ISO9660 Root DR identifier is 0x00 (represented as empty string for DR logic)
				f.jolietName = "\x00" // Joliet Root DR identifier is 0x00
			} else {
				f.iso9660Name = sanitizeISO9660Name(f.originalName, true)
				f.jolietName = truncateJolietName(f.originalName)
			}
		} else {
			f.iso9660Name = sanitizeISO9660Name(f.originalName, false) + ";1" // files get vers. #
			f.jolietName = truncateJolietName(f.originalName)
		}
		// Calculate actual DR size for use in parent directory listings
		f.actualISO9660DrSize = calculateDirectoryRecordSize(getDRIdentifierBytes(f.iso9660Name, false, isRootEntry))
		f.actualJolietDrSize = calculateDirectoryRecordSize(getDRIdentifierBytes(f.jolietName, true, isRootEntry))
	}
	return nil
}

// calculateAllDirectoryExtentSizes computes the on-disk size for each directory's listing.
func (b *ISOBuilder) calculateAllDirectoryExtentSizes() error {
	for i := range b.fileEntries {
		if b.fileEntries[i].isDir {
			b.fileEntries[i].iso9660Size = b.calculateSingleDirectoryExtentSizeBytes(i, false)
			b.fileEntries[i].jolietSize = b.calculateSingleDirectoryExtentSizeBytes(i, true)
		}
	}
	// root directory extent sizes for PVD/SVD direct reference
	b.pvdRootDirExtentSize = b.fileEntries[0].iso9660Size
	b.svdRootDirExtentSize = b.fileEntries[0].jolietSize
	return nil
}

// calculateSingleDirectoryExtentSizeBytes calculates the total byte size of a directory's listing,
// rounded up to the nearest sector.
// : size is used for the DataLength field of the directory's DR.
func (b *ISOBuilder) calculateSingleDirectoryExtentSizeBytes(dirEntryIndex int, isJoliet bool) uint32 {
	dirEntry := b.fileEntries[dirEntryIndex]
	isDirEntryRoot := (dirEntry.pathTableDirNum == 1)

	// every directory listing must contain "." (self) and ".." (parent) entries.
	dotIdentBytes := getDRIdentifierBytes(".", isJoliet, isDirEntryRoot)
	dotDRSize := calculateDirectoryRecordSize(dotIdentBytes)

	dotDotIdentBytes := getDRIdentifierBytes("..", isJoliet, false)
	dotDotDRSize := calculateDirectoryRecordSize(dotDotIdentBytes)

	totalDRBytes := dotDRSize + dotDotDRSize
	for _, childIndex := range dirEntry.children {
		child := b.fileEntries[childIndex]
		childDrSize := child.actualISO9660DrSize
		if isJoliet {
			childDrSize = child.actualJolietDrSize
		}
		totalDRBytes += childDrSize
	}

	if totalDRBytes == 0 {
		// sanity check
		log.Panicf("CriticalError: Dir='%s'(j:%t) totalDRBytes calculated as ZERO. DotDRSize=%d, DotDotDRSize=%d", dirEntry.isoPath, isJoliet, dotDRSize, dotDotDRSize)
	}

	// round up the total DR bytes to the nearest sector size for the extent.
	numSectors := (uint32(totalDRBytes) + SectorSize - 1) / SectorSize
	finalExtentSizeBytes := numSectors * SectorSize
	if finalExtentSizeBytes == 0 {
		log.Panicf("CriticalError: Dir='%s'(j:%t) finalExtentSizeBytes calculated as ZERO (totalDRBytes=%d, numSectors=%d)", dirEntry.isoPath, isJoliet, totalDRBytes, numSectors)
	}
	return finalExtentSizeBytes
}

// assignPathTableSetLBAs is a helper for determinePathTableLBAs.
func assignPathTableSetLBAs(startLBA uint32, numSectorsL, numSectorsM uint32) (lbaL, lbaM, nextLBA uint32) {
	lbaL = startLBA
	nextLBA = startLBA + numSectorsL
	lbaM = nextLBA
	nextLBA += numSectorsM
	return
}

// determinePathTableLBAs calculates and assigns LBAs for all path tables.
func (b *ISOBuilder) determinePathTableLBAs(startLBA uint32) uint32 {
	currentLBA := startLBA
	// Calculate path table sizes (unpadded byte length first)
	pvdPtLBytes := b.calculatePathTableTotalBytes(false) // PVD L-Type
	svdPtLBytes := b.calculatePathTableTotalBytes(true)  // SVD L-Type

	// # of sectors needed for each path table type (L and M are typically same size)
	numSecPvdL := sectorsToContainBytes(pvdPtLBytes)
	numSecPvdM := numSecPvdL // M-type table typically same size as L-type for same standard
	numSecSvdL := sectorsToContainBytes(svdPtLBytes)
	numSecSvdM := numSecSvdL

	// LBAs for PVD primary and optional path tables
	b.lbaPvdPathTableL, b.lbaPvdPathTableM, currentLBA = assignPathTableSetLBAs(currentLBA, numSecPvdL, numSecPvdM)
	// LBAs for SVD primary and optional path tables
	b.lbaSvdPathTableL, b.lbaSvdPathTableM, currentLBA = assignPathTableSetLBAs(currentLBA, numSecSvdL, numSecSvdM)
	// LBAs for second copies (optional tables)
	b.lbaPvdPathTableL2, b.lbaPvdPathTableM2, currentLBA = assignPathTableSetLBAs(currentLBA, numSecPvdL, numSecPvdM)
	b.lbaSvdPathTableL2, b.lbaSvdPathTableM2, currentLBA = assignPathTableSetLBAs(currentLBA, numSecSvdL, numSecSvdM)

	return currentLBA
}

// assignContentLBAs assigns LBAs to all directory extents and file data extents.
func (b *ISOBuilder) assignContentLBAs(startLBA uint32) uint32 {
	currentLBA := startLBA
	// ISO9660 Directory Extents -> then File Data -> then Joliet Directory Extents

	// ISO9660 Directory Extents
	for i := range b.fileEntries {
		if b.fileEntries[i].isDir {
			f := &b.fileEntries[i]
			f.iso9660Sector = currentLBA
			if f.iso9660Size == 0 {
				log.Panicf("InternalError: Dir '%s' iso9660Size is 0 before LBA assignment", f.isoPath)
			}
			if f.iso9660Size%SectorSize != 0 {
				log.Panicf("InternalError: Dir '%s' iso9660Size %d not multiple of SectorSize", f.isoPath, f.iso9660Size)
			}
			numSectors := f.iso9660Size / SectorSize
			currentLBA += numSectors
		}
	}
	// File Data Extents (shared between ISO9660 and Joliet)
	for i := range b.fileEntries {
		if !b.fileEntries[i].isDir {
			f := &b.fileEntries[i]
			f.iso9660Sector = currentLBA // file data LBA
			f.jolietSector = currentLBA  // Joliet DRs point to the same file data LBA
			numSectors := sectorsToContainFileBytes(f.iso9660Size)
			currentLBA += numSectors
		}
	}
	// Joliet Directory Extents
	for i := range b.fileEntries {
		if b.fileEntries[i].isDir {
			f := &b.fileEntries[i]
			f.jolietSector = currentLBA
			if f.jolietSize == 0 {
				log.Panicf("InternalError: Dir '%s' jolietSize is 0 before LBA assignment", f.isoPath)
			}
			if f.jolietSize%SectorSize != 0 {
				log.Panicf("InternalError: Dir '%s' jolietSize %d not multiple of SectorSize", f.isoPath, f.jolietSize)
			}
			numSectors := f.jolietSize / SectorSize
			currentLBA += numSectors
		}
	}
	return currentLBA
}

// pregeneratePathTables creates the byte data for all path tables.
func (b *ISOBuilder) pregeneratePathTables() error {
	b.pvdPathTableLData = b.createPathTable(false, false) // PVD, L-Type
	b.pvdPathTableMData = b.createPathTable(false, true)  // PVD, M-Type
	b.svdPathTableLData = b.createPathTable(true, false)  // SVD, L-Type
	b.svdPathTableMData = b.createPathTable(true, true)   // SVD, M-Type

	// sanity check generated path table sizes against calculated byte lengths
	if len(b.pvdPathTableLData) != b.calculatePathTableTotalBytes(false) {
		return fmt.Errorf("PVD L-Path Table generated length %d != calculated %d", len(b.pvdPathTableLData), b.calculatePathTableTotalBytes(false))
	}
	if len(b.svdPathTableLData) != b.calculatePathTableTotalBytes(true) {
		return fmt.Errorf("SVD L-Path Table generated length %d != calculated %d", len(b.svdPathTableLData), b.calculatePathTableTotalBytes(true))
	}
	// M-Type tables are often the same length as L-Type for the same standard
	if len(b.pvdPathTableMData) != b.calculatePathTableTotalBytes(false) {
		return fmt.Errorf("PVD M-Path Table generated length %d != calculated (L-type for PVD) %d", len(b.pvdPathTableMData), b.calculatePathTableTotalBytes(false))
	}
	if len(b.svdPathTableMData) != b.calculatePathTableTotalBytes(true) {
		return fmt.Errorf("SVD M-Path Table generated length %d != calculated (L-type for SVD) %d", len(b.svdPathTableMData), b.calculatePathTableTotalBytes(true))
	}
	return nil
}
