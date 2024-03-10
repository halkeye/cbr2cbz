package cmd

import (
	"context"
	"io/fs"
	"os"
	"testing"

	"github.com/hack-pad/hackpadfs"
	memfs "github.com/hack-pad/hackpadfs/mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_runConvert(t *testing.T) {
	type args struct {
		cbrFiles []string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "legit cbr",
			args:    args{cbrFiles: []string{"test.cbr"}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys, err := memfs.NewFS()
			require.NoError(t, err)

			f, err := hackpadfs.Create(fsys, "test.cbr")
			require.NoError(t, err)

			realContents, err := os.ReadFile("../fixtures/test.cbr")
			require.NoError(t, err)

			_, err = hackpadfs.WriteFile(f, []byte(realContents))
			assert.NoError(t, err)
			assert.NoError(t, f.Close())

			fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
				t.Log(path)
				return nil
			})

			err = runConvert(context.Background(), fsys, tt.args.cbrFiles)
			if err != nil {
				if !tt.wantErr {
					require.NoError(t, err, "run convert shouldn't have an error")
				} else {
					require.Error(t, err, "run convert should have an error")
				}
			}
		})
	}
}
