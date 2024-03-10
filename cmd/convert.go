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
	"github.com/hack-pad/hackpadfs"
	hackpados "github.com/hack-pad/hackpadfs/os"
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

		fsys := hackpados.NewFS()

		cbrFiles := []string{}

		for _, paths := range args {
			files, err := findCBRFiles(fsys, paths)
			if err != nil {
				log.Fatal(err)
			}
			cbrFiles = append(cbrFiles, files...)
		}

		err = runConvert(cmd.Context(), fsys, cbrFiles)
		if err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(convertCmd)

	convertCmd.Flags().StringVar(&logFileName, "log-file", "cbr2cbz.log", "log file")
}

func runConvert(context context.Context, fsys hackpadfs.FS, cbrFiles []string) error {
	failedFiles := map[string]error{}
	startTime := time.Now()

	if len(cbrFiles) == 0 {
		return errors.New("No files to convert!")
	}

	totalSize, totalCount, err := getFileStats(fsys, "", cbrFiles...)
	if err != nil {
		return errors.Wrap(err, "getting non cbr file stats")
	}

	cbrSize, cbrCount, err := getFileStats(fsys, ".cbr", cbrFiles...)
	if err != nil {
		return errors.Wrap(err, "getting cbr file stats")
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
			return errors.Wrap(err, "getting abs")
		}

		size := strings.TrimSuffix(filepath.Base(cbrFile), filepath.Ext(cbrFile))
		cbzFile := filepath.Join(filepath.Dir(cbrFile), size+".cbz")

		err = convert(context, fsys, cbrFile, cbzFile)

		if err != nil {
			log.Printf("Error Reading %s - Skipping...\n", cbrFile)
			failedFiles[cbrFile] = err
			continue
		}
	}

	printStats(startTime, failedFiles)

	return nil
}

func findCBRFiles(fsys hackpadfs.FS, root string) ([]string, error) {
	var files []string
	err := hackpadfs.WalkDir(fsys, root, func(path string, info fs.DirEntry, err error) error {
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

func convert(ctx context.Context, fsys hackpadfs.FS, cbrFile string, cbzFile string) error {
	log.Printf("Converting: %s to %s\n", cbrFile, cbzFile)

	info, err := fs.Stat(fsys, cbrFile)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return errors.New("is a directory")
	}

	file, err := fsys.Open(cbrFile)
	if err != nil {
		return errors.Wrap(err, "trying to open cbr")
	}
	defer file.Close()

	format, _, err := archiver.Identify(filepath.Base(cbrFile), file)
	if err != nil && !errors.Is(err, archiver.ErrNoMatch) {
		return errors.Wrap(err, "unable to identify")
	}

	if _, ok := format.(archiver.Rar); !ok {
		return errors.New("not a rar file")
	}

	rarFS := archiver.ArchiveFS{Path: cbrFile, Format: archiver.Rar{}, Context: ctx}
	// rarFS, err := archiver.FileSystem(ctx, cbrFile)
	// if err != nil {
	// 	return errors.Wrap(err, "unable to read cbr file")
	// }

	files := []archiver.File{}

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
	outFile, err := hackpadfs.Create(fsys, cbzFile)
	if err != nil {
		return errors.Wrap(err, "unable to create zip")
	}
	defer outFile.Close()

	destFileWriter, ok := outFile.(io.Writer)
	if !ok {
		return errors.New("destination isn't a writable filesystem")
	}

	// create the archive
	err = archiver.Zip{}.Archive(context.Background(), destFileWriter, files)
	if err != nil {
		return errors.Wrap(err, "unable to archive zip")
	}

	err = os.Remove(cbrFile)
	if err != nil {
		return errors.Wrap(err, "deleting old cbr")
	}

	log.Printf("Successfully Converted %s to %s...\n", cbrFile, cbzFile)

	return nil
}

func getFileStats(fsys hackpadfs.FS, suffix string, paths ...string) (uint64, uint32, error) {
	var size uint64
	var count uint32

	handler := func(filename string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(strings.ToLower(filename), suffix) {
			return nil
		}

		info, err := fs.Stat(fsys, filename)
		if err != nil {
			return errors.Wrap(err, "get file stats for specific file")
		}

		size += uint64(info.Size())
		count += 1
		return nil
	}

	for _, path := range paths {
		err := fs.WalkDir(fsys, path, handler)

		if err != nil {
			return size, count, errors.Wrap(err, "walking directory error")
		}
	}
	return size, count, nil
}

func printStats(startTime time.Time, failedFiles map[string]error) {
	runtime := humanize.RelTime(startTime, time.Now(), "", "")
	log.Println("Failed files:")

	for filename, err := range failedFiles {
		log.Printf("\t%s\t%s", filename, err.Error())
	}

	if len(failedFiles) == 0 {
		log.Println("  none")
	}

	log.Println("Runtime:", runtime)

	log.Printf("A log file has been written to %s\n", logFileName)
}
