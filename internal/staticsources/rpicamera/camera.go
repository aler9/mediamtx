//go:build (linux && arm) || (linux && arm64)

package rpicamera

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
)

type camera struct {
	Params params
	OnData func(time.Duration, [][]byte)

	cmd      *exec.Cmd
	pipeOut  *pipe
	pipeIn   *pipe
	finalErr error

	terminate chan struct{}
	done      chan struct{}
}

func (c *camera) initialize() error {
	err := dumpComponent()
	if err != nil {
		return err
	}

	c.pipeOut, err = newPipe()
	if err != nil {
		freeComponent()
		return err
	}

	c.pipeIn, err = newPipe()
	if err != nil {
		c.pipeOut.close()
		freeComponent()
		return err
	}

	env := []string{
		"PIPE_CONF_FD=" + strconv.FormatInt(int64(c.pipeOut.readFD), 10),
		"PIPE_VIDEO_FD=" + strconv.FormatInt(int64(c.pipeIn.writeFD), 10),
		"LD_LIBRARY_PATH=" + dumpPath,
	}

	c.cmd = exec.Command(filepath.Join(dumpPath, executableName))
	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr
	c.cmd.Env = env
	c.cmd.Dir = dumpPath

	err = c.cmd.Start()
	if err != nil {
		c.pipeOut.close()
		c.pipeIn.close()
		freeComponent()
		return err
	}

	c.terminate = make(chan struct{})
	c.done = make(chan struct{})

	go c.run()

	c.pipeOut.write(append([]byte{'c'}, c.Params.serialize()...))

	return nil
}

func (c *camera) close() {
	close(c.terminate)
	<-c.done
	freeComponent()
}

func (c *camera) run() {
	defer close(c.done)
	c.finalErr = c.runInner()
}

func (c *camera) runInner() error {
	cmdDone := make(chan error)
	go func() {
		cmdDone <- c.cmd.Wait()
	}()

	readDone := make(chan error)
	go func() {
		readDone <- c.runReader()
	}()

	for {
		select {
		case err := <-cmdDone:
			c.pipeIn.close()
			c.pipeOut.close()

			<-readDone

			return err

		case err := <-readDone:
			c.pipeIn.close()

			c.pipeOut.write([]byte{'e'})
			c.pipeOut.close()

			<-cmdDone

			return err

		case <-c.terminate:
			c.pipeIn.close()
			<-readDone

			c.pipeOut.write([]byte{'e'})
			c.pipeOut.close()

			<-cmdDone

			return fmt.Errorf("terminated")
		}
	}
}

func (c *camera) runReader() error {
outer:
	for {
		buf, err := c.pipeIn.read()
		if err != nil {
			return err
		}

		switch buf[0] {
		case 'e':
			return fmt.Errorf(string(buf[1:]))

		case 'r':
			break outer

		default:
			return fmt.Errorf("unexpected data from pipe: '0x%.2x'", buf[0])
		}
	}

	for {
		buf, err := c.pipeIn.read()
		if err != nil {
			return err
		}

		switch buf[0] {
		case 'e':
			return fmt.Errorf(string(buf[1:]))

		case 'b':
			tmp := uint64(buf[8])<<56 | uint64(buf[7])<<48 | uint64(buf[6])<<40 | uint64(buf[5])<<32 |
				uint64(buf[4])<<24 | uint64(buf[3])<<16 | uint64(buf[2])<<8 | uint64(buf[1])
			dts := time.Duration(tmp) * time.Microsecond

			nalus, err := h264.AnnexBUnmarshal(buf[9:])
			if err != nil {
				return err
			}

			c.OnData(dts, nalus)

		default:
			return fmt.Errorf("unexpected data from pipe: '0x%.2x'", buf[0])
		}
	}
}

func (c *camera) reloadParams(params params) {
	c.pipeOut.write(append([]byte{'c'}, params.serialize()...))
}

func (c *camera) wait() error {
	<-c.done
	return c.finalErr
}
