package convert

import (
	"reflect"
	"testing"
)

func TestConvertLabelsStringToMap(t *testing.T) {
	t.Run("parsing labels string into map", func(t *testing.T) {
		testCases := []struct {
			name           string
			labels         string
			expectedOutput map[string]string
			expectedError  bool
		}{
			{
				name:           "should return empty map when labels string is empty",
				labels:         "",
				expectedOutput: map[string]string{},
				expectedError:  false,
			},
			{
				name:   "single label string",
				labels: "key=value",
				expectedOutput: map[string]string{
					"key": "value",
				},
				expectedError: false,
			},
			{
				name:   "multiple label string",
				labels: "key1=value1,key2=value2",
				expectedOutput: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				expectedError: false,
			},
			{
				name:   "multiple labels string with whitespaces gets trimmed",
				labels: "key1=value1, key2=value2",
				expectedOutput: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				expectedError: false,
			},
			{
				name:           "malformed labels string (no keys and values)",
				labels:         ",,",
				expectedOutput: nil,
				expectedError:  true,
			},
			{
				name:           "malformed labels string (incorrect format)",
				labels:         "foo,bar",
				expectedOutput: nil,
				expectedError:  true,
			},
			{
				name:           "malformed labels string (missing key)",
				labels:         "key1=value1,=bar",
				expectedOutput: nil,
				expectedError:  true,
			},
			{
				name:           "malformed labels string (missing key and value)",
				labels:         "key1=value1,=bar,=",
				expectedOutput: nil,
				expectedError:  true,
			},
		}

		for _, tc := range testCases {
			t.Logf("test case: %s", tc.name)
			output, err := ConvertLabelsStringToMap(tc.labels)
			if tc.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if err != nil {
				if !tc.expectedError {
					t.Errorf("Did not expect error but got: %v", err)
				}
				continue
			}

			if !reflect.DeepEqual(output, tc.expectedOutput) {
				t.Errorf("Got labels %v, but expected %v", output, tc.expectedOutput)
			}
		}
	})

	t.Run("checking google requirements", func(t *testing.T) {
		testCases := []struct {
			name          string
			labels        string
			expectedError bool
		}{
			{
				name: "64 labels at most",
				labels: `k1=v,k2=v,k3=v,k4=v,k5=v,k6=v,k7=v,k8=v,k9=v,k10=v,k11=v,k12=v,k13=v,k14=v,k15=v,k16=v,k17=v,k18=v,k19=v,k20=v,
                         k21=v,k22=v,k23=v,k24=v,k25=v,k26=v,k27=v,k28=v,k29=v,k30=v,k31=v,k32=v,k33=v,k34=v,k35=v,k36=v,k37=v,k38=v,k39=v,k40=v,
                         k41=v,k42=v,k43=v,k44=v,k45=v,k46=v,k47=v,k48=v,k49=v,k50=v,k51=v,k52=v,k53=v,k54=v,k55=v,k56=v,k57=v,k58=v,k59=v,k60=v,
                         k61=v,k62=v,k63=v,k64=v,k65=v`,
				expectedError: true,
			},
			{
				name:          "label key must start with lowercase char (# case)",
				labels:        "#k=v",
				expectedError: true,
			},
			{
				name:          "label key must start with lowercase char (_ case)",
				labels:        "_k=v",
				expectedError: true,
			},
			{
				name:          "label key must start with lowercase char (- case)",
				labels:        "-k=v",
				expectedError: true,
			},
			{
				name:          "label key can only contain lowercase chars, digits, _ and -)",
				labels:        "k*=v",
				expectedError: true,
			},
			{
				name:          "label key may not have over 63 characters",
				labels:        "abcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghij1234=v",
				expectedError: true,
			},
			{
				name:          "label key cannot contain . and /",
				labels:        "kubernetes.io/created-for/pvc/namespace=v",
				expectedError: true,
			},
			{
				name:          "label value can only contain lowercase chars, digits, _ and -)",
				labels:        "k1=###",
				expectedError: true,
			},
			{
				name:          "label value may not have over 63 characters",
				labels:        "abcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghij1234=v",
				expectedError: true,
			},
			{
				name:          "label value cannot contain . and /",
				labels:        "kubernetes_io_created-for_pvc_namespace=v./",
				expectedError: true,
			},
			{
				name:          "label key can have up to 63 characters",
				labels:        "abcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghij123=v",
				expectedError: false,
			},
			{
				name:          "label value can have up to 63 characters",
				labels:        "abcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghij123=v",
				expectedError: false,
			},
			{
				name:          "label key can contain _ and -",
				labels:        "kubernetes_io_created-for_pvc_namespace=v",
				expectedError: false,
			},
			{
				name:          "label value can contain _ and -",
				labels:        "k=my_value-2",
				expectedError: false,
			},
		}

		for _, tc := range testCases {
			t.Logf("test case: %s", tc.name)
			_, err := ConvertLabelsStringToMap(tc.labels)

			if tc.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tc.expectedError && err != nil {
				t.Errorf("Did not expect error but got: %v", err)
			}
		}
	})

}

