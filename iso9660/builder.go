package iso9660

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// ISOBuilder orchestrates the creation of an ISO 9660 / Joliet image.
type ISOBuilder struct {
	sourceDir      string      // root directory on the filesystem to build the ISO from.
	outputFilename string      // output file
	options        *Options    // options for the ISO image.
	fileEntries    []fileEntry // list of all scanned files and directories.

	totalSectors uint32 // number of sectors in the final ISO image.

	// LBA locations for the Path Tables (Primary and Supplementary, L-Type and M-Type, first and second copies).
	lbaPvdPathTableL, lbaPvdPathTableM, lbaSvdPathTableL, lbaSvdPathTableM     uint32 // doesn't look good..
	lbaPvdPathTableL2, lbaPvdPathTableM2, lbaSvdPathTableL2, lbaSvdPathTableM2 uint32

	// pre-gen byte data for the Path Tables.
	pvdPathTableLData, pvdPathTableMData, svdPathTableLData, svdPathTableMData []byte

	// root directory extent sizes (byte length of the root directory's listing for PVD and SVD).
	// : stored in the Root Directory Record within the PVD/SVD.
	pvdRootDirExtentSize, svdRootDirExtentSize uint32
}

// NewBuilder returns a new ISOBuilder instance with the given source directory, output file path, and options.
// : if opts is nil, DefaultOptions() will be used.
func NewBuilder(sourceDir, outputFilename string, opts *Options) *ISOBuilder {
	if opts == nil {
		opts = DefaultOptions()
	}
	return &ISOBuilder{
		sourceDir:      sourceDir,
		outputFilename: outputFilename,
		options:        opts,
	}
}

// MarkFileNamesAsHidden flags entries whose original filename (the last path component on disk)
// matches any of the provided names as hidden.
// : affects the "Hidden" bit in their Directory Records.s
// : This method should be called after ScanSourceDirectory and before Build.
func (b *ISOBuilder) MarkFileNamesAsHidden(fileNamesToHide ...string) error {
	if len(fileNamesToHide) == 0 {
		return nil
	}

	var error []string // in the case of errors

	for _, name := range fileNamesToHide {
		if name == "" {
			error = append(error, "(empty string)")
			log.Printf("MarkFileNamesAsHidden: Warning - cannot hide an empty filename string.")
			continue
		}
		if name == "." || name == ".." {
			error = append(error, name)
			log.Printf("MarkFileNamesAsHidden: Warning - cannot hide navigational entry '%s' by name.", name)
			continue
		}
		// root directory (index 0) has an internal placeholder for originalName,
		// it cannot be hidden by matching its typical on-disk name this way.
		if b.fileEntries[0].originalName == name { // match is highly unlikely but in any case
			error = append(error, name+" (root)")
			log.Printf("MarkFileNamesAsHidden: Warning - cannot hide the root directory by its original name ('%s') this way.", name)
			continue
		}

		found := false
		for i := range b.fileEntries {
			// skip root's internal placeholder name for matching against user-provided names
			if i == 0 && b.fileEntries[i].originalName == "\x00" {
				continue
			}
			if b.fileEntries[i].originalName == name {
				b.fileEntries[i].isHidden = true
				found = true
				// can't break here as the same original filename might exist in multiple subdirectories
			}
		}
		if !found {
			// warning summary in any case
			error = append(error, name+" (not found)")
			log.Printf("MarkFileNamesAsHidden: Warning - no entry with original name '%s' found to mark as hidden.", name)
		}
	}

	if len(error) > 0 { // return general error -> need to update this
		return fmt.Errorf("encountered issues while attempting to mark files as hidden for: %s", strings.Join(error, ", "))
	}
	return nil
}

// Build constructs the ISO image and writes it to the output file.
// : handles scanning, layout calculation, and writing of all ISO components.
func (b *ISOBuilder) Build() (err error) {
	// if ScanSourceDirectory wasn't called explicitly, call it now.
	if len(b.fileEntries) == 0 || b.fileEntries[0].isoPath != "/" {
		if err = b.ScanSourceDirectory(); err != nil {
			return fmt.Errorf("scanning source directory: %w", err)
		}
	}
	if err = b.calculateLayout(); err != nil {
		return fmt.Errorf("calculating ISO layout: %w", err)
	}

	isoFile, err := os.Create(b.outputFilename)
	if err != nil {
		return fmt.Errorf("creating output file '%s': %w", b.outputFilename, err)
	}
	defer func() {
		closeErr := isoFile.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("closing output file: %w", closeErr)
		}
	}()

	if err = b.writeSystemArea(isoFile); err != nil {
		return fmt.Errorf("writing system area: %w", err)
	}
	if err = b.writeVolumeDescriptors(isoFile); err != nil {
		return fmt.Errorf("writing volume descriptors: %w", err)
	}
	if err = b.writeAllPathTables(isoFile); err != nil {
		return fmt.Errorf("writing path tables: %w", err)
	}
	if err = b.writeAllDirectoryContents(isoFile); err != nil {
		return fmt.Errorf("writing directory contents: %w", err)
	}
	if err = b.writeAllFileData(isoFile); err != nil {
		return fmt.Errorf("writing file data: %w", err)
	}
	if err = b.finalizeImageSize(isoFile); err != nil {
		return fmt.Errorf("finalizing image size: %w", err)
	}
	return err
}
