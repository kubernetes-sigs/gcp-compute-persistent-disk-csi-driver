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
	"testing"

	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRestoreDefaultAnnotation(t *testing.T) {
	tests := []struct {
		name                 string
		storageClass         *storagev1.StorageClass
		otherDefaults        []*storagev1.StorageClass
		expectErr            bool
		expectedStorageClass *storagev1.StorageClass
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
			expectedStorageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "standard-rwo",
					Annotations: map[string]string{
						defaultClassAnnotation: "true",
					},
				},
			},
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
			expectErr: false,
			expectedStorageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "standard-rwo",
					Annotations: map[string]string{},
				},
			},
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
			expectedStorageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "standard-rwo",
					Annotations: map[string]string{},
				},
			},
		},
		{
			name:          "storage class not found",
			storageClass:  nil,
			otherDefaults: []*storagev1.StorageClass{},
			expectErr:     false,
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
			expectedStorageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "standard-rwo",
					Annotations: map[string]string{
						defaultClassAnnotation: "true",
						"custom.io/config":     "value1",
						"app.example.com/ver":  "v1.0",
					},
				},
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
			expectErr: false,
			expectedStorageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "standard-rwo",
					Annotations: map[string]string{
						"provisioner.io/prio": "high",
						"security.io/policy":  "strict",
						"metadata.io/created": "2024-01-01",
					},
				},
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

			if tt.expectedStorageClass != nil {
				// Retrieve the updated StorageClass
				updatedSC, err := fakeClientset.StorageV1().StorageClasses().Get(ctx, "standard-rwo", metav1.GetOptions{})
				if err != nil && !apierrors.IsNotFound(err) {
					t.Errorf("failed to get storage class: %v", err)
					return
				}

				if updatedSC == nil {
					t.Errorf("storage class was not found")
					return
				}

				// Verify annotations match expected
				expectedAnnotations := tt.expectedStorageClass.Annotations
				if expectedAnnotations == nil {
					expectedAnnotations = make(map[string]string)
				}
				actualAnnotations := updatedSC.Annotations
				if actualAnnotations == nil {
					actualAnnotations = make(map[string]string)
				}

				if len(expectedAnnotations) != len(actualAnnotations) {
					t.Errorf("expected %d annotations, got %d: expected %v, got %v", len(expectedAnnotations), len(actualAnnotations), expectedAnnotations, actualAnnotations)
				}

				for key, expectedValue := range expectedAnnotations {
					actualValue, exists := actualAnnotations[key]
					if !exists {
						t.Errorf("annotation %q missing, expected %q", key, expectedValue)
					} else if actualValue != expectedValue {
						t.Errorf("annotation %q mismatch: expected %q, got %q", key, expectedValue, actualValue)
					}
				}

				for key := range actualAnnotations {
					if _, exists := expectedAnnotations[key]; !exists {
						t.Errorf("unexpected annotation %q: %q", key, actualAnnotations[key])
					}
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
