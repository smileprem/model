package expr

import (
	"fmt"
	"strings"

	"goa.design/goa/v3/eval"
)

type (
	// Model describes a software architecture model.
	Model struct {
		Enterprise              string
		People                  People
		Systems                 SoftwareSystems
		DeploymentNodes         []*DeploymentNode
		AddImpliedRelationships bool
	}
)

// Parent returns the parent scope for the given element, nil if eh is a Person
// or SoftwareSystem.
func Parent(eh ElementHolder) ElementHolder {
	switch e := eh.(type) {
	case *SoftwareSystem, *Person:
		return nil
	case *Container:
		return e.System
	case *Component:
		return e.Container
	default:
		panic(fmt.Sprintf("unknown element type %T", e)) // bug
	}
}

// EvalName is the qualified name of the DSL expression.
func (m *Model) EvalName() string { return "model" }

// Validate makes sure all element names are unique.
func (m *Model) Validate() error {
	verr := new(eval.ValidationErrors)
	known := make(map[string]struct{})
	for _, p := range m.People {
		if _, ok := known[p.Name]; ok {
			verr.Add(p, "name already in use")
		}
		known[p.Name] = struct{}{}
	}
	for _, s := range m.Systems {
		if _, ok := known[s.Name]; ok {
			verr.Add(s, "name already in use")
		}
		known[s.Name] = struct{}{}
		containers := make(map[string]struct{})
		for _, c := range s.Containers {
			if _, ok := containers[c.Name]; ok {
				verr.Add(c, "name already in use")
			}
			containers[c.Name] = struct{}{}
			components := make(map[string]struct{})
			for _, cm := range c.Components {
				if _, ok := components[cm.Name]; ok {
					verr.Add(cm, "name already in use")
				}
				components[cm.Name] = struct{}{}
			}
		}
	}

	// Finalize all relationship destination now that the DSL has been executed.
	IterateRelationships(func(r *Relationship) {
		if r.Destination != nil {
			return
		}
		// Relationship was created with Uses and used one or more strings to
		// identify the destination.
		eh, err := m.FindElement(Parent(Registry[r.Source.ID].(ElementHolder)), r.DestinationPath)
		if err != nil {
			verr.AddError(r, err)
			return
		}
		r.Destination = eh.GetElement()
	})

	return verr
}

// Finalize adds all implied relationships if needed.
func (m *Model) Finalize() {
	// Add relationships between container instances.
	Iterate(func(e interface{}) {
		if ci, ok := e.(*ContainerInstance); ok {
			c := Registry[ci.ContainerID].(*Container)
			for _, r := range c.Relationships {
				dc, ok := Registry[r.Destination.ID].(*Container)
				if !ok {
					continue
				}
				Iterate(func(e interface{}) {
					eci, ok := e.(*ContainerInstance)
					if !ok {
						return
					}
					if eci.ContainerID == dc.ID {
						rc := r.Dup(ci.Element, eci.Element)
						rc.LinkedRelationshipID = r.ID
						ci.Relationships = append(ci.Relationships, rc)
					}
				})
			}
		}
	})
	if !m.AddImpliedRelationships {
		return
	}
	// Add relationship between element parents.
	Iterate(func(e interface{}) {
		if r, ok := e.(*Relationship); ok {
			switch s := Registry[r.Source.ID].(type) {
			case *Container:
				addMissingRelationships(s.System.Element, r.Destination, r)
			case *Component:
				addMissingRelationships(s.Container.Element, r.Destination, r)
				addMissingRelationships(s.Container.System.Element, r.Destination, r)
			}
		}
	})
}

// Person returns the person with the given name if any, nil otherwise.
func (m *Model) Person(name string) *Person {
	for _, pp := range m.People {
		if pp.Name == name {
			return pp
		}
	}
	return nil
}

