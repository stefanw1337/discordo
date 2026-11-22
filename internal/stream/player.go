package stream

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// PlayerConfig is the program that plays a stream's video, which it reads from its standard input.
type PlayerConfig struct {
	// Path is the program, such as mpv.
	Path string
	// Args come before those that tell it the format and title.
	Args []string
}

// DefaultPlayerArgs play a live stream in mpv with as little delay as it can: decoded on the GPU where it can be,
// shown as soon as a frame is decoded, and without a cache to fall behind in.
var DefaultPlayerArgs = []string{
	"--no-terminal",
	"--force-window=immediate",
	"--profile=low-latency",
	"--untimed",
	"--cache=no",
	"--hwdec=auto-safe",
}

// player is a running player of a stream's video in one codec.
type player struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	out   *bufio.Writer
	codec string
	ivf   *ivfWriter

	// done is closed once the player exits, after which err is why.
	done chan struct{}
	err  error
	once sync.Once
}

// startPlayer starts the player for video in codec, in a window titled title.
func startPlayer(cfg PlayerConfig, codec, title string) (*player, error) {
	if cfg.Path == "" {
		return nil, errors.New("no player set for streams")
	}

	var format string
	switch codec {
	case codecH264:
		format = "h264"
	case codecVP8:
		format = "ivf"
	default:
		return nil, fmt.Errorf("cannot play %s", codec)
	}

	args := append([]string{}, cfg.Args...)
	if cfg.Path == "mpv" || filepathBase(cfg.Path) == "mpv" || filepathBase(cfg.Path) == "mpv.exe" {
		args = append(args, "--demuxer-lavf-format="+format, "--title="+title, "-")
	} else {
		args = append(args, "-")
	}

	cmd := exec.Command(cfg.Path, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start %s: %w", cfg.Path, err)
	}

	p := &player{cmd: cmd, stdin: stdin, out: bufio.NewWriterSize(stdin, 1<<20), codec: codec, done: make(chan struct{})}
	if codec == codecVP8 {
		p.ivf = &ivfWriter{w: p.out}
	}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

// write sends the player a frame, decoded at timestamp, at once.
func (p *player) write(frame []byte, timestamp uint32) error {
	var err error
	if p.ivf != nil {
		err = p.ivf.writeFrame(frame, timestamp)
	} else {
		_, err = p.out.Write(frame)
	}
	if err == nil {
		err = p.out.Flush()
	}
	return err
}

// stop closes the player's input, which ends it, and kills it if it has not ended a moment later.
func (p *player) stop() {
	p.once.Do(func() {
		p.stdin.Close()
		select {
		case <-p.done:
		case <-time.After(time.Second):
			if p.cmd.Process != nil {
				p.cmd.Process.Kill()
			}
			<-p.done
		}
	})
}

// filepathBase returns the last element of path, split on either kind of separator, so that a Windows path is
// read right on any system.
func filepathBase(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}
