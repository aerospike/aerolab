package bgcp

import "testing"

func TestGcpRequiresHyperdiskBootDisk(t *testing.T) {
	t.Parallel()
	tests := []struct {
		instanceType string
		want         bool
	}{
		{"n4-standard-8", true},
		{"c4a-standard-8", true},
		{"c4a-highmem-16-lssd", true},
		{"e2-medium", false},
		{"n2-standard-4", false},
	}
	for _, tt := range tests {
		if got := gcpRequiresHyperdiskBootDisk(tt.instanceType); got != tt.want {
			t.Errorf("gcpRequiresHyperdiskBootDisk(%q) = %v, want %v", tt.instanceType, got, tt.want)
		}
	}
}

func TestApplyGCPInstanceDiskDefaults(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		disks        []string
		instanceType string
		want         []string
	}{
		{
			name:         "c4a empty disks",
			disks:        nil,
			instanceType: "c4a-standard-8",
			want:         []string{gcpHyperdiskBalancedDefault},
		},
		{
			name:         "c4a pd-balanced root",
			disks:        []string{"type=pd-balanced,size=40", "type=local-ssd"},
			instanceType: "c4a-standard-8",
			want:         []string{"type=hyperdisk-balanced,size=40", "type=local-ssd"},
		},
		{
			name:         "n4 pd-ssd root",
			disks:        []string{"type=pd-ssd,size=20"},
			instanceType: "n4-standard-16",
			want:         []string{"type=hyperdisk-balanced,size=20"},
		},
		{
			name:         "e2 unchanged",
			disks:        []string{"type=pd-balanced,size=20"},
			instanceType: "e2-medium",
			want:         []string{"type=pd-balanced,size=20"},
		},
		{
			name:         "c4a hyperdisk unchanged",
			disks:        []string{"type=hyperdisk-balanced,size=20"},
			instanceType: "c4a-standard-8",
			want:         []string{"type=hyperdisk-balanced,size=20"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyGCPInstanceDiskDefaults(tt.disks, tt.instanceType)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d: got %v want %v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("disk[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
