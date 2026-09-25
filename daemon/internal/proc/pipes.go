package proc

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// pipes are the standard input and output pipes of one child. They are plain operating system
// pipes rather than the ones exec.Cmd makes, because Wait closes those as soon as the child
// exits, which can cut off output that has not been read yet.
type pipes struct {
	stdoutReader *os.File
	stdoutWriter *os.File
	stdinReader  *os.File
	stdinWriter  *os.File
}

// openPipes makes the pipes and hands the child's ends to cmd.
func openPipes(cmd *exec.Cmd, withStdin bool) (*pipes, error) {
	p := &pipes{}
	var err error
	p.stdoutReader, p.stdoutWriter, err = os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("make the output pipe: %w", err)
	}
	cmd.Stdout = p.stdoutWriter
	if !withStdin {
		return p, nil
	}
	p.stdinReader, p.stdinWriter, err = os.Pipe()
	if err != nil {
		p.closeAll()
		return nil, fmt.Errorf("make the input pipe: %w", err)
	}
	cmd.Stdin = p.stdinReader
	return p, nil
}

// closeChildEnds closes the ends that now belong to the child.
func (p *pipes) closeChildEnds() {
	closeFile(p.stdoutWriter)
	closeFile(p.stdinReader)
}

// closeAll closes every end, for a start that failed.
func (p *pipes) closeAll() {
	for _, f := range []*os.File{p.stdoutReader, p.stdoutWriter, p.stdinReader, p.stdinWriter} {
		closeFile(f)
	}
}

// stdinWriterOrNil returns the input writer as an interface that is nil, not a nil file, when
// there is no input pipe.
func (p *pipes) stdinWriterOrNil() io.WriteCloser {
	if p.stdinWriter == nil {
		return nil
	}
	return p.stdinWriter
}

// closeFile closes a file that may be nil. The pipe ends are only closed when the start is over
// or has failed, so an error from Close has nothing left to report.
func closeFile(f *os.File) {
	if f != nil {
		_ = f.Close()
	}
}