func TestConvertTagsStringToMap(t *testing.T) {
	t.Run("parsing tags string into slice", func(t *testing.T) {
		testCases := []struct {
			name           string
			tags           string
			expectedOutput map[string]string
			expectedError  bool
		}{
			{
				name:           "should return empty slice when tags string is empty",
				tags:           "",
				expectedOutput: nil,
				expectedError:  false,
			},
			{
				name:           "single tag string",
				tags:           "parent/key/value",
				expectedOutput: map[string]string{"parent/key": "value"},
				expectedError:  false,
			},
			{
				name:           "multiple tag string",
				tags:           "parent1/key1/value1,parent2/key2/value2",
				expectedOutput: map[string]string{"parent1/key1": "value1", "parent2/key2": "value2"},
				expectedError:  false,
			},
			{
				name:           "multiple tags string with whitespaces gets trimmed",
				tags:           "parent1/key1/value1, parent2/key2/value2",
				expectedOutput: map[string]string{"parent1/key1": "value1", "parent2/key2": "value2"},
				expectedError:  false,
			},
			{
				name:           "malformed tags string (no parent_ids, keys and values)",
				tags:           ",,",
				expectedOutput: nil,
				expectedError:  true,
			},
			{
				name:           "malformed tags string (incorrect format)",
				tags:           "foo,bar",
				expectedOutput: nil,
				expectedError:  true,
			},
			{
				name:           "malformed tags string (missing parent_id)",
				tags:           "parent1/key1/value1,/key2/value2",
				expectedOutput: nil,
				expectedError:  true,
			},
			{
				name:           "malformed tags string (missing key)",
				tags:           "parent1//value1,parent2/key2/value2",
				expectedOutput: nil,
				expectedError:  true,
			},
			{
				name:           "malformed tags string (missing value)",
				tags:           "parent1/key1/value1,parent2/key2/",
				expectedOutput: nil,
				expectedError:  true,
			},
			{
				name:           "same tag parent_id, key and value string used more than once",
				tags:           "parent1/key1/value1,parent1/key1/value1",
				expectedOutput: nil,
				expectedError:  true,
			},
			{
				name:           "same tag parent_id & key string used more than once",
				tags:           "parent1/key1/value1,parent1/key1/value2",
				expectedOutput: nil,
				expectedError:  true,
			},
		}

		for _, tc := range testCases {
			t.Logf("test case: %s", tc.name)
			output, err := ConvertTagsStringToMap(tc.tags)
			if tc.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tc.expectedError && err != nil {
				t.Errorf("Did not expect error but got: %v", err)
			}

			if err == nil && !reflect.DeepEqual(output, tc.expectedOutput) {
				t.Errorf("Got tags %v, but expected %v", output, tc.expectedOutput)
			}
		}
	})

	t.Run("checking google requirements", func(t *testing.T) {
		testCases := []struct {
			name          string
			tags          string
			expectedError bool
		}{
			{
				name: "50 tags at most",
				tags: `p1/k/v,p2/k/v,p3/k/v,p4/k/v,p5/k/v,p6/k/v,p7/k/v,p8/k/v,p9/k/v,p10/k/v,p11/k/v,p12/k/v,p13/k/v,p14/k/v,p15/k/v,p16/k/v,p17/k/v,
						 p18/k/v,p19/k/v,p20/k/v,p21/k/v,p22/k/v,p23/k/v,p24/k/v,p25/k/v,p26/k/v,p27/k/v,p28/k/v,p29/k/v,p30/k/v,p31/k/v,p32/k/v,p33/k/v,
						 p34/k/v,p35/k/v,p36/k/v,p37/k/v,p38/k/v,p39/k/v,p40/k/v,p41/k/v,p42/k/v,p43/k/v,p44/k/v,p45/k/v,p46/k/v,p47/k/v,p48/k/v,p49/k/v,
						 p50/k/v,p51/k/v`,
				expectedError: true,
			},
			{
				name:          "tag parent_id must start with non-zero decimal when OrganizationID is used (leading zeroes case)",
				tags:          "01/k/v",
				expectedError: true,
			},
			{
				name:          "tag parent_id may not have more than 32 characters when OrganizationID is used",
				tags:          "123546789012345678901234567890123/k/v",
				expectedError: true,
			},
			{
				name:          "tag parent_id can have decimal characters when OrganizationID is used",
				tags:          "1234567890/k/v",
				expectedError: false,
			},
			{
				name:          "tag parent_id may not have less than 6 characters when ProjectID is used",
				tags:          "abcde/k/v",
				expectedError: true,
			},
			{
				name:          "tag parent_id must start with lowercase char when ProjectID is used (decimal case)",
				tags:          "1parent/k/v",
				expectedError: true,
			},
			{
				name:          "tag parent_id must start with lowercase char when ProjectID is used (- case)",
				tags:          "-parent/k/v",
				expectedError: true,
			},
			{
				name:          "tag parent_id must end with lowercase alphanumeric char when ProjectID is used (- case)",
				tags:          "parent-/k/v",
				expectedError: true,
			},
			{
				name:          "tag parent_id may not have more than 30 characters when ProjectID is used",
				tags:          "abcdefghijklmnopqrstuvwxyz12345/k/v",
				expectedError: true,
			},
			{
				name:          "tag parent_id can contain lowercase alphanumeric characters and hyphens when ProjectID is used",
				tags:          "parent-id-100/k/v",
				expectedError: false,
			},
			{
				name:          "tag key must start with alphanumeric char (. case)",
				tags:          "parent/.k/v",
				expectedError: true,
			},
			{
				name:          "tag key must start with alphanumeric char (_ case)",
				tags:          "parent/_k/v",
				expectedError: true,
			},
			{
				name:          "tag key must start with alphanumeric char (- case)",
				tags:          "parent/-k/v",
				expectedError: true,
			},
			{
				name:          "tag key can only contain uppercase, lowercase alphanumeric characters, and the following special characters '._-'",
				tags:          "parent/k*/v",
				expectedError: true,
			},
			{
				name:          "tag key may not have over 63 characters",
				tags:          "parent/abcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghij1234/v",
				expectedError: true,
			},
			{
				name:          "tag key can contain uppercase, lowercase alphanumeric characters, and the following special characters '._-'",
				tags:          "parent/Type_of.cloud-platform/v",
				expectedError: false,
			},
			{
				name:          "tag value must start with alphanumeric char (. case)",
				tags:          "parent/k/.v",
				expectedError: true,
			},
			{
				name:          "tag value must start with alphanumeric char (_ case)",
				tags:          "parent/k/_v",
				expectedError: true,
			},
			{
				name:          "tag value must start with alphanumeric char (- case)",
				tags:          "parent/k/-v",
				expectedError: true,
			},
			{
				name:          "tag value can only contain uppercase, lowercase alphanumeric characters, and the following special characters `_-.@%%=+:,*#&(){}[]` and spaces",
				tags:          "parent/k/v*",
				expectedError: true,
			},
			{
				name:          "tag value may not have over 63 characters",
				tags:          "parent/k/abcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghij1234",
				expectedError: true,
			},
			{
				name:          "tag key can contain uppercase, lowercase alphanumeric characters, and the following special characters `_-.@%%=+:,*#&(){}[]` and spaces",
				tags:          "parent/k/Special@value[10]{20}(30)-example",
				expectedError: false,
			},
		}

		for _, tc := range testCases {
			t.Logf("test case: %s", tc.name)
			_, err := ConvertTagsStringToMap(tc.tags)

			if tc.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tc.expectedError && err != nil {
				t.Errorf("Did not expect error but got: %v", err)
			}
		}
	})
}

