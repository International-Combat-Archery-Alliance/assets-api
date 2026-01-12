package ptr

import (
	"testing"
	"time"
)

func TestInt(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"zero", 0, 0},
		{"positive", 42, 42},
		{"negative", -100, -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Int(tt.input)
			if got == nil {
				t.Fatal("expected non-nil pointer")
			}
			if *got != tt.want {
				t.Errorf("Int(%d) = %d, want %d", tt.input, *got, tt.want)
			}
		})
	}
}

func TestInt64(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  int64
	}{
		{"zero", 0, 0},
		{"positive", 9223372036854775807, 9223372036854775807},
		{"negative", -9223372036854775808, -9223372036854775808},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Int64(tt.input)
			if got == nil {
				t.Fatal("expected non-nil pointer")
			}
			if *got != tt.want {
				t.Errorf("Int64(%d) = %d, want %d", tt.input, *got, tt.want)
			}
		})
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"simple", "hello", "hello"},
		{"with spaces", "hello world", "hello world"},
		{"unicode", "你好世界", "你好世界"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := String(tt.input)
			if got == nil {
				t.Fatal("expected non-nil pointer")
			}
			if *got != tt.want {
				t.Errorf("String(%q) = %q, want %q", tt.input, *got, tt.want)
			}
		})
	}
}

func TestDuration(t *testing.T) {
	tests := []struct {
		name  string
		input time.Duration
		want  time.Duration
	}{
		{"zero", 0, 0},
		{"positive", 5 * time.Minute, 5 * time.Minute},
		{"negative", -10 * time.Second, -10 * time.Second},
		{"hours", 24 * time.Hour, 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Duration(tt.input)
			if got == nil {
				t.Fatal("expected non-nil pointer")
			}
			if *got != tt.want {
				t.Errorf("Duration(%v) = %v, want %v", tt.input, *got, tt.want)
			}
		})
	}
}

func TestTime(t *testing.T) {
	now := time.Now()
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2030, 12, 31, 23, 59, 59, 0, time.UTC)

	tests := []struct {
		name  string
		input time.Time
	}{
		{"now", now},
		{"past", past},
		{"future", future},
		{"zero", time.Time{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Time(tt.input)
			if got == nil {
				t.Fatal("expected non-nil pointer")
			}
			if !got.Equal(tt.input) {
				t.Errorf("Time(%v) = %v, want %v", tt.input, *got, tt.input)
			}
		})
	}
}
