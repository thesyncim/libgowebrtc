package media

import "testing"

func TestPTSFromTimestampUs(t *testing.T) {
	tests := []struct {
		name        string
		timestampUs int64
		clockRate   int64
		want        uint32
	}{
		{name: "zero timestamp", timestampUs: 0, clockRate: 90000, want: 0},
		{name: "negative timestamp", timestampUs: -1, clockRate: 90000, want: 0},
		{name: "zero clock", timestampUs: 1_000_000, clockRate: 0, want: 0},
		{name: "video one second", timestampUs: 1_000_000, clockRate: 90000, want: 90000},
		{name: "video thirty fps frame", timestampUs: 33_333, clockRate: 90000, want: 2999},
		{name: "audio twenty ms opus", timestampUs: 20_000, clockRate: 48000, want: 960},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ptsFromTimestampUs(tc.timestampUs, tc.clockRate); got != tc.want {
				t.Fatalf("ptsFromTimestampUs(%d, %d) = %d, want %d", tc.timestampUs, tc.clockRate, got, tc.want)
			}
		})
	}
}