// SoftwareSystem returns the software system with the given name if any, nil
// otherwise.
func (m *Model) SoftwareSystem(name string) *SoftwareSystem {
	for _, s := range m.Systems {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// DeploymentNode returns the deployment node with the given name if any, nil
// otherwise.
func (m *Model) DeploymentNode(name string) *DeploymentNode {
	for _, d := range m.DeploymentNodes {
		if d.Name == name {
			return d
		}
	}
	return nil
}

// FindElement finds the element with the given path in the given scope. The path must be one of:
//
//    - "<Person>", "<SoftwareSystem>", "<SoftwareSystem>/<Container>" or "<SoftwareSystem>/<Container>/<Component>"
//    - "<Container>" (if container is a child of the software system scope)
//    - "<Component>" (if component is a child of the container scope)
//    - "<Container>/<Component>" (if container is a child of the software system scope)
//
// The scope may be nil in which case the path must be rooted with a top level
// element (person or software system).
func (m *Model) FindElement(scope ElementHolder, path string) (eh ElementHolder, err error) {
	elems := strings.Split(path, "/")
	switch len(elems) {
	case 1:
		switch s := scope.(type) {
		case *SoftwareSystem:
			if c := s.Container(path); c != nil {
				eh = c
			}
		case *Container:
			if c := s.Component(path); c != nil {
				eh = c
			}
		}
		if eh == nil {
			if p := m.Person(path); p != nil {
				eh = p
			} else if sys := m.SoftwareSystem(path); sys != nil {
				eh = sys
			} else {
				if scope == nil {
					return nil, fmt.Errorf("%q does not match the name of a person, a software system or the path to container or component in scope", path)
				}
				return nil, fmt.Errorf("%q does not match the name of a person, a software system or an element in the scope of %q", path, scope.GetElement().Name)
			}
		}
	case 2:
		if s, ok := scope.(*SoftwareSystem); ok {
			if c := s.Container(elems[0]); c != nil {
				if cmp := c.Component(elems[1]); cmp != nil {
					eh = cmp
				}
			}
		}
		if eh == nil {
			if s := m.SoftwareSystem(elems[0]); s != nil {
				if c := s.Container(elems[1]); c != nil {
					eh = c
				}
			}
			if eh == nil {
				return nil, fmt.Errorf("%q does not match the name of a software system and container or the name of a container and component in the scope of %q", path, scope.GetElement().Name)
			}
		}
	case 3:
		if s := m.SoftwareSystem(elems[0]); s != nil {
			if c := s.Container(elems[1]); c != nil {
				if cmp := c.Component(elems[2]); cmp != nil {
					eh = cmp
				}
			}
		}
		if eh == nil {
			return nil, fmt.Errorf("%q does not match the name of a software system, container and component", path)
		}
	default:
		return nil, fmt.Errorf("too many colons in path")
	}
	return eh, nil
}

// AddPerson adds the given person to the model. If there is already a person
// with the given name then AddPerson merges both definitions. The merge
// algorithm:
//
//    * overrides the description, technology and URL if provided,
//    * merges any new tag or propery into the existing tags and properties,
//    * merges any new relationship into the existing relationships.
//
// AddPerson returns the new or merged person.
func (m *Model) AddPerson(p *Person) *Person {
	existing := m.Person(p.Name)
	if existing == nil {
		Identify(p)
		m.People = append(m.People, p)
		return p
	}
	if p.Description != "" {
		existing.Description = p.Description
	}
	if olddsl := existing.DSLFunc; olddsl != nil {
		existing.DSLFunc = func() { olddsl(); p.DSLFunc() }
	}
	return existing
}

// AddSystem adds the given software system to the model. If there is already a
// software system with the given name then AddSystem merges both definitions.
// The merge algorithm:
//
//    * overrides the description, technology and URL if provided,
//    * merges any new tag or propery into the existing tags and properties,
//    * merges any new relationship into the existing relationships,
//    * merges any new container into the existing containers.
//
// AddSystem returns the new or merged software system.
func (m *Model) AddSystem(s *SoftwareSystem) *SoftwareSystem {
	existing := m.SoftwareSystem(s.Name)
	if existing == nil {
		Identify(s)
		m.Systems = append(m.Systems, s)
		return s
	}
	if s.Description != "" {
		existing.Description = s.Description
	}
	if olddsl := existing.DSLFunc; olddsl != nil {
		existing.DSLFunc = func() { olddsl(); s.DSLFunc() }
	}
	return existing
}

// AddDeploymentNode adds the given deployment node to the model. If there is
// already a deployment node with the given name then AddDeploymentNode merges
// both definitions. The merge algorithm:
//
//    * overrides the description, technology and URL if provided,
//    * merges any new tag or propery into the existing tags and properties,
//    * merges any new relationship into the existing relationships,
//    * merges any new child deployment node into the existing children,
//    * merges any new container instance or infrastructure nodes into existing
//      ones.
//
// AddDeploymentNode returns the new or merged deployment node.
func (m *Model) AddDeploymentNode(d *DeploymentNode) *DeploymentNode {
	existing := m.DeploymentNode(d.Name)
	if existing == nil {
		Identify(d)
		m.DeploymentNodes = append(m.DeploymentNodes, d)
		return d
	}
	if d.Description != "" {
		existing.Description = d.Description
	}
	if d.Technology != "" {
		existing.Technology = d.Technology
	}
	if olddsl := existing.DSLFunc; olddsl != nil {
		existing.DSLFunc = func() { olddsl(); d.DSLFunc() }
	}
	return existing
}

// addMissingRelationships adds relationships from src to element with ID destID
// and its parents (container system software and component container) based on
// the properties of existing. It only adds a relationship if one doesn't
// already exist with the same description.
func addMissingRelationships(src, dest *Element, existing *Relationship) {
	for _, r := range src.Relationships {
		if r.Destination.ID == dest.ID && r.Description == existing.Description {
			return
		}
	}
	r := existing.Dup(src, dest)
	src.Relationships = append(src.Relationships, r)

	// Add relationships to destination parents as well.
	switch e := Registry[dest.ID].(type) {
	case *Container:
		addMissingRelationships(src, e.System.Element, existing)
	case *Component:
		addMissingRelationships(src, e.Container.Element, existing)
		addMissingRelationships(src, e.Container.System.Element, existing)
	}
}
