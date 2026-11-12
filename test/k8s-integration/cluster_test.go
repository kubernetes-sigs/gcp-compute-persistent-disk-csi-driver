package main

import (
	"errors"
	"os"
	"reflect"
	"testing"
)

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

func TestCandidateGCEZones(t *testing.T) {
	zones := candidateGCEZones("us-central1-b")
	if zones[0] != "us-central1-b" {
		t.Fatalf("expected primary zone first, got %v", zones)
	}
	seen := map[string]bool{}
	for _, z := range zones {
		if seen[z] {
			t.Errorf("duplicate zone %s in candidate list: %v", z, zones)
		}
		seen[z] = true
	}
	for _, want := range []string{"us-central1-a", "us-central1-c", "us-central1-f", "us-west1-b", "us-east1-b"} {
		if !seen[want] {
			t.Errorf("expected %s in candidate list: %v", want, zones)
		}
	}
	// Same-region zones come before the cross-region fallbacks.
	if zones[1] != "us-central1-a" {
		t.Errorf("expected same-region zone right after the primary zone, got %v", zones)
	}

	zones = candidateGCEZones("europe-west1-d")
	if zones[0] != "europe-west1-d" || zones[1] != "europe-west1-a" {
		t.Errorf("expected primary zone then its region siblings, got %v", zones)
	}
}

func TestIsGCEStockoutOutput(t *testing.T) {
	stockout := []string{
		"ERROR: (gcloud.compute.instance-groups.managed.wait-until) Timeout while waiting for group to become stable.",
		"Instance 'e2e-test-prow-minion-group-abcd' creation failed: The zone 'projects/p/zones/us-central1-b' does not have enough resources available to fulfill the request.",
		"- code: ZONE_RESOURCE_POOL_EXHAUSTED",
	}
	for _, out := range stockout {
		if !isGCEStockoutOutput(out) {
			t.Errorf("expected stockout to be recognized in %q", out)
		}
	}
	notStockout := []string{
		"Waiting for 4 ready nodes. 3 ready nodes, 4 registered. Retrying.",
		"ERROR: (gcloud.compute.instances.create) Could not fetch resource: Invalid value for field 'resource.machineType'",
	}
	for _, out := range notStockout {
		if isGCEStockoutOutput(out) {
			t.Errorf("did not expect stockout to be recognized in %q", out)
		}
	}
}

