/*
Copyright 2022 The Kubernetes Authors.

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

package common

import (
	"fmt"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/sets"
)

type ResourceTypes interface {
	Types() []string
}

type resourceTypesStatic func() []string

func (fn resourceTypesStatic) Types() []string { return fn() }

type resourceTypesWatcher struct {
	providedTypes []string
	config        string
	mergedTypes   []string
	rw            sync.RWMutex
}

func NewResourceTypes(providedTypes []string, config string) (ResourceTypes, error) {
	if config == "" {
		return resourceTypesStatic(func() []string { return providedTypes }), nil
	}
	rtw := resourceTypesWatcher{
		providedTypes: providedTypes,
		config:        config,
		rw:            sync.RWMutex{},
	}

	// Read and parse the types for the first time
	// In this way 'resourceTypesWatcher.Types()' returns the correct result
	// even if the config is never updated
	mergedTypes, err := rtw.mergeTypesFromConfig()
	if err != nil {
		return nil, fmt.Errorf("merge type from config: %w", err)
	}
	rtw.mergedTypes = mergedTypes
	rtw.watchConfig()

	return &rtw, nil
}

func (rtw *resourceTypesWatcher) watchConfig() {
	go func() {
		v := viper.New()
		v.SetConfigFile(rtw.config)
		v.SetConfigType("yaml")
		v.OnConfigChange(func(in fsnotify.Event) {
			logrus.Info("a new boskos config is available")
			newMergedTypes, err := rtw.mergeTypesFromConfig()
			if err != nil {
				logrus.WithError(err).Warn("failed to merge types from a new config")
				return
			}

			// Keep the critical region as small as possible
			rtw.rw.Lock()
			rtw.mergedTypes = newMergedTypes
			rtw.rw.Unlock()

			logrus.Infof("new resources: %+v", rtw.mergedTypes)
		})
		v.WatchConfig()
	}()
}

func (rtw *resourceTypesWatcher) mergeTypesFromConfig() ([]string, error) {
	configResources, err := parseResourceTypesFromConfig(rtw.config)
	if err != nil {
		return nil, err
	}
	return sets.NewString(rtw.providedTypes...).Insert(configResources...).List(), nil
}

func (rtw *resourceTypesWatcher) Types() []string {
	rtw.rw.RLock()
	defer rtw.rw.RUnlock()
	return rtw.mergedTypes
}

func parseResourceTypesFromConfig(config string) ([]string, error) {
	types := make([]string, 0)
	cfg, err := ParseConfig(config)
	if err != nil {
		return types, fmt.Errorf("parse config %q: %w", config, err)
	}
	if err := ValidateConfig(cfg); err != nil {
		return types, fmt.Errorf("validate config %q: %w", config, err)
	}
	for _, r := range cfg.Resources {
		types = append(types, r.Type)
	}
	return types, nil
}
