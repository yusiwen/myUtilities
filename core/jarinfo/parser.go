package jarinfo

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"io"
	"path/filepath"
)

type JarInfo struct {
	MinJDKVersion    string
	MinMajorVersion  int
	ClassCount       int
	VersionHistogram map[int]int
}

var jdkVersions = map[int]string{
	44: "1.0",
	45: "1.1",
	46: "1.2",
	47: "1.3",
	48: "1.4",
	49: "5",
	50: "6",
	51: "7",
	52: "8",
	53: "9",
	54: "10",
	55: "11",
	56: "12",
	57: "13",
	58: "14",
	59: "15",
	60: "16",
	61: "17",
	62: "18",
	63: "19",
	64: "20",
	65: "21",
	66: "22",
	67: "23",
	68: "24",
	69: "25",
}

func JDKVersionString(major int) string {
	if v, ok := jdkVersions[major]; ok {
		return v
	}
	return fmt.Sprintf("unknown(%d)", major)
}

func ParseJar(r io.ReaderAt, size int64, progress func(current, total int)) (*JarInfo, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	info := &JarInfo{
		VersionHistogram: make(map[int]int),
	}

	total := len(zr.File)

	for i, f := range zr.File {
		if progress != nil {
			progress(i+1, total)
		}

		if filepath.Ext(f.Name) != ".class" {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", f.Name, err)
		}

		var header [8]byte
		if _, err := io.ReadFull(rc, header[:]); err != nil {
			rc.Close()
			return nil, fmt.Errorf("read header from %s: %w", f.Name, err)
		}
		rc.Close()

		if binary.BigEndian.Uint32(header[0:4]) != 0xCAFEBABE {
			continue
		}

		major := int(binary.BigEndian.Uint16(header[6:8]))
		info.VersionHistogram[major]++
		info.ClassCount++

		if major > info.MinMajorVersion {
			info.MinMajorVersion = major
		}
	}

	if info.ClassCount == 0 {
		return nil, fmt.Errorf("no valid class files found in jar")
	}

	info.MinJDKVersion = JDKVersionString(info.MinMajorVersion)

	return info, nil
}
