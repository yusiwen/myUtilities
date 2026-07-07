package jarinfo

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
)

type JarInfo struct {
	MinJDKVersion    string
	MinMajorVersion  int
	ClassCount       int
	VersionHistogram map[int]int

	Manifest         *ManifestInfo
	Maven            *MavenInfo
	Signed           bool
	VersionedClasses map[int]int
	TotalEntries     int
	CompressedSize   uint64
	UncompressedSize uint64
}

type ManifestInfo struct {
	MainClass           string
	CreatedBy           string
	BuildJDK            string
	ImplVersion         string
	AutomaticModuleName string
	MultiRelease        bool
}

type MavenInfo struct {
	GroupID    string
	ArtifactID string
	Version    string
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
		VersionedClasses: make(map[int]int),
	}
	info.TotalEntries = len(zr.File)

	var hasSF, hasRSA bool

	for i, f := range zr.File {
		if progress != nil {
			progress(i+1, info.TotalEntries)
		}

		info.CompressedSize += f.CompressedSize64
		info.UncompressedSize += f.UncompressedSize64

		name := f.Name

		switch {
		case name == "META-INF/MANIFEST.MF":
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open MANIFEST.MF: %w", err)
			}
			info.Manifest = parseManifest(rc)
			rc.Close()

		case strings.HasPrefix(name, "META-INF/maven/") && strings.HasSuffix(name, "/pom.properties"):
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open %s: %w", name, err)
			}
			info.Maven = parsePomProperties(rc)
			rc.Close()

		case strings.HasPrefix(name, "META-INF/") && strings.HasSuffix(name, ".SF"):
			hasSF = true

		case strings.HasPrefix(name, "META-INF/"):
			ext := filepath.Ext(name)
			if ext == ".RSA" || ext == ".DSA" || ext == ".EC" {
				hasRSA = true
			}

		case strings.HasPrefix(name, "META-INF/versions/") && filepath.Ext(name) == ".class":
			remain := strings.TrimPrefix(name, "META-INF/versions/")
			parts := strings.SplitN(remain, "/", 2)
			if len(parts) >= 2 {
				if v, err := strconv.Atoi(parts[0]); err == nil {
					info.VersionedClasses[v]++
				}
			}

		case filepath.Ext(name) == ".class":
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open %s: %w", name, err)
			}

			var header [8]byte
			if _, err := io.ReadFull(rc, header[:]); err != nil {
				rc.Close()
				return nil, fmt.Errorf("read header from %s: %w", name, err)
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
	}

	info.Signed = hasSF && hasRSA

	if info.ClassCount == 0 {
		return nil, fmt.Errorf("no valid class files found in jar")
	}

	info.MinJDKVersion = JDKVersionString(info.MinMajorVersion)

	return info, nil
}

func parseManifest(r io.Reader) *ManifestInfo {
	data, _ := io.ReadAll(r)
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")

	entries := make(map[string]string)
	var currentKey string

	for _, line := range lines {
		if line == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if currentKey != "" {
				entries[currentKey] += strings.TrimSpace(line)
			}
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		currentKey = line[:colon]
		value := strings.TrimSpace(line[colon+1:])
		entries[currentKey] = value
	}

	return &ManifestInfo{
		MainClass:           entries["Main-Class"],
		CreatedBy:           entries["Created-By"],
		BuildJDK:            entries["Build-Jdk"],
		ImplVersion:         entries["Implementation-Version"],
		AutomaticModuleName: entries["Automatic-Module-Name"],
		MultiRelease:        entries["Multi-Release"] == "true",
	}
}

func parsePomProperties(r io.Reader) *MavenInfo {
	data, _ := io.ReadAll(r)
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")

	m := &MavenInfo{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		value := strings.TrimSpace(line[eq+1:])
		switch key {
		case "groupId":
			m.GroupID = value
		case "artifactId":
			m.ArtifactID = value
		case "version":
			m.Version = value
		}
	}
	if m.GroupID == "" && m.ArtifactID == "" && m.Version == "" {
		return nil
	}
	return m
}
