package cmd

import (
	"context"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/mholt/archiver/v4"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var logFileName = "cbr2cbz.log"

// convertCmd represents the convert command
var convertCmd = &cobra.Command{
	Use:   "convert",
	Short: "Converts one or more files",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_RDWR, 0666)
		if err != nil {
			panic(err)
		}
		mw := io.MultiWriter(os.Stdout, logFile)
		log.SetOutput(mw)

		cbrFiles := []string{}

		for _, paths := range args {
			files, err := findCBRFiles(paths)
			if err != nil {
				log.Fatal(err)
			}
			cbrFiles = append(cbrFiles, files...)
		}

		if len(cbrFiles) == 0 {
			log.Fatal("No files to convert!")
			return
		}

		totalSize, totalCount, err := getFileStats("", cbrFiles...)
		if err != nil {
			log.Fatal(err)
		}

		cbrSize, cbrCount, err := getFileStats(".cbr", cbrFiles...)
		if err != nil {
			log.Fatal(err)
		}

		log.Printf("CBR2CBZ Batch Log\n")
		log.Printf("Version %s\n", version)
		log.Printf("You can check for script updates at https://github.com/halkeye/cbr2cbz (original bash version at https://git.zaks.web.za/thisiszeev/cbr2cbz)\n")
		log.Printf("Batch Start Date & Time: %s\n", time.Now().Format(time.RFC3339))
		log.Printf("\n")
		log.Printf("Considering %d files (%s)\n", totalCount, humanize.Bytes(totalSize))
		log.Printf("   of which...\n")
		log.Printf("Non CBR files: %d (%s)\n", totalCount-cbrCount, humanize.Bytes(totalSize-cbrSize))
		log.Printf("CBR files: %d (%s)\n", len(cbrFiles), humanize.Bytes(cbrSize))

		for _, cbrFile := range cbrFiles {
			cbrFile, err := filepath.Abs(cbrFile)
			if err != nil {
				log.Fatal(err)
			}

			size := strings.TrimSuffix(filepath.Base(cbrFile), filepath.Ext(cbrFile))
			cbzFile := filepath.Join(filepath.Dir(cbrFile), size+".cbz")

			err = convert(cbrFile, cbzFile)

			if err != nil {
				log.Printf("Error Reading %s - Skipping...\n", cbrFile)
				failedFiles++
			}
		}

		PrintStats()
	},
}

func init() {
	rootCmd.AddCommand(convertCmd)

	convertCmd.Flags().StringVar(&logFileName, "log-file", "cbr2cbz.log", "log file")
}

func findCBRFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".cbr") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func countNonCbrFiles(totalSize, cbrSize uint64) int {
	return int(totalSize-cbrSize) / (1024 * 1024)
}

var (
	convSize    int64
	failedFiles int
)

var startTime = time.Now()

func convert(cbrFile, cbzFile string) error {
	log.Printf("Converting: %s to %s\n", cbrFile, cbzFile)
	ctx := context.TODO()

	rarFS, err := archiver.FileSystem(ctx, cbrFile)
	if err != nil {
		return errors.Wrap(err, "unable to read cbr file")
	}

	files := []archiver.File{}
	zipFormat := archiver.Zip{}

	err = fs.WalkDir(rarFS, ".", func(pathName string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if de.IsDir() {
			// nothing to do
			return nil
		}

		info, err := de.Info()
		if err != nil {
			return errors.Wrap(err, "unable to look up file")
		}

		files = append(files, archiver.File{
			FileInfo:      info,
			NameInArchive: pathName,
			Open: func() (io.ReadCloser, error) {
				return rarFS.Open(pathName)
			},
		})
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "walking rar file")
	}

	// create the output file we'll write to
	out, err := os.Create(cbzFile)
	if err != nil {
		return errors.Wrap(err, "unable to create zip")
	}
	defer out.Close()

	// create the archive
	err = zipFormat.Archive(context.Background(), out, files)
	if err != nil {
		return errors.Wrap(err, "unable to archive zip")
	}

	err = os.Remove(cbrFile)
	if err != nil {
		return errors.Wrap(err, "deleting old cbr")
	}

	fileInfo, err := os.Stat(cbzFile)
	if err != nil {
		return errors.Wrap(err, "looking up newly created zip size")
	}
	convSize += fileInfo.Size()

	log.Printf("Successfully Converted %s to %s...\n", cbrFile, cbzFile)

	return nil
}

func getFileStats(suffix string, paths ...string) (uint64, uint32, error) {
	var size uint64
	var count uint32
	for _, path := range paths {
		err := filepath.Walk(path, func(filename string, _ os.FileInfo, err error) error {
			if !strings.HasSuffix(strings.ToLower(filename), suffix) {
				return nil
			}
			info, err := os.Stat(filename)
			if err != nil {
				return err
			}
			size += uint64(info.Size())
			count += 1
			return nil
		})

		if err != nil {
			return size, count, err
		}
	}
	return size, count, nil
}

func PrintStats() {
	runtime := humanize.RelTime(startTime, time.Now(), "", "")
	log.Println("Failed files:")

	for i := 0; i < failedFiles; i++ {
		log.Printf("  // Skipped due to errors")
	}

	if failedFiles == 0 {
		log.Println("  none")
	}

	log.Println("Runtime:", runtime)

	log.Printf("A log file has been written to %s\n", logFileName)
}
