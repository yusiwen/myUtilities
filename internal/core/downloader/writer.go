package downloader

import "os"

// fileWriter is the partially downloaded file. Workers write into it with
// WriteAt (pwrite on Unix), which is safe to call concurrently and does not
// depend on a shared file offset.
type fileWriter struct {
	f    *os.File
	path string
}

// openWriter opens the partial file. fresh truncates any previous content;
// otherwise an existing partial file is reused for a resume. When preallocate
// is set, the file is sized up front (a sparse file on ext4/APFS/XFS) so that
// WriteAt offsets are valid immediately and disk-space problems surface early.
func openWriter(path string, size int64, fresh, preallocate bool, warnf func(string, ...any)) (*fileWriter, error) {
	flags := os.O_RDWR | os.O_CREATE
	if fresh {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return nil, err
	}
	if preallocate && size > 0 {
		if err := f.Truncate(size); err != nil {
			// Not fatal: fall back to growing the file as data arrives. Common
			// on filesystems without sparse-file support or without permission
			// to allocate the full size.
			if warnf != nil {
				warnf("cannot preallocate %s (%v); growing it on demand", path, err)
			}
		}
	}
	return &fileWriter{f: f, path: path}, nil
}

// WriteAt writes p at the given offset.
func (w *fileWriter) WriteAt(p []byte, off int64) (int, error) {
	return w.f.WriteAt(p, off)
}

// Write writes p at the current file offset (sequential/single-stream mode).
func (w *fileWriter) Write(p []byte) (int, error) {
	return w.f.Write(p)
}

// Sync flushes file contents to stable storage.
func (w *fileWriter) Sync() error {
	return w.f.Sync()
}

// Close releases the file handle.
func (w *fileWriter) Close() error {
	return w.f.Close()
}
