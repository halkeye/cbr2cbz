package cmd

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/hack-pad/hackpadfs"
	memfs "github.com/hack-pad/hackpadfs/mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func Test_runConvert(t *testing.T) {
	type args struct {
		cbrFiles []string
	}
	tests := []struct {
		name     string
		args     args
		fileList []string
		fixtures []string
		wantErr  bool
	}{
		{
			name:     "legit cbr",
			args:     args{cbrFiles: []string{"test.cbr"}},
			fixtures: []string{"test.cbr"},
			fileList: []string{"test.cbz"},
			wantErr:  false,
		},
		{
			name:     "is actually cbz",
			args:     args{cbrFiles: []string{"is-zip.cbr"}},
			fixtures: []string{"is-zip.cbr"},
			fileList: []string{"is-zip.cbr"},
			wantErr:  false,
		},
		{
			name:     "recursive",
			args:     args{cbrFiles: []string{"."}},
			fixtures: []string{"test.cbr", "is-zip.cbr"},
			fileList: []string{"test.cbz", "is-zip.cbr"},
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys, err := memfs.NewFS()
			require.NoError(t, err)

			c := converter{
				fs:     fsys,
				logger: testLogger{t},
			}

			for _, path := range tt.fixtures {
				t.Logf("Adding %s to memfs", path)

				f, err := hackpadfs.Create(fsys, path)
				require.NoError(t, err)

				realContents, err := os.ReadFile(filepath.Join("..", "fixtures", path))
				require.NoError(t, err)

				_, err = hackpadfs.WriteFile(f, []byte(realContents))
				assert.NoError(t, err)
				assert.NoError(t, f.Close())
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
