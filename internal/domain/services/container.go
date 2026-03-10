package services

import "chain-analysis-app/internal/app"

type Container struct {
	Health          *HealthService
	Actors          *ActorService
	Annotations     *AnnotationService
	ActorGraph      *ActorGraphService
	AddressExplorer *AddressExplorerService
	Runs            *RunService
}

func New(legacy *app.App) *Container {
	return &Container{
		Health:          &HealthService{legacy: legacy},
		Actors:          &ActorService{legacy: legacy},
		Annotations:     &AnnotationService{legacy: legacy},
		ActorGraph:      &ActorGraphService{legacy: legacy},
		AddressExplorer: &AddressExplorerService{legacy: legacy},
		Runs:            &RunService{legacy: legacy},
	}
}
