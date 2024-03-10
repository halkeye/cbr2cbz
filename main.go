package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	archiver "github.com/mholt/archiver/v4"
	"github.com/pkg/errors"
)

const (
	version     = "0.6"
	tmpDir      = "/tmp/cbr2cbz"
	logFileName = "cbr2cbz.log"
)

var (
	convSize    int64
	failedFiles int
)

var startTime = time.Now()

func main() {
	if len(os.Args) < 2 {
		Help()
		return
	}

	switch os.Args[1] {
	case "single":
		if len(os.Args) < 3 {
			Help()
			return
		}
		SingleRun(os.Args[2])
	case "all":
		BatchRun()
	case "help":
		Help()
	default:
		Help()
	}
}

func Help() {
	fmt.Println("cbr2cbz Conversion Tool")
	fmt.Println("Version", version)
	fmt.Println("https://github.com/halkeye/cbr2cbz (original bash version at https://git.zaks.web.za/thisiszeev/cbr2cbz)")
	fmt.Println()
	fmt.Println("Usage: cbr2cbz single \"filename.cbr\"")
	fmt.Println("  Convert a single file.")
	fmt.Println("Usage: cbr2cbz all")
	fmt.Println("  Convert all files recursively from the current location.")
	fmt.Println("Usage: cbr2cbz help")
	fmt.Println("  Display this text.")
	fmt.Println()
	fmt.Println("Warning: If conversion is successful, the original file(s) will be deleted.")
}

func SingleRun(cbrFile string) {
	cbrFile, err := filepath.Abs(cbrFile)
	if err != nil {
		log.Fatal(err)
	}

	if _, err := os.Stat(cbrFile); os.IsNotExist(err) {
		log.Fatalf("File not found: %s", cbrFile)
	}

	size := strings.TrimSuffix(filepath.Base(cbrFile), filepath.Ext(cbrFile))
	cbzFile := filepath.Join(filepath.Dir(cbrFile), size+".cbz")

	err = convert(cbrFile, cbzFile)
	if err != nil {
		log.Printf("Error Reading %s - Skipping...\n", cbrFile)
		failedFiles++
	}
	PrintStats()
}

func BatchRun() {
	err := os.MkdirAll(tmpDir, 0755)
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cbrFiles, err := findCBRFiles(".")
	if err != nil {
		log.Fatal(err)
	}

	if len(cbrFiles) == 0 {
		fmt.Println("No files to convert!")
		return
	}

	totalSize, err := getFileSize(".")
	if err != nil {
		log.Fatal(err)
	}

	cbrSize, err := getFileSize(cbrFiles...)
	if err != nil {
		log.Fatal(err)
	}

	createLogFile(totalSize, cbrSize, cbrFiles)

	for _, cbrFile := range cbrFiles {
		cbrFile, err := filepath.Abs(cbrFile)
		if err != nil {
			log.Fatal(err)
		}

		size := strings.TrimSuffix(filepath.Base(cbrFile), filepath.Ext(cbrFile))
		cbzFile := filepath.Join(filepath.Dir(cbrFile), size+".cbz")

		convert(cbrFile, cbzFile)
		if err != nil {
			log.Printf("Error Reading %s - Skipping...\n", cbrFile)
			failedFiles++
		}
	}

	PrintStats()
}

func convert(cbrFile, cbzFile string) error {
	fmt.Printf("Converting: %s to %s\n", cbrFile, cbzFile)
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

	fmt.Printf("Successfully Converted %s to %s...\n", cbrFile, cbzFile)

	return nil
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

func getFileSize(paths ...string) (int64, error) {
	var size int64
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return 0, err
		}
		size += info.Size()
	}
	return size, nil
}

func createLogFile(totalSize, cbrSize int64, cbrFiles []string) {
	f, err := os.Create(logFileName)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	fmt.Fprintf(f, "CBR2CBZ Batch Log\n")
	fmt.Fprintf(f, "Version %s\n", version)
	fmt.Fprintf(f, "You can check for script updates at https://github.com/halkeye/cbr2cbz (original bash version at https://git.zaks.web.za/thisiszeev/cbr2cbz)\n")
	fmt.Fprintf(f, "Batch Start Date & Time: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Considering %d files (%db)\n", len(cbrFiles)+countNonCbrFiles(totalSize, cbrSize), totalSize>>20)
	fmt.Fprintf(f, "   of which...\n")
	fmt.Fprintf(f, "CBZ files: %d (%s)\n", countNonCbrFiles(totalSize, cbrSize), formatSize(totalSize-cbrSize))
	fmt.Fprintf(f, "CBR files: %d (%s)\n", len(cbrFiles), formatSize(cbrSize))
}

func countNonCbrFiles(totalSize, cbrSize int64) int {
	return int(totalSize-cbrSize) / (1024 * 1024)
}

func formatSize(size int64) string {
	if size == 0 {
		return "0 B"
	}

	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for size >= 1024 && i < len(units)-1 {
		size /= 1024
		i++
	}
	return fmt.Sprintf("%d %s", size, units[i])
}

func PrintStats() {
	runtime := calculateRuntime()
	fmt.Println()

	fmt.Println("Failed files:")
	log.Println("Failed files:")

	for i := 0; i < failedFiles; i++ {
		fmt.Println(fmt.Sprintf("  // Skipped due to errors"))
		log.Println(fmt.Sprintf("  // Skipped due to errors"))
	}

	if failedFiles == 0 {
		fmt.Println("  none")
		log.Println("  none")
	}

	fmt.Println()
	fmt.Println("Runtime:", runtime)
	log.Println("Runtime:", runtime)

	if failedFiles > 0 {
		fmt.Printf("A log file has been written to %s which contains all the failed files.\n", logFileName)
	}
}

func calculateRuntime() string {
	elapsed := time.Since(startTime)
	days := int(elapsed.Hours() / 24)
	hours := int(elapsed.Hours()) % 24
	minutes := int(elapsed.Minutes()) % 60
	seconds := int(elapsed.Seconds()) % 60

	var runtime string
	if days > 0 {
		runtime += fmt.Sprintf("%d day ", days)
	}
	if hours > 0 {
		runtime += fmt.Sprintf("%d hour ", hours)
	}
	if minutes > 0 {
		runtime += fmt.Sprintf("%d minute ", minutes)
	}
	if days == 0 && hours == 0 && minutes == 0 {
		runtime += fmt.Sprintf("%d second", seconds)
	} else {
		runtime += fmt.Sprintf("%d seconds", seconds)
	}
	return runtime
}
