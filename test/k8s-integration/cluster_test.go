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

func TestCandidateGKELocations(t *testing.T) {
	arg, locs, err := candidateGKELocations("", "us-west1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if arg != "--region" {
		t.Errorf("got arg %s, want --region", arg)
	}
	if len(locs) < 2 || locs[0] != "us-west1" {
		t.Errorf("expected primary region us-west1 first, got %v", locs)
	}
	for i := 1; i < len(locs); i++ {
		if locs[i] == "us-west1" {
			t.Errorf("duplicate primary region in candidate list: %v", locs)
		}
	}

	arg, locs, err = candidateGKELocations("us-east1-b", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if arg != "--zone" {
		t.Errorf("got arg %s, want --zone", arg)
	}
	if len(locs) < 2 || locs[0] != "us-east1-b" {
		t.Errorf("expected primary zone us-east1-b first, got %v", locs)
	}
}

func TestIsGKEStockoutError(t *testing.T) {
	if !isGKEStockoutError("code: GCE_STOCKOUT\nmessage: 'Try a different location, or try again later: Google Compute Engine does not have enough resources available to fulfill request: us-central1.'") {
		t.Errorf("expected GCE_STOCKOUT output to be recognized as stockout")
	}
	if isGKEStockoutError("ERROR: invalid argument") {
		t.Errorf("did not expect invalid argument to be recognized as stockout")
	}
}
