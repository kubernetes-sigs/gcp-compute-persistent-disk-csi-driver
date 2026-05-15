/*
Copyright 2026 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scdefaultrestorer

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestIsVersion136Plus(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		expected  bool
		expectErr bool
	}{
		{
			name:      "1.36 exact",
			version:   "1.36",
			expected:  true,
			expectErr: false,
		},
		{
			name:      "1.36 with v prefix",
			version:   "v1.36",
			expected:  true,
			expectErr: false,
		},
		{
			name:      "1.37",
			version:   "1.37",
			expected:  true,
			expectErr: false,
		},
		{
			name:      "2.0",
			version:   "2.0",
			expected:  true,
			expectErr: false,
		},
		{
			name:      "1.35",
			version:   "1.35",
			expected:  false,
			expectErr: false,
		},
		{
			name:      "1.0",
			version:   "1.0",
			expected:  false,
			expectErr: false,
		},
		{
			name:      "1.36 with patch",
			version:   "1.36.5",
			expected:  true,
			expectErr: false,
		},
		{
			name:      "1.36 with alpha suffix",
			version:   "1.36+alpha.2",
			expected:  true,
			expectErr: false,
		},
		{
			name:      "1.35 with alpha suffix",
			version:   "1.35+alpha.2",
			expected:  false,
			expectErr: false,
		},
		{
			name:      "invalid major version",
			version:   "x.36",
			expected:  false,
			expectErr: true,
		},
		{
			name:      "invalid minor version",
			version:   "1.x",
			expected:  false,
			expectErr: true,
		},
		{
			name:      "malformed version",
			version:   "1",
			expected:  false,
			expectErr: true,
		},
		{
			name:      "empty version",
			version:   "",
			expected:  false,
			expectErr: true,
		},
	}

	restorer := &Restorer{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := restorer.isVersion136Plus(tt.version)
			if tt.expectErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestWaitForKubeconfig(t *testing.T) {
	tests := []struct {
		name       string
		fileExists bool
		timeout    time.Duration
		expectErr  bool
	}{
		{
			name:       "file exists immediately",
			fileExists: true,
			timeout:    5 * time.Second,
			expectErr:  false,
		},
		{
			name:       "timeout waiting for file",
			fileExists: false,
			timeout:    100 * time.Millisecond,
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for testing
			tmpDir := t.TempDir()
			kubeconfigPath := tmpDir + "/kubeconfig"

			if tt.fileExists {
				// Create the file
				if err := os.WriteFile(kubeconfigPath, []byte("test"), 0600); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
			}

			config := Config{
				KubeconfigPath:    kubeconfigPath,
				KubeconfigTimeout: tt.timeout,
			}
			restorer := New(config)

			ctx := context.Background()
			err := restorer.waitForKubeconfig(ctx)

			if tt.expectErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRestoreDefaultAnnotation(t *testing.T) {
	tests := []struct {
		name                 string
		storageClass         *storagev1.StorageClass
		otherDefaults        []*storagev1.StorageClass
		expectErr            bool
		expectDefault        bool
		expectMarker         bool
		unrelatedAnnotations map[string]string
	}{
		{
			name: "restore as default when no other defaults exist",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "standard-rwo",
					Annotations: map[string]string{
						markerAnnotation: "true",
					},
				},
			},
			otherDefaults: []*storagev1.StorageClass{},
			expectErr:     false,
			expectDefault: true,
			expectMarker:  false,
		},
		{
			name: "remove marker only when other defaults exist",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "standard-rwo",
					Annotations: map[string]string{
						markerAnnotation: "true",
					},
				},
			},
			otherDefaults: []*storagev1.StorageClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pd-ssd",
						Annotations: map[string]string{
							defaultClassAnnotation: "true",
						},
					},
				},
			},
			expectErr:     false,
			expectDefault: false,
			expectMarker:  false,
		},
		{
			name: "no marker annotation present",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "standard-rwo",
					Annotations: map[string]string{},
				},
			},
			otherDefaults: []*storagev1.StorageClass{},
			expectErr:     false,
			expectDefault: false,
			expectMarker:  false,
		},
		{
			name:          "storage class not found",
			storageClass:  nil,
			otherDefaults: []*storagev1.StorageClass{},
			expectErr:     false,
			expectDefault: false,
			expectMarker:  false,
		},
		{
			name: "preserve unrelated annotations when restoring as default",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "standard-rwo",
					Annotations: map[string]string{
						markerAnnotation:      "true",
						"custom.io/config":    "value1",
						"app.example.com/ver": "v1.0",
					},
				},
			},
			otherDefaults: []*storagev1.StorageClass{},
			expectErr:     false,
			expectDefault: true,
			expectMarker:  false,
			unrelatedAnnotations: map[string]string{
				"custom.io/config":    "value1",
				"app.example.com/ver": "v1.0",
			},
		},
		{
			name: "preserve unrelated annotations when removing marker",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "standard-rwo",
					Annotations: map[string]string{
						markerAnnotation:      "true",
						"provisioner.io/prio": "high",
						"security.io/policy":  "strict",
						"metadata.io/created": "2024-01-01",
					},
				},
			},
			otherDefaults: []*storagev1.StorageClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pd-ssd",
						Annotations: map[string]string{
							defaultClassAnnotation: "true",
						},
					},
				},
			},
			expectErr:     false,
			expectDefault: false,
			expectMarker:  false,
			unrelatedAnnotations: map[string]string{
				"provisioner.io/prio": "high",
				"security.io/policy":  "strict",
				"metadata.io/created": "2024-01-01",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client
			scheme := runtime.NewScheme()
			storagev1.AddToScheme(scheme)

			objects := []runtime.Object{}
			if tt.storageClass != nil {
				objects = append(objects, tt.storageClass)
			}
			for _, sc := range tt.otherDefaults {
				objects = append(objects, sc)
			}

			fakeClientset := fake.NewClientset(objects...)

			config := Config{
				StorageClassName: "standard-rwo",
			}
			restorer := New(config)

			ctx := context.Background()
			err := restorer.restoreDefaultAnnotation(ctx, fakeClientset)

			if tt.expectErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if tt.storageClass != nil {
				// Retrieve the updated StorageClass
				updatedSC, err := fakeClientset.StorageV1().StorageClasses().Get(ctx, "standard-rwo", metav1.GetOptions{})
				if err != nil && !apierrors.IsNotFound(err) {
					t.Errorf("failed to get storage class: %v", err)
					return
				}

				if updatedSC == nil {
					return
				}

				// Check for default annotation
				hasDefault := updatedSC.Annotations != nil && updatedSC.Annotations[defaultClassAnnotation] == "true"
				if hasDefault != tt.expectDefault {
					t.Errorf("expected default=%v, got %v", tt.expectDefault, hasDefault)
				}

				// Check for marker annotation
				hasMarker := updatedSC.Annotations != nil && updatedSC.Annotations[markerAnnotation] == "true"
				if hasMarker != tt.expectMarker {
					t.Errorf("expected marker=%v, got %v", tt.expectMarker, hasMarker)
				}

				// Verify all unrelated annotations are preserved
				for key, expectedValue := range tt.unrelatedAnnotations {
					actualValue, exists := updatedSC.Annotations[key]
					if !exists {
						t.Errorf("annotation %q was removed, but should have been preserved", key)
					} else if actualValue != expectedValue {
						t.Errorf("annotation %q changed from %q to %q", key, expectedValue, actualValue)
					}
				}
			}
		})
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectErr   bool
		expectSkip  bool
		errContains string
	}{
		{
			name: "skip when default RWX is not false",
			config: Config{
				SCDefaultingByMAPEnabled: "true",
				EmulatedVersion:          "1.36",
				StorageClassName:         "standard-rwo",
			},
			expectErr:  false,
			expectSkip: true,
		},
		{
			name: "skip when version is below 1.36",
			config: Config{
				SCDefaultingByMAPEnabled: "false",
				EmulatedVersion:          "1.35",
				StorageClassName:         "standard-rwo",
			},
			expectErr:  false,
			expectSkip: true,
		},
		{
			name: "invalid version format",
			config: Config{
				SCDefaultingByMAPEnabled: "false",
				EmulatedVersion:          "invalid",
				StorageClassName:         "standard-rwo",
			},
			expectErr:   true,
			expectSkip:  false,
			errContains: "invalid emulated version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restorer := New(tt.config)
			ctx := context.Background()
			err := restorer.Run(ctx)

			if tt.expectErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.expectErr && err != nil && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %v", tt.errContains, err)
				}
			}
		})
	}
}

func TestRestoreDefaultAnnotationNoMarker(t *testing.T) {
	scheme := runtime.NewScheme()
	storagev1.AddToScheme(scheme)

	storageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "standard-rwo",
		},
	}

	fakeClientset := fake.NewClientset(storageClass)

	config := Config{
		StorageClassName: "standard-rwo",
	}
	restorer := New(config)

	ctx := context.Background()
	err := restorer.restoreDefaultAnnotation(ctx, fakeClientset)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify no annotation changes
	updatedSC, err := fakeClientset.StorageV1().StorageClasses().Get(ctx, "standard-rwo", metav1.GetOptions{})
	if err != nil {
		t.Errorf("failed to get storage class: %v", err)
	}

	if len(updatedSC.Annotations) > 0 {
		t.Errorf("expected no annotations, got %v", updatedSC.Annotations)
	}
}

func TestRestoreDefaultAnnotationMultipleDefaults(t *testing.T) {
	scheme := runtime.NewScheme()
	storagev1.AddToScheme(scheme)

	storageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "standard-rwo",
			Annotations: map[string]string{
				markerAnnotation: "true",
			},
		},
	}

	otherDefault1 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pd-ssd",
			Annotations: map[string]string{
				defaultClassAnnotation: "true",
			},
		},
	}

	otherDefault2 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pd-balanced",
			Annotations: map[string]string{
				defaultClassAnnotation: "true",
			},
		},
	}

	fakeClientset := fake.NewSimpleClientset(storageClass, otherDefault1, otherDefault2)

	config := Config{
		StorageClassName: "standard-rwo",
	}
	restorer := New(config)

	ctx := context.Background()
	err := restorer.restoreDefaultAnnotation(ctx, fakeClientset)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify marker is removed but default is not set
	updatedSC, err := fakeClientset.StorageV1().StorageClasses().Get(ctx, "standard-rwo", metav1.GetOptions{})
	if err != nil {
		t.Errorf("failed to get storage class: %v", err)
	}

	hasDefault := updatedSC.Annotations != nil && updatedSC.Annotations[defaultClassAnnotation] == "true"
	hasMarker := updatedSC.Annotations != nil && updatedSC.Annotations[markerAnnotation] == "true"

	if hasDefault {
		t.Errorf("expected default=false, got true")
	}
	if hasMarker {
		t.Errorf("expected marker=false, got true")
	}
}
