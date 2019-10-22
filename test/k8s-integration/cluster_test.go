package main

import "testing"

func TestGetNormalizedVersion(t *testing.T) {
	tests := []struct {
		name      string
		kubeV     string
		gkeV      string
		expectV   string
		expectErr bool
	}{
		{
			name:    "kube",
			kubeV:   "1.13.5",
			expectV: "1.13",
		},
		{
			name:    "kube master",
			kubeV:   "master",
			expectV: "master",
		},
		{
			name:    "gke",
			gkeV:    "1.14.3",
			expectV: "1.14",
		},
		{
			name:    "gke minor",
			gkeV:    "1.14",
			expectV: "1.14",
		},
		{
			name:    "gke latest",
			gkeV:    "latest",
			expectV: "latest",
		},
		{
			name:    "kube minor",
			kubeV:   "1.13",
			expectV: "1.13",
		},
		{
			name:      "kube err",
			kubeV:     "1",
			expectErr: true,
		},
		{
			name:      "neither err",
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotV, err := getNormalizedVersion(tc.kubeV, tc.gkeV)
			if err != nil {
				if !tc.expectErr {
					t.Fatalf("Got unexpected err: %v", err)
				}
				return
			}
			if err == nil && tc.expectErr {
				t.Fatal("Got no error but expected one")
			}
			if gotV != tc.expectV {
				t.Errorf("Got version: %s, expected: %s", gotV, tc.expectV)
			}
		})
	}
}
