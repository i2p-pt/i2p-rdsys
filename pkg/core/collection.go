package core

import (
	"fmt"
	"log"
	"sort"
	"strings"
)

// Collection maps a resource type (e.g. "obfs4") to its corresponding
// split hashring.
type Collection map[string]*SplitHashring

// NewCollection creates and returns a new resource collection.
// rTypes is a map of resource type names to a boolean indicating if the resrouce is unpartitioend
func NewCollection() Collection {
	return make(Collection)
}

func (c Collection) AddResourceType(rName string, unpartitioned bool, proportions map[string]int) {
	log.Printf("Creating split hashring for resource %q.", rName)
	c[rName] = NewSplitHashring()
	if !unpartitioned {
		c[rName].Stencil = BuildStencil(proportions)
	}
}

// String returns a summary of the backend resources.
func (c Collection) String() string {
	keys := []string{}
	for rType := range c {
		keys = append(keys, rType)
	}
	sort.Strings(keys)

	s := []string{}
	for _, key := range keys {
		h := c[key]
		s = append(s, fmt.Sprintf("%d %s", h.Len(), key))
	}
	return strings.Join(s, ", ")
}

// Get returns a slice of resources of the requested type for the given
// distributor.
func (c Collection) Get(distName string, rType string) []Resource {
	sHashring, exists := c[rType]
	if !exists {
		log.Printf("Requested resource type %q not present in our resource collection.", rType)
		return []Resource{}
	}

	subHashring, err := sHashring.GetForDist(distName)
	if err != nil {
		log.Printf("Failed to get resources for distributor %q: %s", distName, err)
		return []Resource{}
	}

	var resources []Resource
	for _, elem := range subHashring.GetAll() {
		resources = append(resources, elem.(Resource))
	}
	return resources
}

// GetHashring returns the hashring of the requested type for the given
// distributor.
func (c Collection) GetHashring(distName string, rType string) *Hashring {
	sHashring, exists := c[rType]
	if !exists {
		log.Printf("Requested resource type %q not present in our resource collection.", rType)
		return NewHashring()
	}

	subHashring, err := sHashring.GetForDist(distName)
	if err != nil {
		log.Printf("Failed to get resources for distributor %q: %s", distName, err)
	}
	return subHashring
}

// ApplyDiff updates the collection with the resources changed in ResrouceDiff
func (c Collection) ApplyDiff(diff *ResourceDiff) {
	for rType, resources := range diff.New {
		log.Printf("Adding %d resources of type %s.", len(resources), rType)
		for _, r := range resources {
			c[rType].Add(r)
		}
	}
	for rType, resources := range diff.Changed {
		log.Printf("Changing %d resources of type %s.", len(resources), rType)
		for _, r := range resources {
			c[rType].AddOrUpdate(r)
		}
	}
	for rType, resources := range diff.Gone {
		log.Printf("Removing %d resources of type %s.", len(resources), rType)
		for _, r := range resources {
			c[rType].Remove(r)
		}
	}
}
