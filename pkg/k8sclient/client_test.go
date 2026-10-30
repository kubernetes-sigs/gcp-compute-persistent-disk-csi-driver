/*
Copyright 2025 The Kubernetes Authors.

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

package k8sclient

import (
	"context"
	"reflect"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	storagev1beta1 "k8s.io/api/storage/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const testAnnotationKey = "pdcsi.gke.io/disk-type-conversion-operation"

// withFakeClient points GetClient at clientset for the duration of a test, and
// shortens the backoff so failure cases don't spend seconds sleeping.
//
// GetClient and backoff are package globals, so tests in this file must not
// call t.Parallel(): a parallel test would swap them out from under another.
func withFakeClient(t *testing.T, clientset kubernetes.Interface) {
	t.Helper()
	originalGetClient := GetClient
	originalBackoff := backoff
	GetClient = func() (kubernetes.Interface, error) { return clientset, nil }
	backoff = wait.Backoff{Duration: time.Millisecond, Factor: 1.0, Steps: 3}
	t.Cleanup(func() {
		GetClient = originalGetClient
		backoff = originalBackoff
	})
}

func newPV(name string, annotations map[string]string) *v1.PersistentVolume {
	return &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: annotations},
	}
}

func getAnnotations(ctx context.Context, t *testing.T, clientset kubernetes.Interface, pvName string) map[string]string {
	t.Helper()
	pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get PersistentVolume %s: %v", pvName, err)
	}
	return pv.Annotations
}

func TestSetPVAnnotation(t *testing.T) {
	testCases := []struct {
		name     string
		existing map[string]string
		value    string
		expected string
	}{
		{
			name:     "adds annotation when none exist",
			existing: nil,
			value:    "Pending",
			expected: "Pending",
		},
		{
			name:     "adds annotation alongside existing ones",
			existing: map[string]string{"unrelated": "keep-me"},
			value:    "Pending",
			expected: "Pending",
		},
		{
			name:     "overwrites an existing value",
			existing: map[string]string{testAnnotationKey: "Pending"},
			value:    "https://www.googleapis.com/compute/alpha/projects/p/zones/z/operations/op-1",
			expected: "https://www.googleapis.com/compute/alpha/projects/p/zones/z/operations/op-1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			clientset := fake.NewSimpleClientset(newPV("test-pv", tc.existing))
			withFakeClient(t, clientset)

			if err := SetPVAnnotation(ctx, "test-pv", testAnnotationKey, tc.value); err != nil {
				t.Fatalf("SetPVAnnotation failed: %v", err)
			}

			annotations := getAnnotations(ctx, t, clientset, "test-pv")
			if got := annotations[testAnnotationKey]; got != tc.expected {
				t.Errorf("Got annotation %q; want %q", got, tc.expected)
			}
			for k, v := range tc.existing {
				if k == testAnnotationKey {
					continue
				}
				if annotations[k] != v {
					t.Errorf("Unrelated annotation %s = %q; want %q", k, annotations[k], v)
				}
			}
		})
	}
}

func TestRemovePVAnnotation(t *testing.T) {
	testCases := []struct {
		name     string
		existing map[string]string
	}{
		{
			name:     "removes an existing annotation",
			existing: map[string]string{testAnnotationKey: "Pending"},
		},
		{
			// A JSON patch "remove" would fail here with a 422, so this is the
			// case the merge patch exists for.
			name:     "removing an absent annotation succeeds",
			existing: map[string]string{"unrelated": "keep-me"},
		},
		{
			name:     "removing from a PV with no annotations succeeds",
			existing: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			clientset := fake.NewSimpleClientset(newPV("test-pv", tc.existing))
			withFakeClient(t, clientset)

			if err := RemovePVAnnotation(ctx, "test-pv", testAnnotationKey); err != nil {
				t.Fatalf("RemovePVAnnotation failed: %v", err)
			}

			annotations := getAnnotations(ctx, t, clientset, "test-pv")
			if _, ok := annotations[testAnnotationKey]; ok {
				t.Errorf("Annotation %s is still present: %v", testAnnotationKey, annotations)
			}
			if v, ok := tc.existing["unrelated"]; ok && annotations["unrelated"] != v {
				t.Errorf("Unrelated annotation was dropped: %v", annotations)
			}
		})
	}
}

func TestUpdatePVAnnotationRemovalValues(t *testing.T) {
	// "" and "null" are the sentinels callers use to mean removal.
	for _, value := range []string{"", "null"} {
		t.Run("value "+value, func(t *testing.T) {
			ctx := context.Background()
			clientset := fake.NewSimpleClientset(newPV("test-pv", map[string]string{testAnnotationKey: "Pending"}))
			withFakeClient(t, clientset)

			if err := UpdatePVAnnotation(ctx, "test-pv", testAnnotationKey, value); err != nil {
				t.Fatalf("UpdatePVAnnotation failed: %v", err)
			}

			if _, ok := getAnnotations(ctx, t, clientset, "test-pv")[testAnnotationKey]; ok {
				t.Errorf("UpdatePVAnnotation with value %q did not remove the annotation", value)
			}
		})
	}
}

func TestPatchPVAnnotationInvalidInput(t *testing.T) {
	testCases := []struct {
		name  string
		pv    string
		key   string
		value string
	}{
		{name: "empty PV name", pv: "", key: testAnnotationKey, value: "Pending"},
		{name: "empty annotation key", pv: "test-pv", key: "", value: "Pending"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(newPV("test-pv", nil))
			withFakeClient(t, clientset)

			if err := SetPVAnnotation(context.Background(), tc.pv, tc.key, tc.value); err == nil {
				t.Errorf("SetPVAnnotation(%q, %q) = nil; want error", tc.pv, tc.key)
			}
		})
	}
}

// countingReactor fails every patch with err and records how many were made.
func countingReactor(clientset *fake.Clientset, err error) *int {
	var calls int
	clientset.PrependReactor("patch", "persistentvolumes", func(k8stesting.Action) (bool, runtime.Object, error) {
		calls++
		return true, nil, err
	})
	return &calls
}

func TestPatchPVAnnotationRetryBehaviour(t *testing.T) {
	pvResource := schema.GroupResource{Resource: "persistentvolumes"}

	testCases := []struct {
		name          string
		patchErr      error
		isExpectedErr func(error) bool
		expectedCalls int
	}{
		{
			// The API server understood the request and refused it, so retrying
			// only delays the CSI call that is waiting on this.
			name:          "not found is not retried",
			patchErr:      apierrors.NewNotFound(pvResource, "test-pv"),
			isExpectedErr: apierrors.IsNotFound,
			expectedCalls: 1,
		},
		{
			name:          "forbidden is not retried",
			patchErr:      apierrors.NewForbidden(pvResource, "test-pv", nil),
			isExpectedErr: apierrors.IsForbidden,
			expectedCalls: 1,
		},
		{
			name:          "server timeout is retried",
			patchErr:      apierrors.NewServerTimeout(pvResource, "patch", 1),
			isExpectedErr: apierrors.IsServerTimeout,
			expectedCalls: 3,
		},
		{
			name:          "conflict is retried",
			patchErr:      apierrors.NewConflict(pvResource, "test-pv", nil),
			isExpectedErr: apierrors.IsConflict,
			expectedCalls: 3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(newPV("test-pv", nil))
			calls := countingReactor(clientset, tc.patchErr)
			withFakeClient(t, clientset)

			err := SetPVAnnotation(context.Background(), "test-pv", testAnnotationKey, "Pending")
			if err == nil {
				t.Fatal("SetPVAnnotation = nil; want error")
			}
			// The backoff on its own reports only that it gave up, so check that
			// the cause survived.
			if !tc.isExpectedErr(err) {
				t.Errorf("SetPVAnnotation error %v does not wrap the %v API error", err, tc.patchErr)
			}
			if *calls != tc.expectedCalls {
				t.Errorf("Made %d patch calls; want %d", *calls, tc.expectedCalls)
			}
		})
	}
}

func TestEmitPVEvent(t *testing.T) {
	ctx := context.Background()
	pv := newPV("test-pv", nil)
	pv.UID = "b115f645-b9a5-44ea-82a4-80f92bc96535"
	clientset := fake.NewSimpleClientset(pv)
	withFakeClient(t, clientset)

	EmitPVEvent(ctx, "test-pv", v1.EventTypeNormal, "DiskTypeConversionStart", "ConvertDiskType",
		`Disk type conversion started for volume "test-pv"`)

	events, err := clientset.CoreV1().Events(metav1.NamespaceDefault).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list events: %v", err)
	}
	if len(events.Items) != 1 {
		t.Fatalf("Got %d events; want 1", len(events.Items))
	}

	event := events.Items[0]
	if event.Reason != "DiskTypeConversionStart" {
		t.Errorf("Got reason %q; want DiskTypeConversionStart", event.Reason)
	}
	if event.Action != "ConvertDiskType" {
		t.Errorf("Got action %q; want ConvertDiskType", event.Action)
	}
	if event.Type != v1.EventTypeNormal {
		t.Errorf("Got type %q; want %q", event.Type, v1.EventTypeNormal)
	}
	if event.Source.Component != EventComponent {
		t.Errorf("Got source component %q; want %q", event.Source.Component, EventComponent)
	}
	// The event has to point at the PersistentVolume for kubectl describe pv to
	// show it.
	if event.InvolvedObject.Kind != "PersistentVolume" || event.InvolvedObject.Name != "test-pv" {
		t.Errorf("Got involved object %+v; want the test-pv PersistentVolume", event.InvolvedObject)
	}
	if event.InvolvedObject.UID != pv.UID {
		t.Errorf("Got involved object UID %q; want %q", event.InvolvedObject.UID, pv.UID)
	}
}

func TestEmitPVEventIsBestEffort(t *testing.T) {
	testCases := []struct {
		name    string
		objects []runtime.Object
	}{
		{
			// Events are a notification, not the record of what happened, so a
			// volume with no PersistentVolume must not take down the caller.
			name:    "no PersistentVolume to attach the event to",
			objects: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			clientset := fake.NewSimpleClientset(tc.objects...)
			withFakeClient(t, clientset)

			EmitPVEvent(ctx, "test-pv", v1.EventTypeNormal, "DiskTypeConversionStart", "ConvertDiskType", "message")

			events, err := clientset.CoreV1().Events(metav1.NamespaceDefault).List(ctx, metav1.ListOptions{})
			if err != nil {
				t.Fatalf("Failed to list events: %v", err)
			}
			if len(events.Items) != 0 {
				t.Errorf("Got %d events; want none", len(events.Items))
			}
		})
	}
}

func TestGetVolumeAttributesClassForPV(t *testing.T) {
	boundPV := func(claimName string) *v1.PersistentVolume {
		pv := newPV("test-pv", nil)
		if claimName != "" {
			pv.Spec.ClaimRef = &v1.ObjectReference{Name: claimName, Namespace: "default"}
		}
		return pv
	}
	pvcWithClass := func(className *string) *v1.PersistentVolumeClaim {
		return &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pvc", Namespace: "default"},
			Spec:       v1.PersistentVolumeClaimSpec{VolumeAttributesClassName: className},
		}
	}
	vac := &storagev1.VolumeAttributesClass{
		ObjectMeta: metav1.ObjectMeta{Name: "vac-hyperdisk"},
		DriverName: "pd.csi.storage.gke.io",
		Parameters: map[string]string{"type": "hyperdisk-balanced", "iops": "3000"},
	}
	className := "vac-hyperdisk"
	emptyClassName := ""

	testCases := []struct {
		name      string
		objects   []runtime.Object
		expParams map[string]string
		expExists bool
		expectErr bool
	}{
		{
			name:      "returns the parameters of the class the claim names",
			objects:   []runtime.Object{boundPV("test-pvc"), pvcWithClass(&className), vac},
			expParams: map[string]string{"type": "hyperdisk-balanced", "iops": "3000"},
			expExists: true,
		},
		{
			// This is how a user cancels work the driver has queued.
			name:      "reports no class when the claim no longer names one",
			objects:   []runtime.Object{boundPV("test-pvc"), pvcWithClass(nil), vac},
			expExists: false,
		},
		{
			name:      "reports no class when the claim names an empty one",
			objects:   []runtime.Object{boundPV("test-pvc"), pvcWithClass(&emptyClassName), vac},
			expExists: false,
		},
		{
			name:      "reports no class when the volume is not bound to a claim",
			objects:   []runtime.Object{boundPV(""), vac},
			expExists: false,
		},
		{
			// A claim is namespaced, so a reference without a namespace
			// cannot be resolved to one.
			name: "reports no class when the claim reference has no namespace",
			objects: []runtime.Object{func() *v1.PersistentVolume {
				pv := boundPV("test-pvc")
				pv.Spec.ClaimRef.Namespace = ""
				return pv
			}(), pvcWithClass(&className), vac},
			expExists: false,
		},
		{
			name:      "reports no class when the claim is gone",
			objects:   []runtime.Object{boundPV("test-pvc"), vac},
			expExists: false,
		},
		{
			name:      "reports no class when the class is gone",
			objects:   []runtime.Object{boundPV("test-pvc"), pvcWithClass(&className)},
			expExists: false,
		},
		{
			name:      "reports no class when the volume has no PersistentVolume",
			objects:   nil,
			expExists: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tc.objects...)
			withFakeClient(t, clientset)

			params, exists, err := GetVolumeAttributesClassForPV(context.Background(), "test-pv")
			if tc.expectErr {
				if err == nil {
					t.Fatalf("GetVolumeAttributesClassForPV = %v, %v, nil; want error", params, exists)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetVolumeAttributesClassForPV failed: %v", err)
			}
			if exists != tc.expExists {
				t.Errorf("Got exists %v; want %v", exists, tc.expExists)
			}
			if !reflect.DeepEqual(params, tc.expParams) {
				t.Errorf("Got parameters %v; want %v", params, tc.expParams)
			}
		})
	}
}

// Transient PVC and VAC read errors must retry before reconciliation.
func TestGetVolumeAttributesClassForPVRetriesTransientReads(t *testing.T) {
	oldBackoff := backoff
	backoff = wait.Backoff{Duration: time.Millisecond, Factor: 1, Steps: 3}
	defer func() { backoff = oldBackoff }()

	className := "vac-hyperdisk"
	pv := newPV("test-pv", nil)
	pv.Spec.ClaimRef = &v1.ObjectReference{Name: "test-pvc", Namespace: "default"}
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pvc", Namespace: "default"},
		Spec:       v1.PersistentVolumeClaimSpec{VolumeAttributesClassName: &className},
	}
	vac := &storagev1.VolumeAttributesClass{
		ObjectMeta: metav1.ObjectMeta{Name: className},
		DriverName: "pd.csi.storage.gke.io",
		Parameters: map[string]string{"type": "hyperdisk-balanced"},
	}
	clientset := fake.NewSimpleClientset(pv, pvc, vac)
	pvcGets, vacGets, clientGets := 0, 0, 0
	clientset.PrependReactor("get", "persistentvolumeclaims", func(action k8stesting.Action) (bool, runtime.Object, error) {
		pvcGets++
		if pvcGets == 1 {
			return true, nil, apierrors.NewServiceUnavailable("temporary PVC read failure")
		}
		return false, nil, nil
	})
	clientset.PrependReactor("get", "volumeattributesclasses", func(action k8stesting.Action) (bool, runtime.Object, error) {
		vacGets++
		if vacGets == 1 {
			return true, nil, apierrors.NewServiceUnavailable("temporary VAC read failure")
		}
		return false, nil, nil
	})
	oldGetClient := GetClient
	GetClient = func() (kubernetes.Interface, error) {
		clientGets++
		return clientset, nil
	}
	defer func() { GetClient = oldGetClient }()

	params, exists, err := GetVolumeAttributesClassForPV(context.Background(), "test-pv")
	if err != nil {
		t.Fatalf("GetVolumeAttributesClassForPV failed after transient reads: %v", err)
	}
	if !exists || !reflect.DeepEqual(params, map[string]string{"type": "hyperdisk-balanced"}) {
		t.Fatalf("GetVolumeAttributesClassForPV = %v, %v; want the class parameters", params, exists)
	}
	if pvcGets != 2 || vacGets != 2 {
		t.Errorf("PVC gets = %d and VAC gets = %d; want two each", pvcGets, vacGets)
	}
	if clientGets != 1 {
		t.Errorf("Kubernetes clients fetched = %d; want one", clientGets)
	}
}

func TestGetVolumeAttributesClassForPVAPIVersions(t *testing.T) {
	boundPV := func() *v1.PersistentVolume {
		pv := newPV("test-pv", nil)
		pv.Spec.ClaimRef = &v1.ObjectReference{Name: "test-pvc", Namespace: "default"}
		return pv
	}
	className := "vac-hyperdisk"
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pvc", Namespace: "default"},
		Spec:       v1.PersistentVolumeClaimSpec{VolumeAttributesClassName: &className},
	}
	params := map[string]string{"type": "hyperdisk-balanced"}

	testCases := []struct {
		name      string
		vac       runtime.Object
		expParams map[string]string
		expExists bool
	}{
		{
			name: "reads the class from storage v1",
			vac: &storagev1.VolumeAttributesClass{
				ObjectMeta: metav1.ObjectMeta{Name: className},
				DriverName: "pd.csi.storage.gke.io",
				Parameters: params,
			},
			expParams: params,
			expExists: true,
		},
		{
			// Clusters before Kubernetes 1.34 serve the class at v1beta1 only.
			// Reading the unserved v1 as a missing class would cancel every
			// conversion on those clusters.
			name: "falls back to storage v1beta1",
			vac: &storagev1beta1.VolumeAttributesClass{
				ObjectMeta: metav1.ObjectMeta{Name: className},
				DriverName: "pd.csi.storage.gke.io",
				Parameters: params,
			},
			expParams: params,
			expExists: true,
		},
		{
			name:      "reports no class when neither version has it",
			vac:       nil,
			expExists: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			objects := []runtime.Object{boundPV(), pvc}
			if tc.vac != nil {
				objects = append(objects, tc.vac)
			}
			clientset := fake.NewSimpleClientset(objects...)
			withFakeClient(t, clientset)

			gotParams, exists, err := GetVolumeAttributesClassForPV(context.Background(), "test-pv")
			if err != nil {
				t.Fatalf("GetVolumeAttributesClassForPV failed: %v", err)
			}
			if exists != tc.expExists {
				t.Errorf("Got exists %v; want %v", exists, tc.expExists)
			}
			if !reflect.DeepEqual(gotParams, tc.expParams) {
				t.Errorf("Got parameters %v; want %v", gotParams, tc.expParams)
			}
		})
	}
}

func TestClearVolumeAttributesClassForPV(t *testing.T) {
	className := "vac-hyperdisk"
	boundPV := func(namespace string) *v1.PersistentVolume {
		pv := newPV("test-pv", nil)
		pv.Spec.ClaimRef = &v1.ObjectReference{Name: "test-pvc", Namespace: namespace}
		return pv
	}
	pvcWithClass := func() *v1.PersistentVolumeClaim {
		return &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pvc", Namespace: "default"},
			Spec:       v1.PersistentVolumeClaimSpec{VolumeAttributesClassName: &className},
		}
	}
	pvcResource := schema.GroupResource{Resource: "persistentvolumeclaims"}

	testCases := []struct {
		name       string
		objects    []runtime.Object
		patchErr   error
		expCleared bool
		expErr     func(error) bool
		// expClass is the class the claim names afterwards, or nil if the
		// claim should not be checked.
		expClass *string
	}{
		{
			name:       "clears the class from the bound claim",
			objects:    []runtime.Object{boundPV("default"), pvcWithClass()},
			expCleared: true,
			expClass:   new(string),
		},
		{
			name:       "does nothing when the volume has no PersistentVolume",
			objects:    nil,
			expCleared: false,
		},
		{
			name:       "does nothing when the volume is not bound to a claim",
			objects:    []runtime.Object{newPV("test-pv", nil), pvcWithClass()},
			expCleared: false,
			expClass:   &className,
		},
		{
			name:       "does nothing when the claim reference has no namespace",
			objects:    []runtime.Object{boundPV(""), pvcWithClass()},
			expCleared: false,
			expClass:   &className,
		},
		{
			name:       "does nothing when the claim is gone",
			objects:    []runtime.Object{boundPV("default")},
			expCleared: false,
		},
		{
			// Kubernetes refuses to clear a class that has been applied, and
			// clusters before 1.34 refuse the change outright.
			name:       "leaves the class in place when the change is refused as invalid",
			objects:    []runtime.Object{boundPV("default"), pvcWithClass()},
			patchErr:   apierrors.NewInvalid(schema.GroupKind{Kind: "PersistentVolumeClaim"}, "test-pvc", nil),
			expCleared: false,
		},
		{
			name:       "leaves the class in place when the change is forbidden",
			objects:    []runtime.Object{boundPV("default"), pvcWithClass()},
			patchErr:   apierrors.NewForbidden(pvcResource, "test-pvc", nil),
			expCleared: false,
		},
		{
			name:       "returns other failures",
			objects:    []runtime.Object{boundPV("default"), pvcWithClass()},
			patchErr:   apierrors.NewServiceUnavailable("temporary PVC patch failure"),
			expCleared: false,
			expErr:     apierrors.IsServiceUnavailable,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tc.objects...)
			if tc.patchErr != nil {
				clientset.PrependReactor("patch", "persistentvolumeclaims", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tc.patchErr
				})
			}
			withFakeClient(t, clientset)

			cleared, err := ClearVolumeAttributesClassForPV(context.Background(), "test-pv")
			if tc.expErr != nil {
				if !tc.expErr(err) {
					t.Fatalf("ClearVolumeAttributesClassForPV error = %v; want %v", err, tc.patchErr)
				}
			} else if err != nil {
				t.Fatalf("ClearVolumeAttributesClassForPV failed: %v", err)
			}
			if cleared != tc.expCleared {
				t.Errorf("Got cleared %v; want %v", cleared, tc.expCleared)
			}
			if tc.expClass == nil {
				return
			}
			pvc, err := clientset.CoreV1().PersistentVolumeClaims("default").Get(context.Background(), "test-pvc", metav1.GetOptions{})
			if err != nil {
				t.Fatalf("Failed to get PersistentVolumeClaim: %v", err)
			}
			got := ""
			if pvc.Spec.VolumeAttributesClassName != nil {
				got = *pvc.Spec.VolumeAttributesClassName
			}
			if got != *tc.expClass {
				t.Errorf("Claim names class %q; want %q", got, *tc.expClass)
			}
		})
	}
}

func TestUpdatePVAnnotations(t *testing.T) {
	pvResource := schema.GroupResource{Resource: "persistentvolumes"}

	t.Run("retries an update that lost a race with another writer", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(newPV("test-pv", map[string]string{testAnnotationKey: "Pending"}))
		updates := 0
		clientset.PrependReactor("update", "persistentvolumes", func(k8stesting.Action) (bool, runtime.Object, error) {
			updates++
			if updates == 1 {
				return true, nil, apierrors.NewConflict(pvResource, "test-pv", nil)
			}
			return false, nil, nil
		})
		withFakeClient(t, clientset)

		err := UpdatePVAnnotations(context.Background(), "test-pv",
			map[string]PVAnnotationCondition{testAnnotationKey: {Value: "Pending", Exists: true}},
			map[string]string{testAnnotationKey: "operation"}, nil)
		if err != nil {
			t.Fatalf("UpdatePVAnnotations failed: %v", err)
		}
		if updates != 2 {
			t.Errorf("Made %d updates; want 2", updates)
		}
		if got := getAnnotations(context.Background(), t, clientset, "test-pv")[testAnnotationKey]; got != "operation" {
			t.Errorf("Annotation %s = %q; want %q", testAnnotationKey, got, "operation")
		}
	})

	t.Run("does not retry or write when a condition no longer holds", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(newPV("test-pv", map[string]string{testAnnotationKey: "newer-operation"}))
		gets, updates := 0, 0
		clientset.PrependReactor("get", "persistentvolumes", func(k8stesting.Action) (bool, runtime.Object, error) {
			gets++
			return false, nil, nil
		})
		clientset.PrependReactor("update", "persistentvolumes", func(k8stesting.Action) (bool, runtime.Object, error) {
			updates++
			return false, nil, nil
		})
		withFakeClient(t, clientset)

		err := UpdatePVAnnotations(context.Background(), "test-pv",
			map[string]PVAnnotationCondition{testAnnotationKey: {Value: "Pending", Exists: true}},
			map[string]string{testAnnotationKey: "operation"}, nil)
		if !apierrors.IsConflict(err) {
			t.Fatalf("UpdatePVAnnotations error = %v; want a conflict", err)
		}
		if gets != 1 || updates != 0 {
			t.Errorf("Made %d gets and %d updates; want 1 get and no updates", gets, updates)
		}
		if got := getAnnotations(context.Background(), t, clientset, "test-pv")[testAnnotationKey]; got != "newer-operation" {
			t.Errorf("Annotation %s = %q; want it left as %q", testAnnotationKey, got, "newer-operation")
		}
	})
}
