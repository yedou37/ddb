package main

import (
	"net"
	"reflect"
	"testing"
)

func TestParsePorts(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []int
		wantErr bool
	}{
		{name: "empty", input: "", want: nil},
		{name: "sorted and deduped", input: "21002, 20082,21002", want: []int{20082, 21002}},
		{name: "invalid text", input: "abc", wantErr: true},
		{name: "invalid range", input: "70000", wantErr: true},
		{name: "empty entry", input: "20082,,21002", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePorts(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ports = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubnetMatches(t *testing.T) {
	ifaces := []ifaceIPv4{
		{
			name: "en0",
			cidr: "192.168.1.10/24",
			net: &net.IPNet{
				IP:   net.ParseIP("192.168.1.0").To4(),
				Mask: net.CIDRMask(24, 32),
			},
		},
		{
			name: "en1",
			cidr: "10.0.0.5/24",
			net: &net.IPNet{
				IP:   net.ParseIP("10.0.0.0").To4(),
				Mask: net.CIDRMask(24, 32),
			},
		},
	}

	matches := subnetMatches(ifaces, net.ParseIP("192.168.1.35").To4())
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].name != "en0" {
		t.Fatalf("matched interface = %s, want en0", matches[0].name)
	}

	noMatches := subnetMatches(ifaces, net.ParseIP("172.20.10.3").To4())
	if len(noMatches) != 0 {
		t.Fatalf("expected no matches, got %d", len(noMatches))
	}
}
