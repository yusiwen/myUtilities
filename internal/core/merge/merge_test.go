package merge

import (
	"errors"
	"strings"
	"testing"
)

func runMerge(t *testing.T, base, ours, theirs string, opts Options) (string, int) {
	t.Helper()
	res, err := Merge([]byte(base), []byte(ours), []byte(theirs), opts)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	return string(res.Text), res.Conflicts
}

func TestMerge(t *testing.T) {
	labels := Labels{Ours: "ours", Base: "base", Theirs: "theirs"}
	tests := []struct {
		name               string
		base, ours, theirs string
		style              Style
		want               string
		wantConflicts      int
	}{
		{
			name:   "no changes",
			base:   "A\nB\n",
			ours:   "A\nB\n",
			theirs: "A\nB\n",
			style:  StyleMerge,
			want:   "A\nB\n",
		},
		{
			name:   "ours only",
			base:   "A\nB\nC\n",
			ours:   "A\nX\nC\n",
			theirs: "A\nB\nC\n",
			want:   "A\nX\nC\n",
		},
		{
			name:   "theirs only",
			base:   "A\nB\nC\n",
			ours:   "A\nB\nC\n",
			theirs: "A\nY\nC\n",
			want:   "A\nY\nC\n",
		},
		{
			name:   "identical change",
			base:   "A\nB\nC\n",
			ours:   "A\nX\nC\n",
			theirs: "A\nX\nC\n",
			want:   "A\nX\nC\n",
		},
		{
			name:   "disjoint changes with a common line between",
			base:   "A\nB\nC\nD\nE\n",
			ours:   "A\nX\nC\nD\nE\n",
			theirs: "A\nB\nC\nY\nE\n",
			want:   "A\nX\nC\nY\nE\n",
		},
		{
			// diff3 and git merge-file treat edits on abutting lines as one
			// conflict rather than interleaving them.
			name:          "abutting changes conflict",
			base:          "A\nB\nC\nD\n",
			ours:          "A\nX\nC\nD\n",
			theirs:        "A\nB\nY\nD\n",
			style:         StyleDiff3,
			want:          "A\n<<<<<<< ours\nX\nC\n||||||| base\nB\nC\n=======\nB\nY\n>>>>>>> theirs\nD\n",
			wantConflicts: 1,
		},
		{
			name:          "insert versus replace",
			base:          "A\nB\nC\n",
			ours:          "A\nX\nB\nC\n",
			theirs:        "A\nY\nC\n",
			style:         StyleDiff3,
			want:          "A\n<<<<<<< ours\nX\nB\n||||||| base\nB\n=======\nY\n>>>>>>> theirs\nC\n",
			wantConflicts: 1,
		},
		{
			name:          "insertions at the same point differ",
			base:          "A\nB\n",
			ours:          "A\nX\nB\n",
			theirs:        "A\nZ\nB\n",
			style:         StyleDiff3,
			want:          "A\n<<<<<<< ours\nX\n||||||| base\n=======\nZ\n>>>>>>> theirs\nB\n",
			wantConflicts: 1,
		},
		{
			name:   "insertions at the same point match",
			base:   "A\nB\n",
			ours:   "A\nX\nB\n",
			theirs: "A\nX\nB\n",
			want:   "A\nX\nB\n",
		},
		{
			name:          "delete versus modify",
			base:          "A\nB\nC\n",
			ours:          "A\nC\n",
			theirs:        "A\nB2\nC\n",
			style:         StyleDiff3,
			want:          "A\n<<<<<<< ours\n||||||| base\nB\n=======\nB2\n>>>>>>> theirs\nC\n",
			wantConflicts: 1,
		},
		{
			name:          "add/add on an empty base",
			base:          "",
			ours:          "O\n",
			theirs:        "T\n",
			style:         StyleDiff3,
			want:          "<<<<<<< ours\nO\n||||||| base\n=======\nT\n>>>>>>> theirs\n",
			wantConflicts: 1,
		},
		{
			name:          "merge style omits the base section",
			base:          "A\nB\nC\n",
			ours:          "A\nX\nC\n",
			theirs:        "A\nY\nC\n",
			style:         StyleMerge,
			want:          "A\n<<<<<<< ours\nX\n=======\nY\n>>>>>>> theirs\nC\n",
			wantConflicts: 1,
		},
		{
			name:   "missing final newline is preserved when clean",
			base:   "A\nB\n",
			ours:   "A\nB",
			theirs: "A\nB\n",
			want:   "A\nB",
		},
		{
			name:          "missing final newline is terminated before a marker",
			base:          "A\nB\n",
			ours:          "A\nY",
			theirs:        "A\nZ\n",
			style:         StyleDiff3,
			want:          "A\n<<<<<<< ours\nY\n||||||| base\nB\n=======\nZ\n>>>>>>> theirs\n",
			wantConflicts: 1,
		},
		{
			name: "empty inputs",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, conflicts := runMerge(t, tt.base, tt.ours, tt.theirs, Options{Style: tt.style, Labels: labels})
			if got != tt.want {
				t.Errorf("text:\n got %q\nwant %q", got, tt.want)
			}
			if conflicts != tt.wantConflicts {
				t.Errorf("conflicts = %d, want %d", conflicts, tt.wantConflicts)
			}
		})
	}
}

