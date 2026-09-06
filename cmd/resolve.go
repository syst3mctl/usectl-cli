package cmd

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/giorgi/usectl/api"
)

// Name resolution.
//
// Every machine-scoped command used to demand raw UUIDs, so even a trivial
// action meant two lookups first:
//
//	usectl apps addons attach 80f75010-... 83830047-... dc2f4fe2-...
//
// Machine names are unique platform-wide (mig 069), pod names are unique per
// machine, and addon names are unique per (machine, type) — so all three are
// safely addressable by name:
//
//	usectl machines pods attach-addon runbyagents painboard database/painboard
//
// UUIDs keep working everywhere; a value that parses as one is passed straight
// through without a lookup.

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(s string) bool { return uuidRe.MatchString(s) }

// ambiguousError reports every candidate rather than picking one, because
// silently choosing among machines is how the wrong one gets deleted.
func ambiguousError(kind, ref string, matches []string) error {
	sort.Strings(matches)
	return fmt.Errorf("%s %q is ambiguous — matches %s\n  use the full name or the id",
		kind, ref, strings.Join(matches, ", "))
}

// resolveMachine turns a machine name, id, or unambiguous id prefix into an id.
func resolveMachine(client *api.Client, ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("machine is required")
	}
	if isUUID(ref) {
		return ref, nil
	}
	projects, err := client.ListProjects()
	if err != nil {
		return "", err
	}
	var byName, byPrefix []string
	for _, pw := range projects {
		if pw.Project.Name == ref {
			byName = append(byName, pw.Project.ID)
		}
		if strings.HasPrefix(pw.Project.ID, ref) {
			byPrefix = append(byPrefix, pw.Project.ID)
		}
	}
	switch {
	case len(byName) == 1:
		return byName[0], nil
	case len(byName) > 1:
		return "", ambiguousError("machine", ref, byName)
	case len(byPrefix) == 1:
		return byPrefix[0], nil
	case len(byPrefix) > 1:
		return "", ambiguousError("machine", ref, byPrefix)
	}
	return "", fmt.Errorf("no machine named %q — run 'usectl machines list'", ref)
}

// resolvePod turns a pod name or id into an app id within one machine.
func resolvePod(client *api.Client, machineID, ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("pod is required")
	}
	if isUUID(ref) {
		return ref, nil
	}
	apps, err := client.ListProjectApps(machineID)
	if err != nil {
		return "", err
	}
	var matches, names []string
	for _, a := range apps {
		names = append(names, a.Name)
		if a.Name == ref || strings.HasPrefix(a.ID, ref) {
			matches = append(matches, a.ID)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", ambiguousError("pod", ref, matches)
	}
	sort.Strings(names)
	return "", fmt.Errorf("no pod named %q in this machine — have: %s", ref, strings.Join(names, ", "))
}

// resolveAddon accepts an addon id, its name, its type, or "type/name".
//
// The "type/name" form exists because name alone is ambiguous by design: a
// machine may hold both a "primary" database and a "primary" s3 bucket.
func resolveAddon(client *api.Client, machineID, ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("addon is required")
	}
	if isUUID(ref) {
		return ref, nil
	}
	addons, err := client.ListProjectAddons(machineID)
	if err != nil {
		return "", err
	}
	wantType, wantName := "", ref
	if t, n, ok := strings.Cut(ref, "/"); ok {
		wantType, wantName = t, n
	}

	var matches, labels []string
	for _, a := range addons {
		labels = append(labels, a.AddonType+"/"+a.Name)
		if wantType != "" {
			if a.AddonType == wantType && a.Name == wantName {
				matches = append(matches, a.ID)
			}
			continue
		}
		// Bare reference: match on either the instance name or the type.
		if a.Name == ref || a.AddonType == ref || strings.HasPrefix(a.ID, ref) {
			matches = append(matches, a.ID)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("addon %q is ambiguous in this machine — qualify it as type/name, e.g. %s",
			ref, strings.Join(labels, " or "))
	}
	sort.Strings(labels)
	return "", fmt.Errorf("no addon %q in this machine — have: %s", ref, strings.Join(labels, ", "))
}
