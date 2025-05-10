package iso9660

import (
	"fmt"
	"io"
	"log"
	"os"
)

// writeSystemArea writes the initial blank system area sectors.
func (b *ISOBuilder) writeSystemArea(w io.WriteSeeker) error {
	// System area is typically 16 sectors of zeros.
	// writeAtSectorAndPad handles writing nil data as zeros for the allocated size.
	if err := writeAtSectorAndPad(w, nil, 0, SystemAreaNumSectors*SectorSize); err != nil {
		return fmt.Errorf("writing system area: %w", err)
	}
	return nil
}

// writeVolumeDescriptors writes the PVD, SVD, and Terminator to the ISO image.
func (b *ISOBuilder) writeVolumeDescriptors(w io.WriteSeeker) error {
	currentSector := uint32(SystemAreaNumSectors) // VDs start after the system area

	pvd := b.createPrimaryVolumeDescriptor()
	if err := writeAtSectorAndPad(w, pvd, int(currentSector), SectorSize); err != nil {
		return fmt.Errorf("PVD write: %w", err)
	}
	currentSector++

	svd := b.createJolietVolumeDescriptor()
	if err := writeAtSectorAndPad(w, svd, int(currentSector), SectorSize); err != nil {
		return fmt.Errorf("SVD write: %w", err)
	}
	currentSector++

	term := b.createVolumeDescriptorTerminator()
	if err := writeAtSectorAndPad(w, term, int(currentSector), SectorSize); err != nil {
		return fmt.Errorf("terminator write: %w", err)
	}
	return nil
}

// writeAllPathTables writes all four path tables (PVD L/M, SVD L/M) and their duplicates.
func (b *ISOBuilder) writeAllPathTables(w io.WriteSeeker) error {
	// PVD (ISO9660) Path Tables
	pvdPtLAllocSize := int(sectorsToContainBytes(len(b.pvdPathTableLData)) * SectorSize) // Size on disk
	if err := writeAtSectorAndPad(w, b.pvdPathTableLData, int(b.lbaPvdPathTableL), pvdPtLAllocSize); err != nil {
		return fmt.Errorf("PVD L-PT (1st): %w", err)
	}
	if err := writeAtSectorAndPad(w, b.pvdPathTableLData, int(b.lbaPvdPathTableL2), pvdPtLAllocSize); err != nil {
		return fmt.Errorf("PVD L-PT (2nd): %w", err)
	}

	pvdPtMAllocSize := int(sectorsToContainBytes(len(b.pvdPathTableMData)) * SectorSize) // Should be same as L-type size
	if err := writeAtSectorAndPad(w, b.pvdPathTableMData, int(b.lbaPvdPathTableM), pvdPtMAllocSize); err != nil {
		return fmt.Errorf("PVD M-PT (1st): %w", err)
	}
	if err := writeAtSectorAndPad(w, b.pvdPathTableMData, int(b.lbaPvdPathTableM2), pvdPtMAllocSize); err != nil {
		return fmt.Errorf("PVD M-PT (2nd): %w", err)
	}

	// SVD (Joliet) Path Tables
	svdPtLAllocSize := int(sectorsToContainBytes(len(b.svdPathTableLData)) * SectorSize)
	if err := writeAtSectorAndPad(w, b.svdPathTableLData, int(b.lbaSvdPathTableL), svdPtLAllocSize); err != nil {
		return fmt.Errorf("SVD L-PT (1st): %w", err)
	}
	if err := writeAtSectorAndPad(w, b.svdPathTableLData, int(b.lbaSvdPathTableL2), svdPtLAllocSize); err != nil {
		return fmt.Errorf("SVD L-PT (2nd): %w", err)
	}

	svdPtMAllocSize := int(sectorsToContainBytes(len(b.svdPathTableMData)) * SectorSize)
	if err := writeAtSectorAndPad(w, b.svdPathTableMData, int(b.lbaSvdPathTableM), svdPtMAllocSize); err != nil {
		return fmt.Errorf("SVD M-PT (1st): %w", err)
	}
	if err := writeAtSectorAndPad(w, b.svdPathTableMData, int(b.lbaSvdPathTableM2), svdPtMAllocSize); err != nil {
		return fmt.Errorf("SVD M-PT (2nd): %w", err)
	}
	return nil
}

