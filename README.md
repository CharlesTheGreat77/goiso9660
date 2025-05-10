# goiso9660

[![Go Version](https://img.shields.io/github/go-mod/go-version/charlesthegreat77/goiso9660?style=flat-square)](https://golang.org)
[![Go Report Card](https://goreportcard.com/badge/github.com/charlesthegreat77/goiso9660?style=flat-square)](https://goreportcard.com/report/github.com/charlesthegreat77/goiso9660)

**goiso9660** empowers Go developers to programmatically **create** ISO9660 (Level 1) and Joliet level 3-compliant disc images. Built with a focus on modularity and adherence to the ECMA-119 standard, it provides both a library for integration and a basic command-line tool for quick image *generation*.

Whether you're **archiving** data, **preparing** software distributions, **exploring** file system structures, or **hidding** payloads in iso files, goiso9660 offers a modular Go-native solution.

## 🌟 Core Features

*   ✅ **ISO 9660 Level 1:** Generates widely compatible images, adhering to strict naming and structure rules.
*   🇵🇱 **Joliet Extension:** Full support for Joliet level 3, enabling:
    *   Unicode filenames (UCS-2).
    *   Filenames up to 64 UCS-2 characters.
    *   Deeper directory hierarchies.
*   ⚙️ **Rich Metadata Customization:** Fine-tune your ISOs with:
    *   Volume Identifiers (for both ISO 9660 and Joliet).
    *   System, Publisher, Data Preparer, and Application Identifiers.
*   🙈 **File Hiding:** Selectively hide files within the ISO image.

## 🚀 Getting Started

### Prerequisites

*   Go `1.22` or newer.

### 📦 Installation

#### As a Go Library (Recommended for Developers)

Integrate GoISO9660 into your Go projects with a single command:
```bash
go get github.com/charlesthegreat77/goiso9660/iso9660
```

### Build from source
```bash
go build -o goiso9660 main.go
./goiso9660 -h
```
Build an iso image
```bash
./goiso9660 -i directory/ -o output.iso

# hide file
./goiso9660 -i directory/ -H payload.exe,link.pdf -o image.iso
```

### Development
Create a basic iso image:

`create.go`
```golang
package main

import (
	"fmt"
	"log"

	"github.com/charlesthegreat77/goiso9660/iso9660"
)

func main() {
	inputDirectory := "your_source_directory_here"

	// (Optional) list any files inputDirectory that you want to hide.
	// eg. filesToHide := []string{"secret.txt"}
	filesToHide := []string{}

	outputISO := "example.iso"

	log.Printf("Creating ISO '%s' from directory '%s'", outputISO, inputDirectory)

	imgOptions := iso9660.DefaultOptions()

	// create ISO builder
	builder := iso9660.NewBuilder(inputDirectory, outputISO, imgOptions)

	if err := builder.ScanSourceDirectory(); err != nil {
		log.Fatalf("Error scanning directory '%s': %v", inputDirectory, err)
	}

	// (Optional) mark file(s) as hidden
	if len(filesToHide) > 0 {
		if err := builder.MarkFileNamesAsHidden(filesToHide...); err != nil {
			log.Printf("Warning: issue marking files as hidden: %v", err)
		}
	}

	// build the ISO image
	if err := builder.Build(); err != nil {
		log.Fatalf("Error building ISO image '%s': %v", outputISO, err)
	}

	fmt.Printf("ISO image '%s' created successfully!\n", outputISO)
}
```

Modify ISO options
```golang
package main

import (
	"fmt"
	"log"

	"github.com/charlesthegreat77/goiso9660/iso9660"
)

func main() {
	inputDirectory := "your_source_directory_here"

	// (optional) list any files inputDirectory that you want to hide.
	// eg. filesToHide := []string{"secret.txt"}
	filesToHide := []string{}

	outputISO := "example.iso"

	log.Printf("Creating ISO '%s' from directory '%s'", outputISO, inputDirectory)

	img := iso9660.DefaultOptions()
    img.VolumeIdentifierISO = "EXAMPLE_ISO"
	img.VolumeIdentifierJoliet = "EXAMPLE_Joliet"
	img.ApplicationIdentifierISO = "MyApplication"
	img.PublisherIdentifierISO = "MyPublisher"

	// create ISO builder
	builder := iso9660.NewBuilder(inputDirectory, outputISO, imgOptions)

	if err := builder.ScanSourceDirectory(); err != nil {
		log.Fatalf("Error scanning directory '%s': %v", inputDirectory, err)
	}

	// (optional) mark file(s) as hidden
	if len(filesToHide) > 0 {
		if err := builder.MarkFileNamesAsHidden(filesToHide...); err != nil {
			log.Printf("Warning: issue marking files as hidden: %v", err)
		}
	}

	// build the ISO image
	if err := builder.Build(); err != nil {
		log.Fatalf("Error building ISO image '%s': %v", outputISO, err)
	}

	fmt.Printf("ISO image '%s' created successfully!\n", outputISO)
}
```


### Roadmap
1. Fix directory file size giving *unusual* isovfy output. (Still opens fine so could be something goofy)


### Note
More extensive testing must be done for general usage unique directory trees but for my case of natively hidding payloads, it does its duties! 