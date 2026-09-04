package collector

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// lineFunc handles one raw JSONL line from a tailed file. path is the source
// file. It returns any emissions to forward.
type lineFunc func(path string, line []byte) []Emission

// dirTailer walks one or more root directories for files matching a suffix,
// remembers a byte offset per file, and feeds new lines to a handler. It is
// intentionally poll-based (no fsnotify dependency) which is robust across
// platforms and network filesystems.
type dirTailer struct {
	roots    []string
	suffix   string // e.g. ".jsonl"
	interval time.Duration
	backfill time.Duration
	handle   lineFunc

	offsets map[string]int64
}

func newDirTailer(roots []string, suffix string, interval, backfill time.Duration, h lineFunc) *dirTailer {
	return &dirTailer{
		roots:    roots,
		suffix:   suffix,
		interval: interval,
		backfill: backfill,
		handle:   h,
		offsets:  map[string]int64{},
	}
}

func (t *dirTailer) run(ctx context.Context, out chan<- Emission, name string) error {
	if t.interval <= 0 {
		t.interval = 2 * time.Second
	}
	// First pass: seed offsets so that files last modified before the backfill
	// window are skipped entirely, and newer files are read from the start.
	t.scan(ctx, out, true)
	tick := time.NewTicker(t.interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			t.scan(ctx, out, false)
		}
	}
}

func (t *dirTailer) scan(ctx context.Context, out chan<- Emission, first bool) {
	cutoff := time.Time{}
	if first && t.backfill > 0 {
		cutoff = time.Now().Add(-t.backfill)
	}
	var files []string
	for _, root := range t.roots {
		if root == "" {
			continue
		}
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // unreadable dir: skip quietly
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(d.Name(), t.suffix) {
				return nil
			}
			files = append(files, p)
			return nil
		})
	}
	sort.Strings(files)
	for _, p := range files {
		select {
		case <-ctx.Done():
			return
		default:
		}
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if first {
			if !cutoff.IsZero() && info.ModTime().Before(cutoff) {
				t.offsets[p] = info.Size() // ignore old file entirely
				continue
			}
			if _, seen := t.offsets[p]; !seen {
				t.offsets[p] = 0 // read from start (within backfill window)
			}
		}
		off := t.offsets[p]
		if info.Size() < off {
			off = 0 // file truncated / rotated
		}
		if info.Size() == off {
			continue
		}
		newOff, ems := t.read(p, off)
		t.offsets[p] = newOff
		for _, e := range ems {
			emit(ctx, out, e)
		}
	}
}

func (t *dirTailer) read(path string, from int64) (int64, []Emission) {
	f, err := os.Open(path)
	if err != nil {
		return from, nil
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(from, io.SeekStart); err != nil {
		return from, nil
	}
	var ems []Emission
	r := bufio.NewReaderSize(f, 256*1024)
	pos := from
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && (err == nil || err == io.EOF) {
			if err == io.EOF {
				// Partial line without trailing newline: leave it for next scan.
				break
			}
			pos += int64(len(line))
			trimmed := strings.TrimSpace(string(line))
			if trimmed == "" {
				continue
			}
			ems = append(ems, t.handle(path, []byte(trimmed))...)
		}
		if err != nil {
			break
		}
	}
	return pos, ems
}

// expandHome turns a leading ~ into the user's home directory.
func expandHome(p string) string {
	if p == "" {
		return ""
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
