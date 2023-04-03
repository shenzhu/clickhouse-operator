// Copyright 2019 Altinity Ltd and/or its affiliates. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model

import (
	"fmt"

	"gopkg.in/d4l3k/messagediff.v1"

	chiV1 "github.com/altinity/clickhouse-operator/pkg/apis/clickhouse.altinity.com/v1"
	"github.com/altinity/clickhouse-operator/pkg/chop"
)

func isZookeeperChangeRequiresReboot(host *chiV1.ChiHost, a, b *chiV1.ChiZookeeperConfig) bool {
	return !a.Equals(b)
}

func makePaths(a, b *chiV1.Settings, prefix string, path *messagediff.Path, value interface{}) (sections []string) {
	if settings, ok := (value).(*chiV1.Settings); ok {
		// Whole settings such as 'files' or 'settings' is being either added or removed
		if settings == nil {
			// Completely remove settings such as 'files' or 'settings', so the value changes from Settings to nil
			// List the whole settings that are removed
			for _, name := range a.Names() {
				sections = append(sections, prefix+"/"+name)
			}
		} else {
			// Introduce new settings such as 'files' or 'settings', so the value changes from nil to Settings
			// List the whole settings that is added
			for _, name := range b.Names() {
				sections = append(sections, prefix+"/"+name)
			}
		}
	} else {
		// Modify settings such as 'files' or 'settings' but without full removal,
		// something is still left in the remaining settings
		suffix := ""
		for _, p := range *path {
			switch mk := p.(type) {
			case messagediff.MapKey:
				switch str := mk.Key.(type) {
				case string:
					suffix += "/" + str
				}
			}
		}
		sections = append(sections, prefix+suffix)
	}
	return sections
}

func makePathsFromDiff(a, b *chiV1.Settings, diff *messagediff.Diff, prefix string) (res []string) {
	for path, value := range diff.Added {
		res = append(res, makePaths(a, b, prefix, path, value)...)
	}
	for path, value := range diff.Removed {
		res = append(res, makePaths(a, b, prefix, path, value)...)
	}
	for path, value := range diff.Modified {
		res = append(res, makePaths(a, b, prefix, path, value)...)
	}
	return res
}

func isSettingsChangeRequiresReboot(host *chiV1.ChiHost, section string, a, b *chiV1.Settings) bool {
	diff, equal := messagediff.DeepDiff(a, b)
	if equal {
		return false
	}
	affectedPaths := makePathsFromDiff(a, b, diff, section)
	return isListedChangeRequiresReboot(host, affectedPaths)
}

func hostVersionMatches(host *chiV1.ChiHost, version string) bool {
	return host.Version.Matches(version)
}

func ruleMatches(set chiV1.OperatorConfigRestartPolicyRuleSet, path string) (matches bool, value bool) {
	for pattern, val := range set {
		if pattern.Match(path) {
			matches = true
			value = val.IsTrue()
			return matches, value
		}
		// Only one check has to be performed since we are expecting rule to have one entry
		matches = false
		value = false
		return matches, value
	}
	matches = false
	value = false
	return matches, value
}

// latestConfigMatch
func latestConfigMatch(host *chiV1.ChiHost, path string) (matches bool, value bool) {
	for _, r := range chop.Config().ClickHouse.ConfigRestartPolicy.Rules {
		// Check ClickHouse version
		_ = fmt.Sprintf("%s", r.Version)
		if hostVersionMatches(host, r.Version) {
			// Check whether any rule matches path
			for _, rule := range r.Rules {
				if ruleMatches, ruleValue := ruleMatches(rule, path); ruleMatches {
					matches = true
					value = ruleValue
				}
			}
		}
	}
	return matches, value
}

// isListedChangeRequiresReboot
func isListedChangeRequiresReboot(host *chiV1.ChiHost, paths []string) bool {
	// Check whether any path matches ClickHouse configuration restart policy rules requires reboot
	for _, path := range paths {
		if matches, value := latestConfigMatch(host, path); matches {
			// This path matches config
			if value {
				// And this path matches and requires reboot - no need to find any other who requires reboot
				return true
			}
		}
	}
	return false
}

// IsConfigurationChangeRequiresReboot checks whether configuration changes requires a reboot
func IsConfigurationChangeRequiresReboot(host *chiV1.ChiHost) bool {
	// Zookeeper
	{
		var old, new *chiV1.ChiZookeeperConfig
		if host.HasAncestor() {
			old = host.GetAncestor().GetZookeeper()
		}
		new = host.GetZookeeper()
		if isZookeeperChangeRequiresReboot(host, old, new) {
			return true
		}
	}
	// Profiles Global
	{
		var old, new *chiV1.Settings
		if host.HasAncestorCHI() {
			old = host.GetAncestorCHI().Spec.Configuration.Profiles
		}
		if host.HasCHI() {
			new = host.GetCHI().Spec.Configuration.Profiles
		}
		if isSettingsChangeRequiresReboot(host, "profiles", old, new) {
			return true
		}
	}
	// Quotas Global
	{
		var old, new *chiV1.Settings
		if host.HasAncestorCHI() {
			old = host.GetAncestorCHI().Spec.Configuration.Quotas
		}
		if host.HasCHI() {
			new = host.GetCHI().Spec.Configuration.Quotas
		}
		if isSettingsChangeRequiresReboot(host, "quotas", old, new) {
			return true
		}
	}
	// Settings Global
	{
		var old, new *chiV1.Settings
		if host.HasAncestorCHI() {
			old = host.GetAncestorCHI().Spec.Configuration.Settings
		}
		if host.HasCHI() {
			new = host.GetCHI().Spec.Configuration.Settings
		}
		if isSettingsChangeRequiresReboot(host, "settings", old, new) {
			return true
		}
	}
	// Settings Local
	{
		var old, new *chiV1.Settings
		if host.HasAncestor() {
			old = host.GetAncestor().Settings
		}
		new = host.Settings
		if isSettingsChangeRequiresReboot(host, "settings", old, new) {
			return true
		}
	}
	// Files Global
	{
		var old, new *chiV1.Settings
		if host.HasAncestorCHI() {
			old = host.GetAncestorCHI().Spec.Configuration.Files.Filter(
				nil,
				[]chiV1.SettingsSection{chiV1.SectionUsers},
				true,
			)
		}
		if host.HasCHI() {
			new = host.GetCHI().Spec.Configuration.Files.Filter(
				nil,
				[]chiV1.SettingsSection{chiV1.SectionUsers},
				true,
			)
		}
		if isSettingsChangeRequiresReboot(host, "files", old, new) {
			return true
		}
	}
	// Files Local
	{
		var old, new *chiV1.Settings
		if host.HasAncestor() {
			old = host.GetAncestor().Files.Filter(
				nil,
				[]chiV1.SettingsSection{chiV1.SectionUsers},
				true,
			)
		}
		new = host.Files.Filter(
			nil,
			[]chiV1.SettingsSection{chiV1.SectionUsers},
			true,
		)
		if isSettingsChangeRequiresReboot(host, "files", old, new) {
			return true
		}
	}
	return false
}
