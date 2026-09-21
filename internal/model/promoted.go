package model

import (
	"strings"
	"sync"
)

// PromotedMembers returns the names of all promoted fields and promoted methods
// reachable through this class's embedded structs and interfaces within the same package.
func (c *Class) PromotedMembers() (map[string]bool, map[string]bool) {
	if c == nil || len(c.Embeds) == 0 || c.File == nil {
		return map[string]bool{}, map[string]bool{}
	}
	collector := newPromotedMemberCollector(c)
	return collector.collect(c.Embeds)
}

type promotedMemberCollector struct {
	classByName     map[string]*Class
	ifaceByName     map[string]*Interface
	directFields    map[string]bool
	directMethods   map[string]bool
	promotedFields  map[string]bool
	promotedMethods map[string]bool
	visited         map[string]bool
}

func newPromotedMemberCollector(c *Class) *promotedMemberCollector {
	classByName, ifaceByName := c.File.packageTypeIndexes()
	return &promotedMemberCollector{
		classByName:     classByName,
		ifaceByName:     ifaceByName,
		directFields:    directFieldSet(c.Fields),
		directMethods:   directMethodSet(c.Methods),
		promotedFields:  map[string]bool{},
		promotedMethods: map[string]bool{},
		visited:         map[string]bool{c.Name: true},
	}
}

func (c *promotedMemberCollector) collect(embeds []string) (map[string]bool, map[string]bool) {
	var classQueue []*Class
	var ifaceQueue []*Interface
	for _, embedName := range embeds {
		c.tryEnqueueClass(embedName, &classQueue, &ifaceQueue)
	}
	c.processClassQueue(classQueue, &ifaceQueue)
	c.processIfaceQueue(ifaceQueue)
	return c.promotedFields, c.promotedMethods
}

func (c *promotedMemberCollector) addField(name string) {
	if !c.directFields[name] && !c.directMethods[name] && !c.promotedFields[name] {
		c.promotedFields[name] = true
	}
}

func (c *promotedMemberCollector) addMethod(name string) {
	if !c.directMethods[name] && !c.directFields[name] && !c.promotedMethods[name] {
		c.promotedMethods[name] = true
	}
}

func (c *promotedMemberCollector) addClassMembers(curr *Class) {
	for _, f := range curr.Fields {
		c.addField(f.Name)
	}
	for _, m := range curr.Methods {
		c.addMethod(m.Name)
	}
}

func (c *promotedMemberCollector) addIfaceMembers(curr *Interface) {
	for _, m := range curr.Methods {
		c.addMethod(m.Name)
	}
}

func (c *promotedMemberCollector) tryEnqueueClass(name string, classQueue *[]*Class, ifaceQueue *[]*Interface) {
	clean := cleanEmbedName(name)
	if clean == "" || c.visited[clean] {
		return
	}
	c.visited[clean] = true
	if embClass := c.classByName[clean]; embClass != nil {
		*classQueue = append(*classQueue, embClass)
		return
	}
	if embIface := c.ifaceByName[clean]; embIface != nil {
		*ifaceQueue = append(*ifaceQueue, embIface)
	}
}

func (c *promotedMemberCollector) tryEnqueueIface(name string, queue *[]*Interface) {
	clean := cleanEmbedName(name)
	if clean == "" || c.visited[clean] {
		return
	}
	c.visited[clean] = true
	if embIface := c.ifaceByName[clean]; embIface != nil {
		*queue = append(*queue, embIface)
	}
}

func (c *promotedMemberCollector) processClassQueue(queue []*Class, ifaceQueue *[]*Interface) {
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		c.addClassMembers(curr)
		for _, embedName := range curr.Embeds {
			c.tryEnqueueClass(embedName, &queue, ifaceQueue)
		}
	}
}

func (c *promotedMemberCollector) processIfaceQueue(queue []*Interface) {
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		c.addIfaceMembers(curr)
		for _, embedName := range curr.Embeds {
			c.tryEnqueueIface(embedName, &queue)
		}
	}
}

// PackageTypeIndex is a lazily built index of named classes and interfaces.
// A runner can share one index across all files in a package.
type PackageTypeIndex struct {
	once        sync.Once
	classes     []*Class
	interfaces  []*Interface
	classByName map[string]*Class
	ifaceByName map[string]*Interface
}

// NewPackageTypeIndex creates a package type index that builds its maps on
// first use.
func NewPackageTypeIndex(classes []*Class, interfaces []*Interface) *PackageTypeIndex {
	return &PackageTypeIndex{classes: classes, interfaces: interfaces}
}

func (i *PackageTypeIndex) maps() (map[string]*Class, map[string]*Interface) {
	i.once.Do(func() {
		classByName := make(map[string]*Class, len(i.classes))
		for _, cls := range i.classes {
			classByName[cls.Name] = cls
		}
		ifaceByName := make(map[string]*Interface, len(i.interfaces))
		for _, iface := range i.interfaces {
			ifaceByName[iface.Name] = iface
		}
		i.classByName = classByName
		i.ifaceByName = ifaceByName
	})
	return i.classByName, i.ifaceByName
}

func (f *File) packageTypeIndexes() (map[string]*Class, map[string]*Interface) {
	if f == nil {
		return map[string]*Class{}, map[string]*Interface{}
	}
	if f.PackageTypeIndex != nil {
		return f.PackageTypeIndex.maps()
	}
	f.analysis.packageTypeIndexOnce.Do(func() {
		classes := f.PackageClasses
		if classes == nil {
			classes = f.Classes
		}
		interfaces := f.PackageInterfaces
		if interfaces == nil {
			interfaces = f.Interfaces
		}
		f.analysis.packageTypeIndex = NewPackageTypeIndex(classes, interfaces)
	})
	return f.analysis.packageTypeIndex.maps()
}

func directFieldSet(fields []*Field) map[string]bool {
	set := make(map[string]bool, len(fields))
	for _, f := range fields {
		set[f.Name] = true
	}
	return set
}

func directMethodSet(methods []*Function) map[string]bool {
	set := make(map[string]bool, len(methods))
	for _, m := range methods {
		set[m.Name] = true
	}
	return set
}

func cleanEmbedName(name string) string {
	clean := strings.TrimPrefix(name, "*")
	if strings.Contains(clean, ".") {
		return ""
	}
	return clean
}
