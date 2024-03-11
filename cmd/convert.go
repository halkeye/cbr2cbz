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
		logger := log.Default()

		logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_RDWR, 0666)
		if err != nil {
			panic(err)
		}
		mw := io.MultiWriter(os.Stdout, logFile)
		logger.SetOutput(mw)

		fsys := hackpados.NewFS()

		c := converter{
			fs:     fsys,
			logger: logger,
		}

		err = c.runConvert(cmd.Context(), args)
		if err != nil {
			logger.Fatal(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(convertCmd)

	convertCmd.Flags().StringVar(&logFileName, "log-file", "cbr2cbz.log", "log file")
}

type converter struct {
	fs     hackpadfs.FS
	logger logger
}

type logger interface {
	Printf(format string, v ...any)
	Println(v ...any)
}

func (c converter) runConvert(context context.Context, paths []string) error {
	failedFiles := map[string]error{}
	startTime := time.Now()

	cbrFiles := []string{}
	for _, path := range paths {
		stat, err := fs.Stat(c.fs, pathToFsPath(path))
		if err != nil {
			return errors.Wrap(err, "error looking up path")
		}

		if stat.IsDir() {
			files, err := findCBRFiles(c.fs, filepath.Join(path, "."))
			if err != nil {
				return errors.Wrap(err, "finding cbrs")
			}
			cbrFiles = append(cbrFiles, files...)
		} else {
			cbrFiles = append(cbrFiles, path)
		}
	}

	if len(cbrFiles) == 0 {
		return errors.New("No files to convert!")
	}

	totalSize, totalCount, err := getFileStats(c.fs, "", cbrFiles...)
	if err != nil {
		return errors.Wrap(err, "getting non cbr file stats")
	}

	cbrSize, cbrCount, err := getFileStats(c.fs, ".cbr", cbrFiles...)
	if err != nil {
		return errors.Wrap(err, "getting cbr file stats")
	}

	c.logger.Printf("CBR2CBZ Batch Log\n")
	c.logger.Printf("Version %s\n", rootCmd.Version)
	c.logger.Printf("You can check for script updates at https://github.com/halkeye/cbr2cbz (original bash version at https://git.zaks.web.za/thisiszeev/cbr2cbz)\n")
	c.logger.Printf("Batch Start Date & Time: %s\n", time.Now().Format(time.RFC3339))
	c.logger.Printf("\n")
	c.logger.Printf("Considering %d files (%s)\n", totalCount, humanize.Bytes(totalSize))
	c.logger.Printf("   of which...\n")
	c.logger.Printf("Non CBR files: %d (%s)\n", totalCount-cbrCount, humanize.Bytes(totalSize-cbrSize))
	c.logger.Printf("CBR files: %d (%s)\n", len(cbrFiles), humanize.Bytes(cbrSize))

	for _, cbrFile := range cbrFiles {
		size := strings.TrimSuffix(filepath.Base(cbrFile), filepath.Ext(cbrFile))
		cbzFile := filepath.Join(filepath.Dir(cbrFile), size+".cbz")

		err = c.convert(context, cbrFile, cbzFile)

		if err != nil {
			c.logger.Printf("Error Reading %s - Skipping...%s\n", cbrFile, err.Error())
			failedFiles[cbrFile] = err
			continue
		}
	}

	c.printStats(startTime, failedFiles)

	return nil
}

func pathToFsPath(path string) string {
	return strings.TrimLeft(strings.TrimRight(path, "/"), "/")
}

func findCBRFiles(fsys fs.FS, root string) ([]string, error) {
	var files []string
	err := fs.WalkDir(fsys, pathToFsPath(root), func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.ToLower(filepath.Ext(path)) == ".cbr" {
			files = append(files, "/"+path)
		}
		return nil
	})
	return files, err
}

func (c converter) convert(ctx context.Context, cbrFile string, cbzFile string) error {
	c.logger.Printf("Converting: %s to %s\n", cbrFile, cbzFile)

	info, err := fs.Stat(c.fs, pathToFsPath(cbrFile))
	if err != nil {
		return errors.Wrap(err, "stating file")
	}

	if info.IsDir() {
		return errors.New("is a directory")
	}

	file, err := c.fs.Open(pathToFsPath(cbrFile))
	if err != nil {
		return errors.Wrap(err, "trying to open cbr")
	}
	defer file.Close()

	format, _, err := archiver.Identify(pathToFsPath(cbrFile), file)
	if err != nil && !errors.Is(err, archiver.ErrNoMatch) {
		return errors.Wrap(err, "unable to identify")
	}

	if _, ok := format.(archiver.Zip); ok {
		// secret zip file pretending to be rar
		hackpadfs.Rename(c.fs, pathToFsPath(cbrFile), pathToFsPath(cbzFile))
		return nil
	}

	if _, ok := format.(archiver.Rar); !ok {
		return errors.New("not a rar file")
	}

	inputStream := io.NewSectionReader(file.(io.ReaderAt), 0, info.Size())
	rarFS := archiver.ArchiveFS{Stream: inputStream, Format: archiver.Rar{}, Context: ctx}

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
	outFile, err := hackpadfs.Create(c.fs, pathToFsPath(cbzFile))
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

	err = hackpadfs.Remove(c.fs, pathToFsPath(cbrFile))
	if err != nil {
		return errors.Wrap(err, "deleting old cbr")
	}

	c.logger.Printf("Successfully Converted %s to %s...\n", cbrFile, cbzFile)

	return nil
}

func (c converter) printStats(startTime time.Time, failedFiles map[string]error) {
	runtime := humanize.RelTime(startTime, time.Now(), "", "")
	c.logger.Println("Failed files:")

	for filename, err := range failedFiles {
		c.logger.Printf("\t%s\t%s", filename, err.Error())
	}

	if len(failedFiles) == 0 {
		c.logger.Println("  none")
	}

	c.logger.Println("Runtime:", runtime)

	c.logger.Printf("A log file has been written to %s\n", logFileName)
}

func getFileStats(fsys hackpadfs.FS, ext string, paths ...string) (uint64, uint32, error) {
	var size uint64
	var count uint32

	for _, path := range paths {
		if len(ext) > 0 && strings.ToLower(filepath.Ext(path)) != ext {
			continue
		}

		info, err := fs.Stat(fsys, pathToFsPath(path))
		if err != nil {
			return 0, 0, errors.Wrap(err, "get file stats for specific file")
		}

		size += uint64(info.Size())
		count += 1
	}
	return size, count, nil
}
