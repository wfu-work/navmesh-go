package services

import "testing"

func TestParseReleasePlatform(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		wantOS   string
		wantArch string
	}{
		{name: "hyphen linux amd64", fileName: "navmesh-client-linux-amd64-v0.0.3.tar.gz", wantOS: "linux", wantArch: "amd64"},
		{name: "underscore linux amd64", fileName: "navmesh-client_linux_amd64_v0.0.3.tar.gz", wantOS: "linux", wantArch: "amd64"},
		{name: "windows x64", fileName: "navmesh-client-windows-x64.exe", wantOS: "windows", wantArch: "amd64"},
		{name: "macos arm64", fileName: "navmesh-client-macos-arm64.zip", wantOS: "darwin", wantArch: "arm64"},
		{name: "linux aarch64", fileName: "navmesh-client_linux_aarch64", wantOS: "linux", wantArch: "arm64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOS, gotArch := parseReleasePlatform(tt.fileName)
			if gotOS != tt.wantOS || gotArch != tt.wantArch {
				t.Fatalf("parseReleasePlatform(%q) = %q, %q; want %q, %q", tt.fileName, gotOS, gotArch, tt.wantOS, tt.wantArch)
			}
		})
	}
}

func TestParseReleaseBinaryPlatform(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantOS   string
		wantArch string
	}{
		{name: "elf amd64", data: elfHeader(62), wantOS: "linux", wantArch: "amd64"},
		{name: "elf arm64", data: elfHeader(183), wantOS: "linux", wantArch: "arm64"},
		{name: "pe amd64", data: peHeader(0x8664), wantOS: "windows", wantArch: "amd64"},
		{name: "pe arm64", data: peHeader(0xaa64), wantOS: "windows", wantArch: "arm64"},
		{name: "macho amd64", data: machoHeader(0x01000007), wantOS: "darwin", wantArch: "amd64"},
		{name: "macho arm64", data: machoHeader(0x0100000c), wantOS: "darwin", wantArch: "arm64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOS, gotArch := parseReleaseBinaryPlatform(tt.data)
			if gotOS != tt.wantOS || gotArch != tt.wantArch {
				t.Fatalf("parseReleaseBinaryPlatform() = %q, %q; want %q, %q", gotOS, gotArch, tt.wantOS, tt.wantArch)
			}
		})
	}
}

func elfHeader(machine byte) []byte {
	data := make([]byte, 64)
	copy(data, []byte{0x7f, 'E', 'L', 'F'})
	data[5] = 1
	data[18] = machine
	return data
}

func peHeader(machine uint16) []byte {
	data := make([]byte, 128)
	data[0] = 'M'
	data[1] = 'Z'
	data[0x3c] = 0x40
	copy(data[0x40:], []byte{'P', 'E', 0, 0})
	data[0x44] = byte(machine)
	data[0x45] = byte(machine >> 8)
	return data
}

func machoHeader(cpu uint32) []byte {
	data := make([]byte, 32)
	data[0] = 0xcf
	data[1] = 0xfa
	data[2] = 0xed
	data[3] = 0xfe
	data[4] = byte(cpu)
	data[5] = byte(cpu >> 8)
	data[6] = byte(cpu >> 16)
	data[7] = byte(cpu >> 24)
	return data
}
