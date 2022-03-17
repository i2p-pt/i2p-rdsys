package core

import (
	"errors"
	"fmt"
	"hash/crc64"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

const crc64Polynomial = 0x42F0E1EBA9EA3693

var crc64Table = crc64.MakeTable(crc64Polynomial)

// ResourceDiff represents a diff that contains new, changed, and gone
// resources.  A resource diff can be applied onto data structures that
// implement a collection of resources, e.g. a Hashring.
type ResourceDiff struct {
	New     ResourceMap `json:"new"`
	Changed ResourceMap `json:"changed"`
	Gone    ResourceMap `json:"gone"`
}

// Hashkey represents an index in a hashring.
type Hashkey uint64

// Hashnodes represents a node in a hashring.
type hashnode struct {
	hashkey    Hashkey
	elem       Resource
	lastUpdate time.Time
}

// Hashring represents a hashring consisting of resources.
type Hashring struct {
	hashnodes []*hashnode
	sync.RWMutex
}

// FilterFunc takes as input a resource and returns true or false, depending on
// its filtering criteria.
type FilterFunc func(r Resource) bool

// NewResourceDiff returns a new ResourceDiff.
func NewResourceDiff() *ResourceDiff {
	return &ResourceDiff{
		New:     make(ResourceMap),
		Changed: make(ResourceMap),
		Gone:    make(ResourceMap),
	}
}

// NewHashkey calculates a hash from the id to be used index in the hashring
func NewHashkey(id string) Hashkey {
	return Hashkey(crc64.Checksum([]byte(id), crc64Table))
}

// NewHashnode returns a new hash node and sets its LastUpdate field to the
// current UTC time.
func NewHashnode(k Hashkey, r Resource) *hashnode {
	return &hashnode{hashkey: k, elem: r, lastUpdate: time.Now().UTC()}
}

// NewHashring returns a new hashring.
func NewHashring() *Hashring {

	h := &Hashring{}
	return h
}

// String returns a string representation of ResourceDiff.
func (m *ResourceDiff) String() string {

	s := []string{}
	f := func(desc string, rMap ResourceMap) {
		for rType, rQueue := range rMap {
			s = append(s, fmt.Sprintf("%d %s %s", len(rQueue), desc, rType))
		}
	}
	f("new", m.New)
	f("changed", m.Changed)
	f("gone", m.Gone)

	return "Resource diff: " + strings.Join(s, ", ")
}

// Len implements the sort interface.
// This function is unsafe and needs a mutex lock before being used
func (h *Hashring) Len() int {
	return len(h.hashnodes)
}

// Less implements the sort interface.
// This function is unsafe and needs a mutex lock before being used
func (h *Hashring) Less(i, j int) bool {
	return h.hashnodes[i].hashkey < h.hashnodes[j].hashkey
}

// Swap implements the sort interface.
// This function is unsafe and needs a mutex lock before being used
func (h *Hashring) Swap(i, j int) {
	h.hashnodes[i], h.hashnodes[j] = h.hashnodes[j], h.hashnodes[i]
}

// ApplyDiff applies the given ResourceDiff to the hashring.  New resources are
// added, changed resources are updated, and gone resources are removed.
func (h *Hashring) ApplyDiff(d *ResourceDiff) {

	for rType, resources := range d.New {
		log.Printf("Adding %d resources of type %s.", len(resources), rType)
		for _, r := range resources {
			h.Add(r)
		}
	}
	for rType, resources := range d.Changed {
		log.Printf("Changing %d resources of type %s.", len(resources), rType)
		for _, r := range resources {
			h.AddOrUpdate(r)
		}
	}
	for rType, resources := range d.Gone {
		log.Printf("Removing %d resources of type %s.", len(resources), rType)
		for _, r := range resources {
			h.Remove(r)
		}
	}
}

// Add adds the given resource to the hashring.  If the resource is already
// present, we update its timestamp and return an error.
func (h *Hashring) Add(r Resource) error {
	h.Lock()
	defer h.Unlock()

	// Does the hashring already have the resource?
	if i, err := h.getIndex(r.Uid()); err == nil {
		h.hashnodes[i].lastUpdate = time.Now().UTC()
		return errors.New("resource already present in hashring")
	}
	h.maybeTestResource(r)

	n := NewHashnode(r.Uid(), r)
	h.hashnodes = append(h.hashnodes, n)
	sort.Sort(h)
	return nil
}

// maybeTestResource may test the given resource.  The resource is *not* tested
// if all of the following conditions are met:
//
//   * The resource (as identified by its UID *and* OID) already exists.
//   * The resource has been last tested before the resource's expiry.
func (h *Hashring) maybeTestResource(r Resource) {

	var oldR Resource
	// Does the resource already exist in our hashring?
	if i, err := h.getIndex(r.Uid()); err == nil {
		oldR = h.hashnodes[i].elem
		// And is it exactly the same as the one we're dealing with?
		if oldR.Oid() == r.Oid() {
			rTest := oldR.TestResult()
			r = oldR
			// Is the resource already tested?
			if rTest != nil && rTest.State != StateUntested {
				// And if so, has it been tested recently?
				if time.Now().UTC().Sub(rTest.LastTested) < oldR.Expiry() {
					return
				}
			}
		}
	}
	go r.Test()
}

// AddOrUpdate attempts to add the given resource to the hashring.  If it
// already is in the hashring, we update it if (and only if) its object ID
// changed.
func (h *Hashring) AddOrUpdate(r Resource) (event int) {
	h.Lock()
	defer h.Unlock()

	event = ResourceUnchanged
	h.maybeTestResource(r)
	// Does the hashring already have the resource?
	if i, err := h.getIndex(r.Uid()); err == nil {
		h.hashnodes[i].lastUpdate = time.Now().UTC()
		// If so, we only update it if its object ID changed.
		if h.hashnodes[i].elem.Oid() != r.Oid() {
			h.hashnodes[i].elem = r
			event = ResourceChanged
		}
	} else {
		n := NewHashnode(r.Uid(), r)
		h.hashnodes = append(h.hashnodes, n)
		sort.Sort(h)
		event = ResourceIsNew
	}
	return
}

// Remove removes the given resource from the hashring.  If the hashring is
// empty or we cannot find the key, an error is returned.
func (h *Hashring) Remove(r Resource) error {
	h.Lock()
	defer h.Unlock()
	return h.remove(r)
}

// remove without locking the mutex
func (h *Hashring) remove(r Resource) error {
	i, err := h.getIndex(r.Uid())
	if err != nil {
		return err
	}

	leftPart := h.hashnodes[:i]
	rightPart := h.hashnodes[i+1:]
	h.hashnodes = append(leftPart, rightPart...)

	return nil
}

// getIndex attempts to return the index of the given hash key.  If the given
// hash key is present in the hashring, we return its index.  If the hashring
// is empty, an error is returned.  If the hash key cannot be found, an error
// is returned *and* the returned index is set to the *next* matching element
// in the hashring.
func (h *Hashring) getIndex(k Hashkey) (int, error) {

	if h.Len() == 0 {
		return -1, errors.New("hashring is empty")
	}

	i := sort.Search(h.Len(), func(i int) bool {
		return h.hashnodes[i].hashkey >= k
	})

	if i >= h.Len() {
		i = 0
	}

	if i < h.Len() && h.hashnodes[i].hashkey == k {
		return i, nil
	} else {
		return i, errors.New("could not find key in hashring")
	}
}

// Get attempts to retrieve the element identified by the given hash key.  If
// the hashring is empty, an error is returned.  If there is no exact match for
// the given hash key, we return the element whose hash key is the closest to
// the given hash key in descending direction.
func (h *Hashring) Get(k Hashkey) (Resource, error) {
	h.RLock()
	defer h.RUnlock()

	i, err := h.getIndex(k)
	if err != nil && i == -1 {
		return nil, err
	}
	return h.hashnodes[i].elem, nil
}

// GetExact attempts to retrieve the element identified by the given hash key.
// If we cannot find the element, an error is returned.
func (h *Hashring) GetExact(k Hashkey) (Resource, error) {
	h.RLock()
	defer h.RUnlock()

	i, err := h.getIndex(k)
	if err != nil {
		return nil, err
	}
	return h.hashnodes[i].elem, nil
}

// GetMany behaves like Get with the exception that it attempts to return the
// given number of elements.  If the number of desired elements exceeds the
// number of elements in the hashring, an error is returned.
func (h *Hashring) GetMany(k Hashkey, num int) ([]Resource, error) {
	h.RLock()
	defer h.RUnlock()

	if num > h.Len() {
		return nil, errors.New("requested more elements than hashring has")
	}

	var resources []Resource
	i, err := h.getIndex(k)
	if err != nil && i == -1 {
		return nil, err
	}

	for j := i; j < num+i; j++ {
		resources = append(resources, h.hashnodes[j%h.Len()].elem)
	}
	return resources, nil
}

// GetAll returns all of the hashring's resources.
func (h *Hashring) GetAll() []Resource {
	h.RLock()
	defer h.RUnlock()

	var elems []Resource
	for _, node := range h.hashnodes {
		elems = append(elems, node.elem)
	}
	return elems
}

// Filter filters the resources of this hashring with the given filter function
// and returns the remaining resources as another hashring.
func (h *Hashring) Filter(f FilterFunc) *Hashring {
	h.RLock()
	defer h.RUnlock()

	r := &Hashring{}
	for _, n := range h.hashnodes {
		if f(n.elem.(Resource)) {
			r.Add(n.elem.(Resource))
		}
	}
	return r
}

// Prune prunes and returns expired resources from the hashring.
func (h *Hashring) Prune() []Resource {
	h.Lock()
	defer h.Unlock()

	now := time.Now().UTC()
	pruned := []Resource{}

	for _, node := range h.hashnodes {
		if now.Sub(node.lastUpdate) > node.elem.Expiry() {
			pruned = append(pruned, node.elem)
			h.remove(node.elem)
		}
	}

	return pruned
}