// writeAllDirectoryContents writes the ISO9660 and Joliet directory listings for all directories.
func (b *ISOBuilder) writeAllDirectoryContents(w io.WriteSeeker) error {
	for i, f := range b.fileEntries {
		if f.isDir {
			// ISO9660 Directory Listing
			isoListingBytes, err := b.createDirectoryListing(i, false)
			if err != nil {
				return fmt.Errorf("generating ISO9660 listing for '%s': %w", f.isoPath, err)
			}
			// f.iso9660Size is the pre-calc., sector-aligned allocated size for this directory's listing
			if uint32(len(isoListingBytes)) > f.iso9660Size {
				return fmt.Errorf("ISO9660 list for '%s'(%s) gen_len %d > alloc_size %d", f.isoPath, f.iso9660Name, len(isoListingBytes), f.iso9660Size)
			}
			if err := writeAtSectorAndPad(w, isoListingBytes, int(f.iso9660Sector), int(f.iso9660Size)); err != nil {
				return fmt.Errorf("writing ISO9660 dir extent for '%s': %w", f.isoPath, err)
			}

			// Joliet Directory Listing
			jolietListingBytes, err := b.createDirectoryListing(i, true)
			if err != nil {
				return fmt.Errorf("generating Joliet listing for '%s': %w", f.isoPath, err)
			}
			if uint32(len(jolietListingBytes)) > f.jolietSize {
				return fmt.Errorf("joliet list for '%s'(%s) gen_len %d > alloc_size %d", f.isoPath, f.jolietName, len(jolietListingBytes), f.jolietSize)
			}
			if err := writeAtSectorAndPad(w, jolietListingBytes, int(f.jolietSector), int(f.jolietSize)); err != nil {
				return fmt.Errorf("writing Joliet dir extent for '%s': %w", f.isoPath, err)
			}
		}
	}
	return nil
}

// writeAllFileData writes the actual content of all files to the ISO image.
func (b *ISOBuilder) writeAllFileData(w io.WriteSeeker) error {
	for _, f := range b.fileEntries {
		if !f.isDir {
			fileDataBytes, err := os.ReadFile(f.diskPath)
			if err != nil {
				return fmt.Errorf("reading file '%s': %w", f.diskPath, err)
			}
			if uint32(len(fileDataBytes)) != f.iso9660Size { // iso9660Size and jolietSize are same for files
				return fmt.Errorf("size mismatch for file '%s': scanned %d, actual %d", f.diskPath, f.iso9660Size, len(fileDataBytes))
			}

			// totalAllocatedBytesOnDisk is their data size rounded up to the nearest sector. : for files
			numSectorsForFile := sectorsToContainFileBytes(f.iso9660Size)
			allocatedBytesForFile := int(numSectorsForFile * SectorSize)

			if err := writeAtSectorAndPad(w, fileDataBytes, int(f.iso9660Sector), allocatedBytesForFile); err != nil {
				return fmt.Errorf("writing file data for '%s': %w", f.diskPath, err)
			}
		}
	}
	return nil
}

