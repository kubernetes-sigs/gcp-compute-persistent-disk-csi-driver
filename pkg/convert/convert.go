package convert

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	volumehelpers "k8s.io/cloud-provider/volume/helpers"
)

var (

	// Regular expressions for validating parent_id, key and value of a resource tag.
	regexParent = regexp.MustCompile(`(^[1-9][0-9]{0,31}$)|(^[a-z][a-z0-9-]{4,28}[a-z0-9]$)`)
	regexKey    = regexp.MustCompile(`^[a-zA-Z0-9]([0-9A-Za-z_.-]{0,61}[a-zA-Z0-9])?$`)
	regexValue  = regexp.MustCompile(`^[a-zA-Z0-9]([0-9A-Za-z_.@%=+:,*#&()\[\]{}\-\s]{0,61}[a-zA-Z0-9])?$`)
)

// ConvertStringToInt64 converts a string to int64
func ConvertStringToInt64(str string) (int64, error) {
	quantity, err := resource.ParseQuantity(str)
	if err != nil {
		return -1, err
	}
	return volumehelpers.RoundUpToB(quantity)
}

// ConvertMiStringToInt64 converts a GiB string to int64
func ConvertMiStringToInt64(str string) (int64, error) {
	quantity, err := resource.ParseQuantity(str)
	if err != nil {
		return -1, err
	}
	return volumehelpers.RoundUpToMiB(quantity)
}

// ConvertGiStringToInt64 converts a GiB string to int64
func ConvertGiStringToInt64(str string) (int64, error) {
	quantity, err := resource.ParseQuantity(str)
	if err != nil {
		return -1, err
	}
	return volumehelpers.RoundUpToGiB(quantity)
}

// ConvertStringToBool converts a string to a boolean.
func ConvertStringToBool(str string) (bool, error) {
	switch strings.ToLower(str) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("Unexpected boolean string %s", str)
}

// ConvertLabelsStringToMap converts the labels from string to map
// example: "key1=value1,key2=value2" gets converted into {"key1": "value1", "key2": "value2"}
// See https://cloud.google.com/compute/docs/labeling-resources#label_format for details.
func ConvertLabelsStringToMap(labels string) (map[string]string, error) {
	const labelsDelimiter = ","
	const labelsKeyValueDelimiter = "="

	labelsMap := make(map[string]string)
	if labels == "" {
		return labelsMap, nil
	}

	regexKey, _ := regexp.Compile(`^\p{Ll}[\p{Ll}0-9_-]{0,62}$`)
	checkLabelKeyFn := func(key string) error {
		if !regexKey.MatchString(key) {
			return fmt.Errorf("label value %q is invalid (should start with lowercase letter / lowercase letter, digit, _ and - chars are allowed / 1-63 characters", key)
		}
		return nil
	}

	regexValue, _ := regexp.Compile(`^[\p{Ll}0-9_-]{0,63}$`)
	checkLabelValueFn := func(value string) error {
		if !regexValue.MatchString(value) {
			return fmt.Errorf("label value %q is invalid (lowercase letter, digit, _ and - chars are allowed / 0-63 characters", value)
		}

		return nil
	}

	keyValueStrings := strings.Split(labels, labelsDelimiter)
	for _, keyValue := range keyValueStrings {
		keyValue := strings.Split(keyValue, labelsKeyValueDelimiter)

		if len(keyValue) != 2 {
			return nil, fmt.Errorf("labels %q are invalid, correct format: 'key1=value1,key2=value2'", labels)
		}

		key := strings.TrimSpace(keyValue[0])
		if err := checkLabelKeyFn(key); err != nil {
			return nil, err
		}

		value := strings.TrimSpace(keyValue[1])
		if err := checkLabelValueFn(value); err != nil {
			return nil, err
		}

		labelsMap[key] = value
	}

	const maxNumberOfLabels = 64
	if len(labelsMap) > maxNumberOfLabels {
		return nil, fmt.Errorf("more than %d labels is not allowed, given: %d", maxNumberOfLabels, len(labelsMap))
	}

	return labelsMap, nil
}

