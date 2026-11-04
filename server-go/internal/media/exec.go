// Package media wraps the ffmpeg/ffprobe subprocesses: running them with a
// timeout, probing source videos, and generating editor preview assets.
package media

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"stickers-server/internal/apperr"
	"stickers-server/internal/models"
)

// Msgs are the user-facing messages for the two ways a subprocess can fail.
type Msgs struct {
	Timeout string
	Failed  string
}

// tail returns at most the last n bytes of b as valid UTF-8.
func tail(b []byte, n int) string {
	if len(b) > n {
		b = b[len(b)-n:]
	}
	return strings.ToValidUTF8(string(b), "")
}

func head(b []byte, n int) string {
	if len(b) > n {
		b = b[:n]
	}
	return strings.ToValidUTF8(string(b), "")
}

// run executes name with args, killing it when timeout elapses. A timeout
// is reported as timedOut=true (and the process is killed and reaped).
func run(ctx context.Context, timeout time.Duration, name string, args ...string) (stdout, stderr []byte, timedOut bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var out, errBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err = cmd.Run()
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, nil, true, err
	}
	return out.Bytes(), errBuf.Bytes(), false, err
}

// RunFFmpeg runs `ffmpeg -y <args>`. A timeout becomes an models.Timeout
// error; any other failure becomes render_failed with the stderr tail as
// detail.
func RunFFmpeg(ctx context.Context, timeout time.Duration, msgs Msgs, args ...string) error {
	full := append([]string{"-y", "-hide_banner", "-nostats"}, args...)
	_, stderr, timedOut, err := run(ctx, timeout, "ffmpeg", full...)
	if timedOut {
		return apperr.New(models.Timeout, msgs.Timeout)
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err() // caller cancelled; not a render failure
		}
		detail := tail(stderr, 800)
		if detail == "" {
			detail = err.Error()
		}
		return apperr.New(models.RenderFailedCode, msgs.Failed).WithDetail(detail)
	}
	return nil
}
