package checker

import (
	"testing"
)

func TestEncodeGRPCFrame(t *testing.T) {
	payload := []byte{0x0a, 0x03, 'f', 'o', 'o'}
	frame := encodeGRPCFrame(payload)

	if frame[0] != 0 {
		t.Errorf("compress flag = %d, want 0", frame[0])
	}

	if len(frame) != 10 {
		t.Errorf("frame length = %d, want 10", len(frame))
	}

	decoded, err := decodeGRPCFrame(frame)
	if err != nil {
		t.Fatalf("decodeGRPCFrame: %v", err)
	}
	if string(decoded) != string(payload) {
		t.Errorf("decoded = %v, want %v", decoded, payload)
	}
}

func TestDecodeGRPCFrameErrors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"too short", []byte{0, 0}},
		{"truncated", []byte{0, 0, 0, 0, 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeGRPCFrame(tt.data)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestEncodeHealthRequest(t *testing.T) {
	t.Run("empty service", func(t *testing.T) {
		result := encodeHealthRequest("")
		if result != nil {
			t.Errorf("expected nil for empty service, got %v", result)
		}
	})

	t.Run("with service", func(t *testing.T) {
		result := encodeHealthRequest("myservice")
		if len(result) == 0 {
			t.Fatal("expected non-empty result")
		}
		if result[0] != 0x0a {
			t.Errorf("field tag = %02x, want 0x0a", result[0])
		}
		if result[1] != 9 {
			t.Errorf("string length = %d, want 9", result[1])
		}
		if string(result[2:]) != "myservice" {
			t.Errorf("service = %q, want %q", string(result[2:]), "myservice")
		}
	})
}

func TestDecodeHealthResponse(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want int32
	}{
		{"empty = SERVING", nil, 1},
		{"SERVING", []byte{0x08, 0x01}, 1},
		{"NOT_SERVING", []byte{0x08, 0x02}, 2},
		{"SERVICE_UNKNOWN", []byte{0x08, 0x03}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeHealthResponse(tt.data)
			if got != tt.want {
				t.Errorf("decodeHealthResponse() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGRPCFrameRoundTrip(t *testing.T) {
	empty := encodeGRPCFrame(nil)
	decoded, err := decodeGRPCFrame(empty)
	if err != nil {
		t.Fatalf("decode empty frame: %v", err)
	}
	if len(decoded) != 0 {
		t.Errorf("decoded empty frame length = %d, want 0", len(decoded))
	}
}