func TestConvertStringToBool(t *testing.T) {
	tests := []struct {
		desc        string
		inputStr    string
		expected    bool
		expectError bool
	}{
		{
			desc:        "valid true",
			inputStr:    "true",
			expected:    true,
			expectError: false,
		},
		{
			desc:        "valid mixed case true",
			inputStr:    "True",
			expected:    true,
			expectError: false,
		},
		{
			desc:        "valid false",
			inputStr:    "false",
			expected:    false,
			expectError: false,
		},
		{
			desc:        "valid mixed case false",
			inputStr:    "False",
			expected:    false,
			expectError: false,
		},
		{
			desc:        "invalid",
			inputStr:    "yes",
			expected:    false,
			expectError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := ConvertStringToBool(tc.inputStr)
			if err != nil && !tc.expectError {
				t.Errorf("Got error %v converting string to bool %s; expect no error", err, tc.inputStr)
			}
			if err == nil && tc.expectError {
				t.Errorf("Got no error converting string to bool %s; expect an error", tc.inputStr)
			}
			if err == nil && got != tc.expected {
				t.Errorf("Got %v for converting string to bool; expect %v", got, tc.expected)
			}
		})
	}
}

func TestConvertStringToInt64(t *testing.T) {
	tests := []struct {
		desc        string
		inputStr    string
		expInt64    int64
		expectError bool
	}{
		{
			desc:        "valid number string",
			inputStr:    "10000",
			expInt64:    10000,
			expectError: false,
		},
		{
			desc:        "test higher number",
			inputStr:    "15000",
			expInt64:    15000,
			expectError: false,
		},
		{
			desc:        "round M to number",
			inputStr:    "1M",
			expInt64:    1000000,
			expectError: false,
		},
		{
			desc:        "round m to number",
			inputStr:    "1m",
			expInt64:    1,
			expectError: false,
		},
		{
			desc:        "round k to number",
			inputStr:    "1k",
			expInt64:    1000,
			expectError: false,
		},
		{
			desc:        "invalid empty string",
			inputStr:    "",
			expInt64:    0,
			expectError: true,
		},
		{
			desc:        "invalid string",
			inputStr:    "ew%65",
			expInt64:    0,
			expectError: true,
		},
		{
			desc:        "invalid KiB string",
			inputStr:    "10KiB",
			expInt64:    10000,
			expectError: true,
		},
		{
			desc:        "invalid GB string",
			inputStr:    "10GB",
			expInt64:    0,
			expectError: true,
		},
		{
			desc:        "round Ki to number",
			inputStr:    "1Ki",
			expInt64:    1024,
			expectError: false,
		},
		{
			desc:        "round k to number",
			inputStr:    "10k",
			expInt64:    10000,
			expectError: false,
		},
		{
			desc:        "round Mi to number",
			inputStr:    "10Mi",
			expInt64:    10485760,
			expectError: false,
		},
		{
			desc:        "round M to number",
			inputStr:    "10M",
			expInt64:    10000000,
			expectError: false,
		},
		{
			desc:        "round G to number",
			inputStr:    "10G",
			expInt64:    10000000000,
			expectError: false,
		},
		{
			desc:        "round Gi to number",
			inputStr:    "100Gi",
			expInt64:    107374182400,
			expectError: false,
		},
		{
			desc:        "round decimal to number",
			inputStr:    "1.2Gi",
			expInt64:    1288490189,
			expectError: false,
		},
		{
			desc:        "round big value to number",
			inputStr:    "8191Pi",
			expInt64:    9222246136947933184,
			expectError: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			actualInt64, err := ConvertStringToInt64(tc.inputStr)
			if err != nil && !tc.expectError {
				t.Errorf("Got error %v converting string to int64 %s; expect no error", err, tc.inputStr)
			}
			if err == nil && tc.expectError {
				t.Errorf("Got no error converting string to int64 %s; expect an error", tc.inputStr)
			}
			if err == nil && actualInt64 != tc.expInt64 {
				t.Errorf("Got %d for converting string to int64; expect %d", actualInt64, tc.expInt64)
			}
		})
	}
}

