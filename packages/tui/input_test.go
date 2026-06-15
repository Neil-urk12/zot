package tui

import "testing"

func TestReaderParsesCSIUShiftEnter(t *testing.T) {
	k := readKey(t, "\x1b[13;2u")
	if k.Kind != KeyEnter || !k.Shift || k.Alt {
		t.Fatalf("Read kind=%v shift=%v alt=%v, want shift+enter", k.Kind, k.Shift, k.Alt)
	}
}

func TestReaderParsesModifyOtherKeysShiftEnter(t *testing.T) {
	k := readKey(t, "\x1b[27;2;13~")
	if k.Kind != KeyEnter || !k.Shift || k.Alt {
		t.Fatalf("Read kind=%v shift=%v alt=%v, want shift+enter", k.Kind, k.Shift, k.Alt)
	}
}

func TestReaderParsesCSIUCtrlC(t *testing.T) {
	k := readKey(t, "\x1b[99;5u")
	if k.Kind != KeyCtrlC || !k.Ctrl {
		t.Fatalf("Read kind=%v ctrl=%v, want ctrl+c", k.Kind, k.Ctrl)
	}
}

func TestReaderParsesModifyOtherKeysCtrlC(t *testing.T) {
	k := readKey(t, "\x1b[27;5;99~")
	if k.Kind != KeyCtrlC || !k.Ctrl {
		t.Fatalf("Read kind=%v ctrl=%v, want ctrl+c", k.Kind, k.Ctrl)
	}
}

func TestReaderParsesSGRMouseWheel(t *testing.T) {
	cases := []struct {
		seq  string
		want KeyKind
	}{
		{"\x1b[<64;10;20M", KeyMouseWheelUp},
		{"\x1b[<65;10;20M", KeyMouseWheelDown},
	}
	for _, tc := range cases {
		k := readKey(t, tc.seq)
		if k.Kind != tc.want {
			t.Fatalf("Read(%q) kind=%v, want %v", tc.seq, k.Kind, tc.want)
		}
	}
}

func readKey(t *testing.T, seq string) Key {
	t.Helper()
	idx := 0
	r := NewReader(func() (byte, error) {
		b := seq[idx]
		idx++
		return b, nil
	})
	k, err := r.Read()
	if err != nil {
		t.Fatalf("Read(%q): %v", seq, err)
	}
	return k
}
