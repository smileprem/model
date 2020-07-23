package model

import (
	. "goa.design/structurizr/dsl"
	"goa.design/structurizr/examples/nested/styles"
)

var Subsystem2 = Workspace("Subsystem 2", "This is a model of subsystem 2.", func() {
	var System = SoftwareSystem("Subsystem 2", "A software system that belongs to subsystem 2.", func() {
		Container("Microservice A", "A microservice of subsystem 2", "Go and Goa")
		Tag("system")
	})

	Views(func() {
		styles.DefineAll() // Use shared styles

		SystemContextView(System, "Subsystem 2 context", "System context diagram for Subsystem 2.", func() {
			AddAll()
			AutoLayout(RankTopBottom)
		})
	})
})