func TestConvertMiStringToInt64(t *testing.T) {
	tests := []struct {
		desc        string
		inputStr    string
		expInt64    int64
		expectError bool
	}{
		{
			desc:        "valid number string",
			inputStr:    "10000",
			expInt64:    1,
			expectError: false,
		},
		{
			desc:        "round Ki to MiB",
			inputStr:    "1000Ki",
			expInt64:    1,
			expectError: false,
		},
		{
			desc:        "round k to MiB",
			inputStr:    "1000k",
			expInt64:    1,
			expectError: false,
		},
		{
			desc:        "round Mi to MiB",
			inputStr:    "1000Mi",
			expInt64:    1000,
			expectError: false,
		},
		{
			desc:        "round M to MiB",
			inputStr:    "1000M",
			expInt64:    954,
			expectError: false,
		},
		{
			desc:        "round G to MiB",
			inputStr:    "1000G",
			expInt64:    953675,
			expectError: false,
		},
		{
			desc:        "round Gi to MiB",
			inputStr:    "10000Gi",
			expInt64:    10240000,
			expectError: false,
		},
		{
			desc:        "round decimal to MiB",
			inputStr:    "1.2Gi",
			expInt64:    1229,
			expectError: false,
		},
		{
			desc:        "round big value to MiB",
			inputStr:    "8191Pi",
			expInt64:    8795019280384,
			expectError: false,
		},
		{
			desc:        "invalid empty string",
			inputStr:    "",
			expInt64:    0,
			expectError: true,
		},
		{
			desc:        "invalid KiB string",
			inputStr:    "10KiB",
			expInt64:    10000,
			expectError: true,
		},
		{
			desc:        "invalid GB string",
			inputStr:    "10GB",
			expInt64:    0,
			expectError: true,
		},
		{
			desc:        "invalid string",
			inputStr:    "ew%65",
			expInt64:    0,
			expectError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			actualInt64, err := ConvertMiStringToInt64(tc.inputStr)
			if err != nil && !tc.expectError {
				t.Errorf("Got error %v converting string to int64 %s; expect no error", err, tc.inputStr)
			}
			if err == nil && tc.expectError {
				t.Errorf("Got no error converting string to int64 %s; expect an error", tc.inputStr)
			}
			if err == nil && actualInt64 != tc.expInt64 {
				t.Errorf("Got %d for converting string to int64; expect %d", actualInt64, tc.expInt64)
			}
		})
	}
}

