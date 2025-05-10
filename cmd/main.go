package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/charlesthegreat77/goiso9660/iso9660"
)

var (
	inputDirectory string
	outputISO      string
	hiddenFiles    string
	help           bool
)

func main() {
	flag.StringVar(&inputDirectory, "i", "", "specify path to directory/file")
	flag.StringVar(&outputISO, "o", "output.iso", "specify the output file")
	flag.StringVar(&hiddenFiles, "H", "", "specify files to hide in the iso file [separated by comma]")
	flag.BoolVar(&help, "h", false, "show usage")
	flag.Parse()

	if help || inputDirectory == "" {
		flag.Usage()
		return
	}
	var files []string
	if len(hiddenFiles) > 0 {
		for _, f := range strings.Split(hiddenFiles, ",") {
			if trimmed := strings.TrimSpace(f); trimmed != "" {
				files = append(files, trimmed)
			}
		}
	}

	log.Printf("Building ISO from '%s' to '%s'", inputDirectory, outputISO)

	opts := iso9660.DefaultOptions() // optional
	opts.VolumeIdentifierISO = "MyCD_ISO"
	opts.VolumeIdentifierJoliet = "MyCD_Joliet"
	opts.ApplicationIdentifierISO = "MyApplication"
	opts.PublisherIdentifierISO = "MyPublisher"

	builder := iso9660.NewBuilder(inputDirectory, outputISO, opts)

	// ScanSourceDirectory is part of the public API and should be called separately
	if err := builder.ScanSourceDirectory(); err != nil {
		log.Fatalf("Error scanning directory: %v", err)
	}

	// mark files as hiddens
	err := builder.MarkFileNamesAsHidden(files...)
	if err != nil {
		log.Printf("Warning during MarkFileNamesAsHidden: %v", err)
	}

	if err := builder.Build(); err != nil {
		log.Fatalf("Error building ISO: %v", err)
	}

	fmt.Println("ISO created successfully:", outputISO)
}
