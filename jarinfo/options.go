package jarinfo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/yusiwen/myUtilities/core/jarinfo"
	"golang.org/x/term"
)

type Options struct {
	Info  InfoOptions  `cmd:"" name:"info" help:"Analyze a JAR file."`
	Serve ServeOptions `cmd:"" name:"serve" help:"Start JAR analyzer HTTP server."`
}

type InfoOptions struct {
	File string `arg:"" name:"file" help:"Path to JAR file."`
}

type ServeOptions struct {
	Port int `help:"Port to listen on." default:"8086"`
}

func (o *InfoOptions) Run() error {
	f, err := os.Open(o.File)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	isTerm := term.IsTerminal(int(os.Stderr.Fd()))
	var progress func(current, total int)
	if isTerm && fi.Size() > 10*1024*1024 {
		progress = func(current, total int) {
			fmt.Fprintf(os.Stderr, "\rProcessing: %d/%d", current, total)
			if current == total {
				fmt.Fprintf(os.Stderr, "\n")
			}
		}
	}

	info, err := jarinfo.ParseJar(f, fi.Size(), progress)
	if err != nil {
		return fmt.Errorf("parse jar: %w", err)
	}

	fmt.Printf("Target JDK:     %s\n", info.MinJDKVersion)
	fmt.Printf("Classes:        %d\n", info.ClassCount)
	fmt.Printf("Total entries:  %d\n", info.TotalEntries)

	ratio := uint64(0)
	if info.UncompressedSize > 0 {
		ratio = info.CompressedSize * 100 / info.UncompressedSize
	}
	fmt.Printf("Compressed:     %s \u2192 %s (%d%%)\n",
		humanSize(info.CompressedSize),
		humanSize(info.UncompressedSize), ratio)

	if info.Manifest != nil {
		fmt.Println("Manifest:")
		fmt.Printf("  Main-Class:            %s\n", orDash(info.Manifest.MainClass))
		fmt.Printf("  Created-By:            %s\n", orDash(info.Manifest.CreatedBy))
		fmt.Printf("  Build-Jdk:             %s\n", orDash(info.Manifest.BuildJDK))
		fmt.Printf("  Implementation-Version: %s\n", orDash(info.Manifest.ImplVersion))
		fmt.Printf("  Automatic-Module-Name:  %s\n", orDash(info.Manifest.AutomaticModuleName))
	}

	if info.Maven != nil {
		fmt.Printf("Maven:          %s:%s:%s\n", info.Maven.GroupID, info.Maven.ArtifactID, info.Maven.Version)
	}

	fmt.Printf("Signed:         %t\n", info.Signed)

	if len(info.VersionedClasses) > 0 {
		fmt.Println("Multi-release:  true")
		var versions []int
		for v := range info.VersionedClasses {
			versions = append(versions, v)
		}
		sort.Ints(versions)
		for _, v := range versions {
			fmt.Printf("  JDK %d: %d classes\n", v, info.VersionedClasses[v])
		}
	}

	fmt.Println("Version breakdown:")
	var majors []int
	for major := range info.VersionHistogram {
		majors = append(majors, major)
	}
	sort.Ints(majors)
	for _, major := range majors {
		count := info.VersionHistogram[major]
		fmt.Printf("  Java %-4s (%d): %d\n", jarinfo.JDKVersionString(major), major, count)
	}

	return nil
}

func (o *ServeOptions) Run() error {
	mux := http.NewServeMux()
	mux.Handle("/", FrontendHandler())
	RegisterHandlers(mux)
	fmt.Printf("JAR analyzer server listening on :%d\n", o.Port)
	return http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux)
}

func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/jarinfo/analyze", handleAnalyze)
}

func handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 100<<20)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, fmt.Sprintf("parse form: %v", err), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, fmt.Sprintf("read file: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".jar") {
		http.Error(w, "only .jar files are accepted", http.StatusBadRequest)
		return
	}

	tmpFile, err := os.CreateTemp("", "jar-*.jar")
	if err != nil {
		http.Error(w, fmt.Sprintf("create temp: %v", err), http.StatusInternalServerError)
		return
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		http.Error(w, fmt.Sprintf("write temp: %v", err), http.StatusInternalServerError)
		return
	}
	tmpFile.Close()

	f, err := os.Open(tmpName)
	if err != nil {
		http.Error(w, fmt.Sprintf("open temp: %v", err), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		http.Error(w, fmt.Sprintf("stat temp: %v", err), http.StatusInternalServerError)
		return
	}

	info, err := jarinfo.ParseJar(f, fi.Size(), nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("parse jar: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func humanSize(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div := uint64(unit)
	exp := 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
