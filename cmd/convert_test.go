package cmd

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/hack-pad/hackpadfs"
	memfs "github.com/hack-pad/hackpadfs/mem"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	realCBRContents    []byte
	notrealCBRContents []byte
)

type filenameBytes map[string][]byte

func init() {
	var err error
	realCBRContents, err = os.ReadFile(absPathJoin("..", "fixtures", "test.cbr"))
	if err != nil {
		panic(err)
	}
	notrealCBRContents, err = os.ReadFile(absPathJoin("..", "fixtures", "is-zip.cbr"))
	if err != nil {
		panic(err)
	}
}

func setupFS(t *testing.T, fixtures filenameBytes) (hackpadfs.FS, error) {
	t.Helper()

	fsys, err := memfs.NewFS()
	require.NoError(t, err)
	for path, bytes := range fixtures {
		t.Logf("Adding %s to memfs", path)

		err := hackpadfs.MkdirAll(fsys, filepath.Dir(path), fs.ModePerm)
		require.NoError(t, err)

		f, err := hackpadfs.Create(fsys, path)
		require.NoError(t, err)

		_, err = hackpadfs.WriteFile(f, bytes)
		assert.NoError(t, err)
		assert.NoError(t, f.Close())
	}

	return fsys, nil
}

type testLogger struct {
	t *testing.T
}

func (l testLogger) Printf(format string, v ...any) {
	l.t.Helper()
	l.t.Logf(format, v...)
}

func (l testLogger) Println(v ...any) {
	l.t.Helper()
	l.t.Log(v...)
}

func absPathJoin(elem ...string) string {
	path := filepath.Join(elem...)
	path, err := filepath.Abs(path)
	if err != nil {
		panic(errors.Wrap(err, "calculating abs path"))
	}
	return path
}

func Test_runConvert(t *testing.T) {
	type args struct {
		cbrFiles []string
	}
	tests := []struct {
		name     string
		args     args
		fileList []string
		fixtures filenameBytes
		wantErr  bool
	}{
		{
			name: "legit cbr",
			args: args{cbrFiles: []string{"test.cbr"}},
			fixtures: filenameBytes{
				"test.cbr": realCBRContents,
			},
			fileList: []string{"test.cbz"},
			wantErr:  false,
		},
		{
			name: "is actually cbz",
			args: args{cbrFiles: []string{"is-zip.cbr"}},
			fixtures: filenameBytes{
				"is-zip.cbr": notrealCBRContents,
			},
			fileList: []string{"is-zip.cbz"},
			wantErr:  false,
		},
		{
			name: "recursive",
			args: args{cbrFiles: []string{"."}},
			fixtures: filenameBytes{
				"test.cbr":  realCBRContents,
				"test1.cbr": realCBRContents,
			},
			fileList: []string{"test.cbz", "test1.cbz"},
			wantErr:  false,
		},
		{
			name: "fullpaths_dir",
			args: args{cbrFiles: []string{"/path/to/dir"}},
			fixtures: filenameBytes{
				"path/to/dir/test.cbr":  realCBRContents,
				"path/to/dir/test1.cbr": realCBRContents,
			},
			fileList: []string{"path/to/dir/test.cbz", "path/to/dir/test1.cbz"},
			wantErr:  false,
		},
		{
			name: "fullpaths_file",
			args: args{cbrFiles: []string{"/path/to/dir/test.cbr"}},
			fixtures: filenameBytes{
				"path/to/dir/test.cbr": realCBRContents,
			},
			fileList: []string{"path/to/dir/test.cbz"},
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys, err := setupFS(t, tt.fixtures)
			require.NoError(t, err)

			c := &converter{
				fs:     fsys,
				logger: testLogger{t},
			}

			err = c.runConvert(context.Background(), tt.args.cbrFiles)
			if err != nil {
				if !tt.wantErr {
					require.NoError(t, err, "run convert shouldn't have an error")
				} else {
					require.Error(t, err, "run convert should have an error")
				}
			}

			fileList := []string{}

			err = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				if d.IsDir() {
					return nil
				}

				fileList = append(fileList, path)
				return nil
			})
			require.NoError(t, err, "getting list of files")

			t.Logf(fmt.Sprintf("fileList: %v || tt.fileList: %v", fileList, tt.fileList))

			require.ElementsMatch(t, fileList, tt.fileList, "files match in the end")
		})
	}
}

func Test_findFilesAndSize(t *testing.T) {
	tests := []struct {
		name     string
		fixtures filenameBytes
		wantErr  bool
		cbrFiles []string
		cbrSize  uint64
		allFiles []string
		allSize  uint64
	}{
		{
			name: "one each",
			fixtures: filenameBytes{
				"test.cbr": realCBRContents,
				"test.cbz": notrealCBRContents,
			},
			cbrFiles: []string{"test.cbr"},
			cbrSize:  140,
			allFiles: []string{"test.cbr", "test.cbz"},
			allSize:  485,
		},
		{
			name: "multiple each",
			fixtures: filenameBytes{
				"test1.cbr": realCBRContents,
				"test2.cbr": realCBRContents,
				"test3.cbz": notrealCBRContents,
				"test4.cbz": notrealCBRContents,
			},
			cbrFiles: []string{"test1.cbr", "test2.cbr"},
			cbrSize:  280,
			allFiles: []string{"test1.cbr", "test2.cbr", "test3.cbz", "test4.cbz"},
			allSize:  970,
		},
		{
			name: "fullpaths_dir",
			fixtures: filenameBytes{
				"path/to/dir/test.cbr": realCBRContents,
				"path/to/dir/test.cbz": notrealCBRContents,
			},
			cbrFiles: []string{"path/to/dir/test.cbr"},
			cbrSize:  140,
			allFiles: []string{"path/to/dir/test.cbr", "path/to/dir/test.cbz"},
			allSize:  485,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys, err := setupFS(t, tt.fixtures)
			require.NoError(t, err)

			c := &converter{
				fs:     fsys,
				logger: testLogger{t},
			}

			fileNames := []string{}
			for name := range tt.fixtures {
				fileNames = append(fileNames, name)
			}

			err = c.findFilesAndSize(context.Background(), fileNames)
			if err != nil {
				if !tt.wantErr {
					require.NoError(t, err, "findFilesAndSize shouldn't have an error")
				} else {
					require.Error(t, err, "findFilesAndSize should have an error")
				}
			}

			require.ElementsMatch(t, tt.allFiles, c.allFiles, "files match in the end")
			require.ElementsMatch(t, tt.cbrFiles, c.cbrFiles, "files match in the end")
			require.Equal(t, strconv.FormatUint(c.allSize, 10), strconv.FormatUint(tt.allSize, 10))
			require.Equal(t, strconv.FormatUint(c.cbrSize, 10), strconv.FormatUint(tt.cbrSize, 10))
		})
	}
}