// ConvertTagsStringToMap converts the tags from string to Tag slice
// example: "parent_id1/tag_key1/tag_value1,parent_id2/tag_key2/tag_value2" gets
// converted into {"parent_id1/tag_key1":"tag_value1", "parent_id2/tag_key2":"tag_value2"}
// See https://cloud.google.com/resource-manager/docs/tags/tags-overview,
// https://cloud.google.com/resource-manager/docs/tags/tags-creating-and-managing for details
func ConvertTagsStringToMap(tags string) (map[string]string, error) {
	const tagsDelimiter = ","
	const tagsParentIDKeyValueDelimiter = "/"

	tagsMap := make(map[string]string)
	if tags == "" {
		return nil, nil
	}

	checkTagParentIDFn := func(tag, parentID string) error {
		if !regexParent.MatchString(parentID) {
			return fmt.Errorf("tag parent_id %q for tag %q is invalid. parent_id can have a maximum of 32 characters and cannot be empty. parent_id can be either OrganizationID or ProjectID. OrganizationID must consist of decimal numbers, and cannot have leading zeroes and ProjectID must be 6 to 30 characters in length, can only contain lowercase letters, numbers, and hyphens, and must start with a letter, and cannot end with a hyphen", parentID, tag)
		}
		return nil
	}

	checkTagKeyFn := func(tag, key string) error {
		if !regexKey.MatchString(key) {
			return fmt.Errorf("tag key %q for tag %q is invalid. Tag key can have a maximum of 63 characters and cannot be empty. Tag key must begin and end with an alphanumeric character, and must contain only uppercase, lowercase alphanumeric characters, and the following special characters `._-`", key, tag)
		}
		return nil
	}

	checkTagValueFn := func(tag, value string) error {
		if !regexValue.MatchString(value) {
			return fmt.Errorf("tag value %q for tag %q is invalid. Tag value can have a maximum of 63 characters and cannot be empty. Tag value must begin and end with an alphanumeric character, and must contain only uppercase, lowercase alphanumeric characters, and the following special characters `_-.@%%=+:,*#&(){}[]` and spaces", value, tag)
		}

		return nil
	}

	checkTagParentIDKey := sets.String{}
	parentIDkeyValueStrings := strings.Split(tags, tagsDelimiter)
	for _, parentIDkeyValueString := range parentIDkeyValueStrings {
		parentIDKeyValue := strings.Split(parentIDkeyValueString, tagsParentIDKeyValueDelimiter)

		if len(parentIDKeyValue) != 3 {
			return nil, fmt.Errorf("tag %q is invalid, correct format: 'parent_id1/key1/value1,parent_id2/key2/value2'", parentIDkeyValueString)
		}

		parentID := strings.TrimSpace(parentIDKeyValue[0])
		if err := checkTagParentIDFn(parentIDkeyValueString, parentID); err != nil {
			return nil, err
		}

		key := strings.TrimSpace(parentIDKeyValue[1])
		if err := checkTagKeyFn(parentIDkeyValueString, key); err != nil {
			return nil, err
		}

		value := strings.TrimSpace(parentIDKeyValue[2])
		if err := checkTagValueFn(parentIDkeyValueString, value); err != nil {
			return nil, err
		}

		parentIDKeyStr := fmt.Sprintf("%s/%s", parentID, key)
		if checkTagParentIDKey.Has(parentIDKeyStr) {
			return nil, fmt.Errorf("tag parent_id & key combination %q exists more than once", parentIDKeyStr)
		}
		checkTagParentIDKey.Insert(parentIDKeyStr)

		tagsMap[parentIDKeyStr] = value
	}

	// The maximum number of tags allowed per resource is 50. For more details check the following:
	// https://cloud.google.com/resource-manager/docs/tags/tags-creating-and-managing#attaching
	// https://cloud.google.com/resource-manager/docs/limits#tag-limits
	const maxNumberOfTags = 50
	if len(tagsMap) > maxNumberOfTags {
		return nil, fmt.Errorf("more than %d tags is not allowed, given: %d", maxNumberOfTags, len(tagsMap))
	}

	return tagsMap, nil
}
