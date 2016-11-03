package write

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
)

type Writer interface {
	Write(filename string, content []byte) error
}

type funcWriter func(string, []byte) error

var fileWriter = funcWriter(func(filename string, content []byte) error {
	return ioutil.WriteFile(filename, content, 0644)
})

func CreateWriter(diff bool, diffcmd string) (Writer, error) {
	if !diff {
		return NewFileWriter(), nil
	}
	return NewDiffWriter(diffcmd), nil
}

func NewFileWriter() Writer {
	return fileWriter
}

func NewDiffWriter(diffCmd string) Writer {
	return funcWriter(func(filename string, content []byte) error {
		renamed := fmt.Sprintf("%s.%d.renamed", filename, os.Getpid())
		if err := ioutil.WriteFile(renamed, content, 0644); err != nil {
			return err
		}
		defer os.Remove(renamed)

		diff, err := exec.Command(diffCmd, "-u", filename, renamed).CombinedOutput()
		if len(diff) > 0 {
			// diff exits with a non-zero status when the files don't match.
			// Ignore that failure as long as we get output.
			os.Stdout.Write(diff)
			return nil
		}
		if err != nil {
			return fmt.Errorf("computing diff: %v", err)
		}
		return nil
	})
}

func (f funcWriter) Write(filename string, content []byte) error {
	return f(filename, content)
}