// finalizeImageSize ensures the ISO file is padded or truncated to the exact total calculated size.
func (b *ISOBuilder) finalizeImageSize(isoFile *os.File) error {
	expectedImageSizeBytes := int64(b.totalSectors) * SectorSize
	currentImageSizeBytes, err := isoFile.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("seeking to end of ISO file: %w", err)
	}

	if currentImageSizeBytes < expectedImageSizeBytes {
		paddingBytesNeeded := expectedImageSizeBytes - currentImageSizeBytes
		if paddingBytesNeeded > 0 {
			chunkSizeBytes := int64(SectorSize * 128) // e.g., 256KB chunks
			if chunkSizeBytes > paddingBytesNeeded {
				chunkSizeBytes = paddingBytesNeeded
			}
			paddingChunk := make([]byte, chunkSizeBytes)

			for paddingBytesNeeded > 0 {
				bytesToWriteThisChunk := chunkSizeBytes
				if paddingBytesNeeded < chunkSizeBytes {
					bytesToWriteThisChunk = paddingBytesNeeded
				}
				n, errWrite := isoFile.Write(paddingChunk[:bytesToWriteThisChunk])
				if errWrite != nil {
					return fmt.Errorf("writing final image padding: %w", errWrite)
				}
				paddingBytesNeeded -= int64(n)
			}
		}
	} else if currentImageSizeBytes > expectedImageSizeBytes {
		log.Printf("Warning: ISO image size %d bytes > expected %d bytes. Truncating.", currentImageSizeBytes, expectedImageSizeBytes)
		if errTrunc := isoFile.Truncate(expectedImageSizeBytes); errTrunc != nil {
			return fmt.Errorf("truncating final image: %w", errTrunc)
		}
	}
	return nil
}

// writeAtSectorAndPad writes data to a specific sector in the WriteSeeker,
// padding with zeros up to totalAllocatedBytesOnDisk.
// sectorNum is 0-indexed.
// totalAllocatedBytesOnDisk must be a multiple of SectorSize if > 0.
func writeAtSectorAndPad(w io.WriteSeeker, data []byte, sectorNum int, totalAllocatedBytesOnDisk int) error {
	if totalAllocatedBytesOnDisk > 0 && totalAllocatedBytesOnDisk%SectorSize != 0 {
		// This indicates a logic error elsewhere in size calculation.
		log.Panicf("writeAtSectorAndPad: totalAllocatedBytesOnDisk %d is not a multiple of SectorSize %d for sector %d", totalAllocatedBytesOnDisk, SectorSize, sectorNum)
	}
	if len(data) > totalAllocatedBytesOnDisk {
		return fmt.Errorf("data length %d > allocated %d for sector %d", len(data), totalAllocatedBytesOnDisk, sectorNum)
	}
	if totalAllocatedBytesOnDisk < 0 { // Should not happen
		return fmt.Errorf("negative totalAllocatedBytesOnDisk %d for sector %d", totalAllocatedBytesOnDisk, sectorNum)
	}

	targetOffset := int64(sectorNum) * int64(SectorSize)
	if _, err := w.Seek(targetOffset, io.SeekStart); err != nil {
		return fmt.Errorf("seeking to offset %d (sector %d): %w", targetOffset, sectorNum, err)
	}

	bytesWritten := 0
	if len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return fmt.Errorf("writing %d bytes data at sector %d: %w", len(data), sectorNum, err)
		}
		if n != len(data) {
			return fmt.Errorf("short write data sector %d: wrote %d/%d", sectorNum, n, len(data))
		}
		bytesWritten = n
	}

	paddingNeeded := totalAllocatedBytesOnDisk - bytesWritten
	if paddingNeeded < 0 {
		return fmt.Errorf("internal error: negative padding %d (totalAlloc %d, written %d) for sector %d", paddingNeeded, totalAllocatedBytesOnDisk, bytesWritten, sectorNum)
	}

	if paddingNeeded > 0 {
		padBuf := make([]byte, SectorSize)
		for paddingNeeded > 0 {
			chunkToWrite := len(padBuf)
			if paddingNeeded < chunkToWrite {
				chunkToWrite = paddingNeeded
			}
			n, err := w.Write(padBuf[:chunkToWrite])
			if err != nil {
				return fmt.Errorf("padding %d bytes at sector %d: %w", paddingNeeded, sectorNum, err)
			}
			if n != chunkToWrite {
				return fmt.Errorf("short padding write sector %d: wrote %d/%d", sectorNum, n, chunkToWrite)
			}
			paddingNeeded -= n
		}
	}
	return nil
}