func TestConvertGiStringToInt64(t *testing.T) {
	tests := []struct {
		desc        string
		inputStr    string
		expInt64    int64
		expectError bool
	}{
		{
			desc:        "valid number string",
			inputStr:    "10000",
			expInt64:    1,
			expectError: false,
		},
		{
			desc:        "round Ki to GiB",
			inputStr:    "1000000Ki",
			expInt64:    1,
			expectError: false,
		},
		{
			desc:        "round k to GiB",
			inputStr:    "1000000k",
			expInt64:    1,
			expectError: false,
		},
		{
			desc:        "round Mi to GiB",
			inputStr:    "1000Mi",
			expInt64:    1,
			expectError: false,
		},
		{
			desc:        "round M to GiB",
			inputStr:    "1000M",
			expInt64:    1,
			expectError: false,
		},
		{
			desc:        "round G to GiB",
			inputStr:    "1000G",
			expInt64:    932,
			expectError: false,
		},
		{
			desc:        "round Gi to GiB - most common case",
			inputStr:    "1234Gi",
			expInt64:    1234,
			expectError: false,
		},
		{
			desc:        "round decimal to GiB",
			inputStr:    "1.2Gi",
			expInt64:    2,
			expectError: false,
		},
		{
			desc:        "round big value to GiB",
			inputStr:    "8191Pi",
			expInt64:    8588886016,
			expectError: false,
		},
		{
			desc:        "invalid empty string",
			inputStr:    "",
			expInt64:    0,
			expectError: true,
		},
		{
			desc:        "invalid KiB string",
			inputStr:    "10KiB",
			expInt64:    10000,
			expectError: true,
		},
		{
			desc:        "invalid GB string",
			inputStr:    "10GB",
			expInt64:    0,
			expectError: true,
		},
		{
			desc:        "invalid string",
			inputStr:    "ew%65",
			expInt64:    0,
			expectError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			actualInt64, err := ConvertGiStringToInt64(tc.inputStr)
			if err != nil && !tc.expectError {
				t.Errorf("Got error %v converting string to int64 %s; expect no error", err, tc.inputStr)
			}
			if err == nil && tc.expectError {
				t.Errorf("Got no error converting string to int64 %s; expect an error", tc.inputStr)
			}
			if err == nil && actualInt64 != tc.expInt64 {
				t.Errorf("Got %d for converting string to int64; expect %d", actualInt64, tc.expInt64)
			}
		})
	}
}
