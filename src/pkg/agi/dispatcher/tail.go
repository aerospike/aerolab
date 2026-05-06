package dispatcher

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"
)

type lineWriter func(line string, offset int64) error

func tailFile(ctx context.Context, path string, offset int64, write lineWriter) error {
	var inode uint64
	for {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		info, err := f.Stat()
		if err != nil {
			_ = f.Close()
			return err
		}
		inode = fileInode(info)
		if offset > 0 {
			if _, err := f.Seek(offset, io.SeekStart); err != nil {
				_ = f.Close()
				return err
			}
		} else if offset < 0 {
			if _, err := f.Seek(0, io.SeekEnd); err != nil {
				_ = f.Close()
				return err
			}
			if pos, err := f.Seek(0, io.SeekCurrent); err == nil {
				offset = pos
			}
		}
		err = tailOpenFile(ctx, path, f, inode, &offset, write)
		_ = f.Close()
		if err == nil || errors.Is(err, errRotated) {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, errRotated) {
				offset = 0
			}
			continue
		}
		return err
	}
}

var errRotated = errors.New("log rotated")

func tailOpenFile(ctx context.Context, path string, f *os.File, inode uint64, offset *int64, write lineWriter) error {
	reader := bufio.NewReader(f)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line, err := reader.ReadString('\n')
		if err == nil {
			*offset += int64(len(line))
			if len(line) > 0 && line[len(line)-1] == '\n' {
				line = line[:len(line)-1]
			}
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if err := write(line, *offset); err != nil {
				return err
			}
			continue
		}
		if err != io.EOF {
			return err
		}
		cur, serr := os.Stat(path)
		if serr == nil {
			curInode := fileInode(cur)
			if curInode != 0 && inode != 0 && curInode != inode {
				return errRotated
			}
			if cur.Size() < *offset {
				return errRotated
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func fileInode(info os.FileInfo) uint64 {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return uint64(st.Ino)
}

func tailStartOffset(backfill bool) int64 {
	if backfill {
		return 0
	}
	return -1
}

func validateSourceFile(path string) error {
	if path == "" {
		return fmt.Errorf("source file is empty")
	}
	return nil
}
