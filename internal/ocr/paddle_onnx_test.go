//go:build onnx

package ocr

import "testing"

func TestValidatePaddleDevice(t *testing.T) {
	tests := []struct {
		name    string
		device  string
		gpuID   int
		wantErr bool
	}{
		{name: "default CPU", device: ""},
		{name: "explicit CPU", device: "cpu"},
		{name: "CUDA device zero", device: "gpu", gpuID: 0},
		{name: "CUDA device index", device: "gpu", gpuID: 2},
		{name: "negative CUDA device", device: "gpu", gpuID: -1, wantErr: true},
		{name: "unsupported device", device: "tpu", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePaddleDevice(test.device, test.gpuID)
			if (err != nil) != test.wantErr {
				t.Fatalf("validatePaddleDevice(%q, %d) error = %v, wantErr %v", test.device, test.gpuID, err, test.wantErr)
			}
		})
	}
}