func TestMergeMarkerSizeAndLabels(t *testing.T) {
	got, conflicts := runMerge(t, "A\nB\n", "A\nX\n", "A\nY\n", Options{
		Style:      StyleDiff3,
		Labels:     Labels{Ours: "o", Base: "b", Theirs: "t"},
		MarkerSize: 3,
	})
	want := "A\n<<< o\nX\n||| b\nB\n===\nY\n>>> t\n"
	if got != want {
		t.Errorf("text:\n got %q\nwant %q", got, want)
	}
	if conflicts != 1 {
		t.Errorf("conflicts = %d, want 1", conflicts)
	}
}

func TestMergeDefaultLabels(t *testing.T) {
	got, _ := runMerge(t, "A\nB\n", "A\nX\n", "A\nY\n", Options{Style: StyleDiff3})
	for _, marker := range []string{"<<<<<<< ours\n", "||||||| base\n", ">>>>>>> theirs\n"} {
		if !strings.Contains(got, marker) {
			t.Errorf("output %q lacks default marker %q", got, marker)
		}
	}
}

func TestMergeRejectsInvalidOptions(t *testing.T) {
	if _, err := Merge(nil, nil, nil, Options{Style: Style(42)}); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("style error = %v, want ErrInvalidOptions", err)
	}
	if _, err := Merge(nil, nil, nil, Options{MarkerSize: -1}); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("marker size error = %v, want ErrInvalidOptions", err)
	}
}

func TestMergeSymmetry(t *testing.T) {
	base, ours, theirs := "A\nB\nC\nD\nE\n", "A\nX\nC\nD\nE\n", "A\nB\nC\nY\nE\n"

	// A clean merge is symmetric once the labels follow the sides.
	got, gotConflicts := runMerge(t, base, ours, theirs, Options{Style: StyleDiff3})
	swapped, swappedConflicts := runMerge(t, base, theirs, ours, Options{Style: StyleDiff3})
	if gotConflicts != 0 || swappedConflicts != 0 {
		t.Fatalf("clean merge reported conflicts: %d / %d", gotConflicts, swappedConflicts)
	}
	if got != swapped {
		t.Errorf("swapping sides changed a clean merge:\n got %q\nwant %q", got, swapped)
	}

	// A conflict keeps its shape: the same number of conflict regions either way.
	conflicted := "A\nB\nC\nD\n"
	first, c1 := runMerge(t, conflicted, "A\nX\nC\nD\n", "A\nB\nY\nD\n", Options{Style: StyleDiff3})
	second, c2 := runMerge(t, conflicted, "A\nB\nY\nD\n", "A\nX\nC\nD\n", Options{Style: StyleDiff3})
	if c1 != c2 || c1 == 0 {
		t.Fatalf("conflict counts differ: %d vs %d", c1, c2)
	}
	if first == second {
		t.Error("swapping conflicting sides should swap the rendered sides")
	}
}

func TestMergeIdempotentSides(t *testing.T) {
	base := "A\nB\nC\n"
	ours := "A\nX\nC\n"
	theirs := "A\nB\nZ\n"

	for name, tc := range map[string][3]string{
		"all base":         {base, base, base},
		"both ours":        {base, ours, ours},
		"theirs unchanged": {base, ours, base},
		"ours unchanged":   {base, base, theirs},
	} {
		want := tc[1]
		if name == "ours unchanged" {
			want = tc[2]
		}
		got, conflicts := runMerge(t, tc[0], tc[1], tc[2], Options{})
		if conflicts != 0 {
			t.Errorf("%s: conflicts = %d, want 0", name, conflicts)
		}
		if got != want {
			t.Errorf("%s: text = %q, want %q", name, got, want)
		}
	}
}

func TestMergeLargeInsertionKeepsOrder(t *testing.T) {
	var base strings.Builder
	for i := 0; i < 2000; i++ {
		base.WriteString("line\n")
	}
	ours := strings.Replace(base.String(), "line\nline\n", "line\ninserted\nline\n", 1)
	theirs := strings.Replace(base.String(), "line\nline\n", "line\nother\nline\n", 1)

	got, conflicts := runMerge(t, base.String(), ours, theirs, Options{Style: StyleMerge})
	if conflicts != 1 {
		t.Fatalf("conflicts = %d, want 1", conflicts)
	}
	if !strings.Contains(got, "inserted") || !strings.Contains(got, "other") {
		t.Errorf("conflict text lost a side: %q...", got[:200])
	}
}