func TestSetGCEHyperdiskNodeEnvs(t *testing.T) {
	t.Run("sets the defaults", func(t *testing.T) {
		t.Setenv("NODE_SIZE", "")
		t.Setenv("NODE_DISK_TYPE", "")
		if err := setGCEHyperdiskNodeEnvs(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := os.Getenv("NODE_SIZE"); got != gceHyperdiskNodeMachineType {
			t.Errorf("NODE_SIZE = %q, want %q", got, gceHyperdiskNodeMachineType)
		}
		if got := os.Getenv("NODE_DISK_TYPE"); got != gceHyperdiskNodeDiskType {
			t.Errorf("NODE_DISK_TYPE = %q, want %q", got, gceHyperdiskNodeDiskType)
		}
	})

	t.Run("keeps values set in the environment", func(t *testing.T) {
		t.Setenv("NODE_SIZE", "e4-standard-4")
		t.Setenv("NODE_DISK_TYPE", "hyperdisk-balanced")
		if err := setGCEHyperdiskNodeEnvs(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := os.Getenv("NODE_SIZE"); got != "e4-standard-4" {
			t.Errorf("NODE_SIZE = %q, want the value from the environment", got)
		}
		if got := os.Getenv("NODE_DISK_TYPE"); got != "hyperdisk-balanced" {
			t.Errorf("NODE_DISK_TYPE = %q, want the value from the environment", got)
		}
	})
}

// fakeGCEClusterUp records the zones kube-up was attempted in and answers
// each attempt from results, keyed by zone.
type fakeGCEClusterUp struct {
	results   map[string]error
	outputs   map[string]string
	attempted []string
	downs     int
}

func (f *fakeGCEClusterUp) up(string) (string, error) {
	zone := os.Getenv("KUBE_GCE_ZONE")
	f.attempted = append(f.attempted, zone)
	return f.outputs[zone], f.results[zone]
}

func (f *fakeGCEClusterUp) down(string) error {
	f.downs++
	return nil
}

func installFakeGCEClusterUp(t *testing.T, f *fakeGCEClusterUp, available func(zone string) bool) {
	t.Helper()
	origUp, origDown, origAvail, origLog := runGCEClusterUp, runGCEClusterDown, gceMachineTypeInZone, logGCEInstanceGroupErrors
	t.Cleanup(func() {
		runGCEClusterUp, runGCEClusterDown, gceMachineTypeInZone, logGCEInstanceGroupErrors = origUp, origDown, origAvail, origLog
	})
	runGCEClusterUp = f.up
	runGCEClusterDown = f.down
	gceMachineTypeInZone = func(_, zone string) bool { return available(zone) }
	logGCEInstanceGroupErrors = func(string) {}
	t.Setenv("KUBE_GCE_ZONE", "")
	t.Setenv("NODE_SIZE", gceHyperdiskNodeMachineType)
}

func TestBringUpGCEClusterWithZoneFallback(t *testing.T) {
	const stockout = "ERROR: (gcloud.compute.instance-groups.managed.wait-until) Timeout while waiting for group to become stable."
	all := func(string) bool { return true }

	t.Run("succeeds in primary zone without fallback", func(t *testing.T) {
		f := &fakeGCEClusterUp{results: map[string]error{"us-central1-b": nil}}
		installFakeGCEClusterUp(t, f, func(string) bool { t.Fatal("machine type check not expected for primary zone"); return false })
		zone, err := bringUpGCEClusterWithZoneFallback("/k8s", "us-central1-b")
		if err != nil || zone != "us-central1-b" {
			t.Fatalf("got zone %q, err %v; want us-central1-b, nil", zone, err)
		}
		if len(f.attempted) != 1 || f.downs != 0 {
			t.Errorf("attempted %v, downs %d; want a single attempt and no teardown", f.attempted, f.downs)
		}
	})

	t.Run("falls back to the next zone on stockout", func(t *testing.T) {
		f := &fakeGCEClusterUp{
			results: map[string]error{"us-central1-b": errors.New("exit status 1"), "us-central1-a": nil},
			outputs: map[string]string{"us-central1-b": stockout},
		}
		installFakeGCEClusterUp(t, f, all)
		zone, err := bringUpGCEClusterWithZoneFallback("/k8s", "us-central1-b")
		if err != nil || zone != "us-central1-a" {
			t.Fatalf("got zone %q, err %v; want us-central1-a, nil", zone, err)
		}
		if want := []string{"us-central1-b", "us-central1-a"}; !reflect.DeepEqual(f.attempted, want) {
			t.Errorf("attempted %v, want %v", f.attempted, want)
		}
		if f.downs != 1 {
			t.Errorf("downs %d, want the partial cluster torn down once", f.downs)
		}
		if got := os.Getenv("KUBE_GCE_ZONE"); got != "us-central1-a" {
			t.Errorf("KUBE_GCE_ZONE = %q, want us-central1-a so that kube-down targets the right cluster", got)
		}
	})

	t.Run("skips zones without the node machine type", func(t *testing.T) {
		f := &fakeGCEClusterUp{
			results: map[string]error{"us-central1-b": errors.New("exit status 1"), "us-west1-b": nil},
			outputs: map[string]string{"us-central1-b": stockout},
		}
		installFakeGCEClusterUp(t, f, func(zone string) bool { return zone == "us-west1-b" })
		zone, err := bringUpGCEClusterWithZoneFallback("/k8s", "us-central1-b")
		if err != nil || zone != "us-west1-b" {
			t.Fatalf("got zone %q, err %v; want us-west1-b, nil", zone, err)
		}
		if want := []string{"us-central1-b", "us-west1-b"}; !reflect.DeepEqual(f.attempted, want) {
			t.Errorf("attempted %v, want %v", f.attempted, want)
		}
	})

	t.Run("does not retry a failure that is not a stockout", func(t *testing.T) {
		f := &fakeGCEClusterUp{
			results: map[string]error{"us-central1-b": errors.New("exit status 1")},
			outputs: map[string]string{"us-central1-b": "Detected 1 ready nodes, found 4 nodes out of expected 4."},
		}
		installFakeGCEClusterUp(t, f, all)
		if _, err := bringUpGCEClusterWithZoneFallback("/k8s", "us-central1-b"); err == nil {
			t.Fatal("expected error")
		}
		if len(f.attempted) != 1 || f.downs != 0 {
			t.Errorf("attempted %v, downs %d; want a single attempt and no teardown", f.attempted, f.downs)
		}
	})

	t.Run("gives up after all candidate zones are stocked out", func(t *testing.T) {
		f := &fakeGCEClusterUp{results: map[string]error{}, outputs: map[string]string{}}
		for _, zone := range candidateGCEZones("us-central1-b") {
			f.results[zone] = errors.New("exit status 1")
			f.outputs[zone] = stockout
		}
		installFakeGCEClusterUp(t, f, all)
		_, err := bringUpGCEClusterWithZoneFallback("/k8s", "us-central1-b")
		if err == nil {
			t.Fatal("expected error")
		}
		if n := len(candidateGCEZones("us-central1-b")); len(f.attempted) != n || f.downs != n-1 {
			t.Errorf("attempted %d zones with %d teardowns; want %d attempts and %d teardowns", len(f.attempted), f.downs, n, n-1)
		}
	})
}
